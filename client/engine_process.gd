extends Node
# Autoload (see project.godot [autoload]). Owns the lifecycle of the bundled
# engine.exe (embedded Postgres + HTTP/WS API, started once for the life of
# this app) and exposes Start/Stop for the UDP telemetry listener, which the
# user controls on demand — see main_menu.gd's top-bar Play/Stop control,
# which drives this through start_listener/stop_listener and listens for
# listener_status_changed/listener_error. See NewTelemetryEngine/cmd/engine
# for the process this launches.

signal engine_ready
signal engine_failed(reason: String)
signal listener_status_changed(running: bool, port: int)
signal listener_error(message: String)

const DEFAULT_API_PORT := 8081
const DEFAULT_PG_PORT := 5439
const DEFAULT_UDP_PORT := 20777
const HEALTH_POLL_INTERVAL := 0.5
const HEALTH_TIMEOUT_S := 30.0
const SETTINGS_PATH := "user://settings.cfg"

var base_url := "http://127.0.0.1:%d" % DEFAULT_API_PORT

var _pid := -1
var _shutting_down := false
var _engine_is_ready := false
var _listener_running := false
var _listener_port := DEFAULT_UDP_PORT
var _listener_busy := false

var _health_http: HTTPRequest
var _listener_http: HTTPRequest
var _shutdown_http: HTTPRequest
var _health_timer: Timer
var _health_elapsed := 0.0

# --- overlay (blocks interaction until the engine is healthy) ---
var _overlay: CanvasLayer
var _overlay_status: Label
var _overlay_retry_btn: Button


func _ready() -> void:
	# Without this, Godot quits immediately on the OS close signal before
	# _notification below gets a chance to shut the engine process down
	# gracefully — see _begin_shutdown.
	get_tree().set_auto_accept_quit(false)

	_load_settings()
	_build_overlay()

	_health_http = HTTPRequest.new()
	add_child(_health_http)
	_health_http.request_completed.connect(_on_health_completed)

	_listener_http = HTTPRequest.new()
	add_child(_listener_http)
	_listener_http.request_completed.connect(_on_listener_completed)

	_shutdown_http = HTTPRequest.new()
	add_child(_shutdown_http)

	_launch_engine()


func _notification(what: int) -> void:
	if what == NOTIFICATION_WM_CLOSE_REQUEST:
		if _shutting_down:
			return
		_shutting_down = true
		_begin_shutdown()


# ---------------------------------------------------------------- launching

func _engine_exe_path() -> String:
	var dir := OS.get_executable_path().get_base_dir()
	var candidate := dir.path_join("engine.exe" if OS.get_name() == "Windows" else "engine")
	if FileAccess.file_exists(candidate):
		return candidate
	if OS.has_feature("editor"):
		# Dev convenience: running from the Godot editor, exe lives in the
		# separate Go project's build output rather than next to Godot.
		var dev_candidate := "C:/Users/ychri/Documents/TelemetryApp/engine/dist/engine.exe"
		if FileAccess.file_exists(dev_candidate):
			return dev_candidate
	return ""


func _launch_engine() -> void:
	var exe := _engine_exe_path()
	if exe == "":
		_overlay_status.text = "Couldn't find engine.exe next to the application.\nReinstall or contact support."
		_overlay_retry_btn.hide()
		engine_failed.emit("engine.exe not found")
		return

	var data_dir := OS.get_user_data_dir().path_join("enginedata")
	var args := [
		"--api-port", str(DEFAULT_API_PORT),
		"--pg-port", str(DEFAULT_PG_PORT),
		"--data-dir", data_dir,
	]
	var pid := OS.create_process(exe, args, false)
	if pid <= 0:
		_overlay_status.text = "Failed to start the local server process."
		engine_failed.emit("create_process failed")
		return

	_pid = pid
	_overlay_status.text = "Starting local server..."
	_health_elapsed = 0.0
	_poll_health()


func _poll_health() -> void:
	if _engine_is_ready:
		return
	if _health_elapsed >= HEALTH_TIMEOUT_S:
		_overlay_status.text = "Local server didn't respond in time."
		_overlay_retry_btn.show()
		engine_failed.emit("health timeout")
		return
	_health_http.request(base_url + "/healthz")


func _on_health_completed(result: int, code: int, _headers: PackedStringArray, _body: PackedByteArray) -> void:
	if _engine_is_ready:
		return
	if result == HTTPRequest.RESULT_SUCCESS and code == 200:
		_engine_is_ready = true
		_overlay.hide()
		engine_ready.emit()
		return
	_health_elapsed += HEALTH_POLL_INTERVAL
	await get_tree().create_timer(HEALTH_POLL_INTERVAL).timeout
	_poll_health()


