extends Node
# Autoload (see project.godot [autoload]). Checks GitHub Releases for a
# newer tagged version than the one baked into this build (res://version.txt,
# written by the release workflow from the git tag — see
# .github/workflows/release.yml), and if found, can silently download +
# launch the new installer, then quit so it can replace this running build.
#
# This is "auto-update" via re-running the Inno Setup installer silently
# (/VERYSILENT /SUPPRESSMSGBOXES /NORESTART) rather than a bespoke patcher —
# Inno Setup's CloseApplications setting handles closing this process during
# install, and the installer itself IS the update mechanism we already have
# for fresh installs, so there's only one packaging path to maintain.

signal update_available(version: String, download_url: String)
signal check_failed(reason: String)

const REPO := "Ychristino/TelemetrySpeed"
const RELEASES_URL := "https://api.github.com/repos/%s/releases/latest" % REPO

var current_version := "0.0.0"
var _http: HTTPRequest
var _latest_version := ""
var _latest_download_url := ""

func _ready() -> void:
	var f := FileAccess.open("res://version.txt", FileAccess.READ)
	if f != null:
		current_version = f.get_as_text().strip_edges()
		f.close()

	_http = HTTPRequest.new()
	add_child(_http)
	_http.request_completed.connect(_on_request_completed)

func check_now() -> void:
	var err := _http.request(RELEASES_URL, ["Accept: application/vnd.github+json", "User-Agent: TelemetrySpeed-UpdateChecker"])
	if err != OK:
		check_failed.emit("could not start update check request")

func _on_request_completed(result: int, code: int, _headers: PackedStringArray, body: PackedByteArray) -> void:
	if result != HTTPRequest.RESULT_SUCCESS or code != 200:
		check_failed.emit("HTTP %d" % code)
		return
	var parsed = JSON.parse_string(body.get_string_from_utf8())
	if not (parsed is Dictionary) or not parsed.has("tag_name"):
		check_failed.emit("unexpected response")
		return

	var tag := String(parsed["tag_name"])
	var remote_version := tag.trim_prefix("v")

	var download_url := ""
	for asset in parsed.get("assets", []):
		var name := String(asset.get("name", ""))
		if name.to_lower().ends_with("setup.exe"):
			download_url = String(asset.get("browser_download_url", ""))
			break

	if download_url == "":
		check_failed.emit("release has no installer asset")
		return

	if _is_newer(remote_version, current_version):
		_latest_version = remote_version
		_latest_download_url = download_url
		update_available.emit(remote_version, download_url)

# Compares two "x.y.z" version strings numerically, segment by segment —
# a plain string compare would wrongly say "0.9.0" > "0.10.0".
func _is_newer(remote: String, local: String) -> bool:
	var r := remote.split(".")
	var l := local.split(".")
	for i in range(max(r.size(), l.size())):
		var rv := int(r[i]) if i < r.size() else 0
		var lv := int(l[i]) if i < l.size() else 0
		if rv != lv:
			return rv > lv
	return false

# Downloads the new installer to a temp file and launches it silently, then
# quits this process so the installer (CloseApplications=yes in the .iss)
# can safely replace its files. Caller should confirm with the user first —
# this proceeds immediately once called.
func download_and_install() -> void:
	if _latest_download_url == "":
		return
	var dl := HTTPRequest.new()
	add_child(dl)
	dl.request_completed.connect(func(result: int, code: int, _h: PackedStringArray, body: PackedByteArray):
		if result != HTTPRequest.RESULT_SUCCESS or code != 200:
			check_failed.emit("download failed: HTTP %d" % code)
			dl.queue_free()
			return
		var path := OS.get_cache_dir().path_join("TelemetrySpeedSetup-%s.exe" % _latest_version)
		var f := FileAccess.open(path, FileAccess.WRITE)
		if f == null:
			check_failed.emit("could not write installer to %s" % path)
			dl.queue_free()
			return
		f.store_buffer(body)
		f.close()
		dl.queue_free()
		_launch_installer(path)
	)
	dl.request(_latest_download_url)

# A freshly-written .exe can be transiently locked for a moment by Windows
# Defender's on-write scan, which makes CreateProcess fail right after we
# finish writing a large downloaded binary. create_process's return value
# was never checked before, so that failure was silent: the app just quit
# anyway with no installer running and nothing to show for it — exactly what
# an update that "does nothing" looks like. Retry a few times before giving
# up, and only quit once the installer actually launched.
func _launch_installer(path: String, attempt: int = 1) -> void:
	var pid := OS.create_process(path, ["/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART"], false)
	if pid > 0:
		EngineProcess.shutdown_and_quit()
		return
	if attempt >= 5:
		check_failed.emit("could not launch installer after %d attempts" % attempt)
		return
	await get_tree().create_timer(0.5).timeout
	_launch_installer(path, attempt + 1)
