class_name TelemetryAPIClient
extends Node

signal sessions_received(sessions: Array)
signal session_received(detail: Dictionary)
signal participants_received(participants: Array)
signal lap_times_received(laps: Array)
signal ideal_lap_info_received(info: Dictionary)
signal live_drivers_received(drivers: Array)
signal telemetry_received(data: PackedByteArray)
signal stream_frame(data: PackedByteArray)
signal stream_fields_received(fields: Array)
signal request_error(msg: String)
signal export_data_received(data: PackedByteArray)
signal export_error(msg: String)
signal import_result_received(summary: Dictionary)
signal import_error(msg: String)

@export var base_url := "http://localhost:8081"

var _http: HTTPRequest
var _pending := ""
var _busy := false
var _queue: Array = []

var _ws: WebSocketPeer
var _ws_active := false
var _ws_filter := {}
var _ws_filter_sent := false

# Export/import get their own HTTPRequest nodes rather than going through
# the shared _http queue above — they're rare, explicit, user-triggered
# one-shot actions (not part of the steady polling/fetch flow), and import
# needs a raw POST body, which the queue's GET-only _dispatch doesn't support.
var _export_http: HTTPRequest
var _import_http: HTTPRequest

func _ready() -> void:
	_http = HTTPRequest.new()
	add_child(_http)
	_http.request_completed.connect(_on_http_completed)

	_export_http = HTTPRequest.new()
	add_child(_export_http)
	_export_http.request_completed.connect(_on_export_completed)

	_import_http = HTTPRequest.new()
	add_child(_import_http)
	_import_http.request_completed.connect(_on_import_completed)

func fetch_sessions(game := "", track := "") -> void:
	var url := base_url + "/sessions"
	var params: PackedStringArray
	if game != "":
		params.append("game=" + game.uri_encode())
	if track != "":
		params.append("track=" + track.uri_encode())
	if params.size() > 0:
		url += "?" + "&".join(params)
	_enqueue("sessions", url)

func fetch_session(session_id: String) -> void:
	_enqueue("session", base_url + "/sessions/" + session_id)

func fetch_participants(session_id: String) -> void:
	_enqueue("participants", base_url + "/sessions/" + session_id + "/participants")

# Aggregated across every session ever recorded for this game+track — not
# just the latest — since a track gets a new session each time recording
# restarts. Each entry carries its own session_id for fetch_telemetry.
func fetch_lap_times(game: String, track: String) -> void:
	_enqueue("laptimes", "%s/laptimes?game=%s&track=%s" % [base_url, game.uri_encode(), track.uri_encode()])

func fetch_live_drivers(game: String, track: String) -> void:
	_enqueue("live", "%s/live?game=%s&track=%s" % [base_url, game.uri_encode(), track.uri_encode()])

# The theoretical best lap for this game+track — fastest Sector 1/2/3 ever
# recorded, independently of driver/lap. Returns {} (via ideal_lap_info_received)
# when there isn't enough data yet for all three sectors.
func fetch_ideal_lap_info(game: String, track: String) -> void:
	_enqueue("idealap", "%s/idealap?game=%s&track=%s" % [base_url, game.uri_encode(), track.uri_encode()])

# Fetches the stitched ideal-lap telemetry blob — same binary shape as
# fetch_telemetry's response, so it's handled via the same telemetry_received
# signal without any special-casing on the client.
func fetch_ideal_lap_telemetry(game: String, track: String) -> void:
	var url := "%s/telemetry/ideal?game=%s&track=%s&fields=%s" % [
		base_url,
		game.uri_encode(),
		track.uri_encode(),
		",".join(TelemetryDecoder.DEFAULT_FIELD_NAMES),
	]
	_enqueue("telemetry", url)

# session_id is optional but should be passed whenever known (e.g. from a
# fetch_lap_times/fetch_live_drivers entry) — without it, the server falls
# back to "most recent session for this game+track", which is only correct
# when a track has just one session on record.
func fetch_telemetry(game: String, track: String, lap: int, driver: String, session_id: String = "") -> void:
	var url := "%s/telemetry?game=%s&track=%s&lap=%d&driver=%s&fields=%s" % [
		base_url,
		game.uri_encode(),
		track.uri_encode(),
		lap,
		driver.uri_encode(),
		",".join(TelemetryDecoder.DEFAULT_FIELD_NAMES),
	]
	if session_id != "":
		url += "&session_id=" + session_id.uri_encode()
	_enqueue("telemetry", url)

# scope is "all", "track", or "lap" — see database.ExportScope. track scope
# needs game+track; lap scope needs session_id, car_index, and lap_number.
func fetch_export(scope: String, game: String = "", track: String = "", session_id: String = "", car_index: int = -1, lap_number: int = -1) -> void:
	var params := ["scope=" + scope.uri_encode()]
	if game != "":
		params.append("game=" + game.uri_encode())
	if track != "":
		params.append("track=" + track.uri_encode())
	if session_id != "":
		params.append("session_id=" + session_id.uri_encode())
	if car_index >= 0:
		params.append("car_index=%d" % car_index)
	if lap_number >= 0:
		params.append("lap_number=%d" % lap_number)
	var err := _export_http.request(base_url + "/export?" + "&".join(params))
	if err != OK:
		export_error.emit("Failed to start export request")