func _on_retry_pressed() -> void:
	_overlay_retry_btn.hide()
	_overlay_status.text = "Starting local server..."
	if _pid > 0 and OS.is_process_running(_pid):
		# a previous attempt's process is still around — leave it, just
		# resume health-polling instead of spawning a second one.
		_health_elapsed = 0.0
		_poll_health()
	else:
		_launch_engine()


# --------------------------------------------------------------- listener

# Public API for whichever screen hosts the Play/Stop control (currently
# main_menu.gd's top bar). Ignored while the previous start/stop is still in
# flight or the engine isn't up yet — callers should disable their button on
# listener_status_changed/listener_error instead of guarding this themselves.
func start_listener(port: int) -> void:
	if not _engine_is_ready or _listener_busy:
		return
	_listener_busy = true
	_listener_http.request(base_url + "/listener/start?port=%d" % port, [], HTTPClient.METHOD_POST)


func stop_listener() -> void:
	if not _engine_is_ready or _listener_busy:
		return
	_listener_busy = true
	_listener_http.request(base_url + "/listener/stop", [], HTTPClient.METHOD_POST)


func is_listener_running() -> bool:
	return _listener_running


func get_listener_port() -> int:
	return _listener_port


func _on_listener_completed(result: int, code: int, _headers: PackedStringArray, body: PackedByteArray) -> void:
	_listener_busy = false
	if result != HTTPRequest.RESULT_SUCCESS:
		listener_error.emit("request failed")
		return
	var parsed = JSON.parse_string(body.get_string_from_utf8())
	if code != 200 or not (parsed is Dictionary):
		var msg := body.get_string_from_utf8().strip_edges()
		listener_error.emit(msg if msg != "" else "HTTP %d" % code)
		return
	_apply_listener_status(parsed.get("running", false), int(parsed.get("port", 0)))


func _apply_listener_status(running: bool, port: int) -> void:
	_listener_running = running
	if running and port > 0:
		_listener_port = port
		_save_settings()
	listener_status_changed.emit(running, _listener_port)


# ----------------------------------------------------------------- shutdown

func _begin_shutdown() -> void:
	if _overlay:
		_overlay.show()
		_overlay_retry_btn.hide()
		_overlay_status.text = "Closing..."
	if _pid <= 0 or not _engine_is_ready:
		_finish_shutdown()
		return
	_shutdown_http.request_completed.connect(func(_r, _c, _h, _b): _wait_for_exit(), CONNECT_ONE_SHOT)
	if _shutdown_http.request(base_url + "/shutdown", [], HTTPClient.METHOD_POST) != OK:
		_wait_for_exit()


func _wait_for_exit() -> void:
	var elapsed := 0.0
	while _pid > 0 and OS.is_process_running(_pid) and elapsed < 8.0:
		await get_tree().create_timer(0.2).timeout
		elapsed += 0.2
	if _pid > 0 and OS.is_process_running(_pid):
		OS.kill(_pid)
	_finish_shutdown()


func _finish_shutdown() -> void:
	get_tree().quit()


# ------------------------------------------------------------------ settings

func _load_settings() -> void:
	var cfg := ConfigFile.new()
	if cfg.load(SETTINGS_PATH) == OK:
		_listener_port = cfg.get_value("listener", "port", DEFAULT_UDP_PORT)


func _save_settings() -> void:
	var cfg := ConfigFile.new()
	cfg.load(SETTINGS_PATH) # best-effort — start blank if it doesn't exist yet
	cfg.set_value("listener", "port", _listener_port)
	cfg.save(SETTINGS_PATH)


# ------------------------------------------------------------------------ UI

func _build_overlay() -> void:
	_overlay = CanvasLayer.new()
	_overlay.layer = 100
	_overlay.process_mode = Node.PROCESS_MODE_ALWAYS
	add_child(_overlay)

	var bg := ColorRect.new()
	bg.color = Color(0, 0, 0, 0.85)
	bg.set_anchors_preset(Control.PRESET_FULL_RECT)
	bg.mouse_filter = Control.MOUSE_FILTER_STOP
	_overlay.add_child(bg)

	var center := CenterContainer.new()
	center.set_anchors_preset(Control.PRESET_FULL_RECT)
	_overlay.add_child(center)

	var vbox := VBoxContainer.new()
	vbox.alignment = BoxContainer.ALIGNMENT_CENTER
	center.add_child(vbox)

	_overlay_status = Label.new()
	_overlay_status.text = "Starting local server..."
	_overlay_status.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_overlay_status.add_theme_font_size_override("font_size", 20)
	vbox.add_child(_overlay_status)

	_overlay_retry_btn = Button.new()
	_overlay_retry_btn.text = "Retry"
	_overlay_retry_btn.hide()
	_overlay_retry_btn.pressed.connect(_on_retry_pressed)
	vbox.add_child(_overlay_retry_btn)