func _on_export_completed(result: int, code: int, _headers: PackedStringArray, body: PackedByteArray) -> void:
	if result != HTTPRequest.RESULT_SUCCESS or code != 200:
		export_error.emit("Export failed: HTTP %d" % code)
		return
	export_data_received.emit(body)

# bytes is a .tvexport file exactly as fetch_export/handleExport produced —
# gzip-compressed JSON, sent verbatim as the request body.
func upload_import(bytes: PackedByteArray) -> void:
	var headers := ["Content-Type: application/octet-stream"]
	var err := _import_http.request_raw(base_url + "/import", headers, HTTPClient.METHOD_POST, bytes)
	if err != OK:
		import_error.emit("Failed to start import request")

func _on_import_completed(result: int, code: int, _headers: PackedStringArray, body: PackedByteArray) -> void:
	if result != HTTPRequest.RESULT_SUCCESS or code != 200:
		import_error.emit("Import failed: HTTP %d — %s" % [code, body.get_string_from_utf8()])
		return
	var parsed = JSON.parse_string(body.get_string_from_utf8())
	if parsed is Dictionary:
		import_result_received.emit(parsed)
	else:
		import_error.emit("Import: unexpected response")

func connect_stream(game: String, track: String, lap: int, driver: String, session_id: String = "") -> void:
	disconnect_stream()
	_ws = WebSocketPeer.new()
	var ws_url := base_url.replace("http://", "ws://").replace("https://", "wss://") + "/ws/stream"
	if _ws.connect_to_url(ws_url) != OK:
		request_error.emit("WebSocket: failed to connect to " + ws_url)
		_ws = null
		return
	_ws_filter = {
		game = game, track = track, lap = lap, driver = driver, session_id = session_id,
		fields = Array(TelemetryDecoder.DEFAULT_FIELD_NAMES),
	}
	_ws_filter_sent = false
	_ws_active = true

func disconnect_stream() -> void:
	if _ws != null:
		_ws.close()
	_ws = null
	_ws_active = false
	_ws_filter_sent = false

func _process(_delta: float) -> void:
	if not _ws_active or _ws == null:
		return
	_ws.poll()
	match _ws.get_ready_state():
		WebSocketPeer.STATE_OPEN:
			if not _ws_filter_sent:
				_ws_filter_sent = true
				_ws.send_text(JSON.stringify(_ws_filter))
			while _ws.get_available_packet_count() > 0:
				var packet := _ws.get_packet()
				if _ws.was_string_packet():
					_on_ws_text(packet.get_string_from_utf8())
				else:
					stream_frame.emit(packet)
		WebSocketPeer.STATE_CLOSED:
			_ws_active = false

# The server sends exactly one JSON text message before any binary frames:
# either {"fields": [...]} confirming the resolved field list/order, or
# {"error": "..."} if the filter was rejected. Everything after that on a
# given connection is a binary telemetry frame.
func _on_ws_text(text: String) -> void:
	var parsed = JSON.parse_string(text)
	if not (parsed is Dictionary):
		request_error.emit("WebSocket: unexpected text message: " + text)
		return
	if parsed.has("error"):
		request_error.emit("WebSocket: " + str(parsed["error"]))
	elif parsed.has("fields"):
		stream_fields_received.emit(parsed["fields"])

func _enqueue(pending: String, url: String) -> void:
	if _busy:
		_queue.append({pending = pending, url = url})
		return
	_dispatch(pending, url)

func _dispatch(pending: String, url: String) -> void:
	_busy = true
	_pending = pending
	_http.request(url)

func _on_http_completed(result: int, code: int, _headers: PackedStringArray, body: PackedByteArray) -> void:
	var current := _pending
	_busy = false
	if _queue.size() > 0:
		var next = _queue.pop_front()
		_dispatch(next.pending, next.url)

	if result != HTTPRequest.RESULT_SUCCESS or code != 200:
		request_error.emit("HTTP %s error: result=%d code=%d" % [current, result, code])
		return

	match current:
		"sessions":
			var parsed = JSON.parse_string(body.get_string_from_utf8())
			if parsed is Array:
				sessions_received.emit(parsed)
			else:
				request_error.emit("sessions: unexpected response")
		"session":
			var parsed = JSON.parse_string(body.get_string_from_utf8())
			if parsed is Dictionary:
				session_received.emit(parsed)
			else:
				request_error.emit("session: unexpected response")
		"participants":
			var parsed = JSON.parse_string(body.get_string_from_utf8())
			if parsed is Array:
				participants_received.emit(parsed)
			else:
				request_error.emit("participants: unexpected response")
		"laptimes":
			var parsed = JSON.parse_string(body.get_string_from_utf8())
			if parsed is Array:
				lap_times_received.emit(parsed)
			else:
				request_error.emit("laptimes: unexpected response")
		"idealap":
			var parsed = JSON.parse_string(body.get_string_from_utf8())
			if parsed is Dictionary:
				ideal_lap_info_received.emit(parsed)
			else:
				request_error.emit("idealap: unexpected response")
		"live":
			var parsed = JSON.parse_string(body.get_string_from_utf8())
			if parsed is Array:
				live_drivers_received.emit(parsed)
			else:
				request_error.emit("live: unexpected response")
		"telemetry":
			telemetry_received.emit(body)
