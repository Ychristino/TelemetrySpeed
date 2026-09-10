class_name ChartCanvas
extends Control

const LEFT_MARGIN   := 44.0
const RIGHT_MARGIN  := 18.0
const TOP_MARGIN    := 16.0
const BOTTOM_MARGIN := 20.0
const X_TICK_COUNT  := 5
const Y_TICK_COUNT  := 5

# Pseudo-field for the time-delta metric. Unlike every other field, its
# points aren't decoded straight from one series' own raw blob — they're
# computed against a second, pinned "reference" raw blob (see
# add_delta_series/rebase_delta_series and _redecode below).
const DELTA_FIELD := "delta"

@onready var _lines_root: Control           = $lines_root
@onready var _placeholder: Label            = $placeholder_label
@onready var _x_ticks_root: Control         = $x_ticks
@onready var _y_ticks_root: Control         = $y_ticks
@onready var _cursor_line: Line2D           = $cursor_line
@onready var _cursor_readout: PanelContainer = $cursor_readout
@onready var _cursor_readout_list: VBoxContainer = $cursor_readout/cursor_readout_list

var _x_mode := "distance"  # "distance" | "time"
var _active_field := ""

# field → { y_min, y_max, auto_y, series: { key → {raw, points, color, line} } }
var _metrics: Dictionary = {}

var _min_x := 0.0
var _max_x := 1.0
var _x_auto := true
# Full extent of the actual data — the current view (_min_x/_max_x) is
# always clamped inside this, so pan/zoom can't scroll past real data.
var _data_min_x := 0.0
var _data_max_x := 1.0

var _panning_y := false
var _pan_y_start_mouse := 0.0
var _pan_y_start_min := 0.0
var _pan_y_start_max := 0.0

func _ready() -> void:
	resized.connect(_redraw)
	mouse_exited.connect(_hide_cursor)

# Drops bubble here too (the canvas covers most of the pane's area).
func _can_drop_data(_at_position: Vector2, data: Variant) -> bool:
	return typeof(data) == TYPE_DICTIONARY and data.get("type") == "metric"

func _drop_data(_at_position: Vector2, data: Variant) -> void:
	# owner (not get_parent()) — canvas sits inside a "content" wrapper, not
	# directly under ChartPane, but owner always points to the scene root.
	var pane := owner
	if pane and pane.has_signal("metric_dropped"):
		pane.metric_dropped.emit(data["field"], data["label"])

# Wheel behavior is purely modifier-driven now (no zone dependence):
# Alt+wheel = zoom the active metric's Y range, Ctrl+wheel = zoom X,
# plain wheel = pan X. Left-drag pans the active metric's Y range anywhere
# on the canvas, including over the Y-axis tick column.
func _gui_input(event: InputEvent) -> void:
	if event is InputEventMouseButton:
		_handle_mouse_button(event)
	elif event is InputEventMouseMotion:
		_update_cursor(event.position)
		if _panning_y:
			_handle_y_drag(event)

func _handle_mouse_button(event: InputEventMouseButton) -> void:
	if event.button_index == MOUSE_BUTTON_LEFT:
		if event.pressed:
			_panning_y = true
			_pan_y_start_mouse = event.position.y
			if _active_field != "" and _metrics.has(_active_field):
				var m: Dictionary = _metrics[_active_field]
				_pan_y_start_min = m["y_min"]
				_pan_y_start_max = m["y_max"]
		else:
			_panning_y = false
		return

	if not event.pressed:
		return

	var zoom_in := event.button_index == MOUSE_BUTTON_WHEEL_UP
	var zoom_out := event.button_index == MOUSE_BUTTON_WHEEL_DOWN
	if not (zoom_in or zoom_out):
		return

	if event.alt_pressed:
		_zoom_active_y(zoom_in)
	elif event.ctrl_pressed:
		_zoom_x(zoom_in, event.position.x)
	else:
		_pan_x(-1 if zoom_in else 1)

func _handle_y_drag(event: InputEventMouseMotion) -> void:
	if _active_field == "" or not _metrics.has(_active_field):
		return
	var m: Dictionary = _metrics[_active_field]
	var rect_h: float = max(size.y - BOTTOM_MARGIN - TOP_MARGIN, 1.0)
	var span: float = _pan_y_start_max - _pan_y_start_min
	var dy_val: float = (event.position.y - _pan_y_start_mouse) / rect_h * span
	m["y_min"] = _pan_y_start_min + dy_val
	m["y_max"] = _pan_y_start_max + dy_val
	m["auto_y"] = false
	_redraw()

# Rescales the active metric's value range, centered on its current midpoint —
# the "redimension that Y axis" control.
func _zoom_active_y(zoom_in: bool) -> void:
	if _active_field == "" or not _metrics.has(_active_field):
		return
	var m: Dictionary = _metrics[_active_field]
	var factor: float = 0.85 if zoom_in else (1.0 / 0.85)
	var mid: float  = (m["y_min"] + m["y_max"]) * 0.5
	var half: float = (m["y_max"] - m["y_min"]) * 0.5 * factor
	m["y_min"] = mid - half
	m["y_max"] = mid + half
	m["auto_y"] = false
	_redraw()

# Zooms the shared X range toward the cursor, like a normal chart zoom.
func _zoom_x(zoom_in: bool, mouse_x: float) -> void:
	var rect_w: float = max(size.x - LEFT_MARGIN - RIGHT_MARGIN, 1.0)
	var cursor_frac: float = clamp((mouse_x - LEFT_MARGIN) / rect_w, 0.0, 1.0)
	var cursor_val: float = _min_x + cursor_frac * (_max_x - _min_x)
	var factor: float = 0.85 if zoom_in else (1.0 / 0.85)
	var new_span: float = max((_max_x - _min_x) * factor, 0.001)
	_min_x = cursor_val - cursor_frac * new_span
	_max_x = _min_x + new_span
	_x_auto = false
	_clamp_x_view()
	_redraw()

# Steps the shared X range left/right by a fraction of the current span.
func _pan_x(direction: int) -> void:
	var delta: float = (_max_x - _min_x) * 0.1 * float(direction)
	_min_x += delta
	_max_x += delta
	_x_auto = false
	_clamp_x_view()
	_redraw()

# Keeps the current view inside the actual data's full extent — pan/zoom
# can never scroll past the real left/right boundaries.
func _clamp_x_view() -> void:
	var span: float = _max_x - _min_x
	var data_span: float = _data_max_x - _data_min_x
	if data_span <= 0.0:
		return
	if span >= data_span:
		_min_x = _data_min_x
		_max_x = _data_max_x
		return
	if _min_x < _data_min_x:
		_min_x = _data_min_x
		_max_x = _min_x + span
	elif _max_x > _data_max_x:
		_max_x = _data_max_x
		_min_x = _max_x - span

# ── Hover cursor: vertical line + nearest-point readout ──────────────────────

func _update_cursor(mouse_pos: Vector2) -> void:
	var rect := Rect2(LEFT_MARGIN, TOP_MARGIN, max(size.x - LEFT_MARGIN - RIGHT_MARGIN, 1.0), max(size.y - BOTTOM_MARGIN - TOP_MARGIN, 1.0))
	if _metrics.is_empty() or mouse_pos.x < rect.position.x or mouse_pos.x > rect.position.x + rect.size.x:
		_hide_cursor()
		return

	_cursor_line.visible = true
	_cursor_line.clear_points()
	_cursor_line.add_point(Vector2(mouse_pos.x, rect.position.y))
	_cursor_line.add_point(Vector2(mouse_pos.x, rect.position.y + rect.size.y))

	var dx := _max_x - _min_x
	if dx <= 0.0: dx = 1.0
	var target_x: float = _min_x + (mouse_pos.x - rect.position.x) / rect.size.x * dx

	for c in _cursor_readout_list.get_children():
		_cursor_readout_list.remove_child(c)
		c.free()
	for field in _metrics:
		var m: Dictionary = _metrics[field]
		var series: Dictionary = m["series"]
		if series.is_empty():
			continue
		var header := Label.new()
		header.text = String(m.get("label", field))
		header.add_theme_font_size_override("font_size", 10)
		header.modulate = Color(1, 1, 1, 0.85)
		_cursor_readout_list.add_child(header)
		for key in series:
			var s: Dictionary = series[key]
			var val: float = _nearest_value(s["points"], target_x)
			var row := Label.new()
			row.text = "  %s: %.2f" % [_driver_label_from_key(key), val]
			row.add_theme_font_size_override("font_size", 10)
			row.modulate = (s["line"] as Line2D).default_color
			_cursor_readout_list.add_child(row)

	_position_readout(mouse_pos.x, rect)

func _hide_cursor() -> void:
	_cursor_line.visible = false
	_cursor_readout.visible = false

# Places the readout to the right of the cursor line, flipping to the left
# if it would otherwise run off the pane's edge.
func _position_readout(x_px: float, rect: Rect2) -> void:
	_cursor_readout.visible = true
	var w: float = _cursor_readout.get_combined_minimum_size().x
	var h: float = _cursor_readout.get_combined_minimum_size().y
	var target_x: float = x_px + 8.0
	if target_x + w > size.x:
		target_x = x_px - w - 8.0
	_cursor_readout.position = Vector2(clamp(target_x, rect.position.x, max(size.x - w, rect.position.x)), 4.0)
	_cursor_readout.size = Vector2(w, h)

func _nearest_value(points: Array, target_x: float) -> float:
	if points.is_empty():
		return 0.0
	var best_i := 0
	var best_d: float = abs(float(points[0].x) - target_x)
	for i in range(1, points.size()):
		var d: float = abs(float(points[i].x) - target_x)
		if d < best_d:
			best_d = d
			best_i = i
	return points[best_i].y

func _driver_label_from_key(key: String) -> String:
	var parts := key.split("#")
	if parts.size() < 2:
		return key
	return "%s L%s" % [parts[0], parts[1]]

# ── Metric / series management ───────────────────────────────────────────────

func has_metric(field: String) -> bool:
	return _metrics.has(field)

func add_metric(field: String, label: String = "") -> void:
	if _metrics.has(field):
		return
	_metrics[field] = {
		label = label if label != "" else field,
		y_min = 0.0, y_max = 1.0,
		data_y_min = 0.0, data_y_max = 1.0,
		auto_y = true, series = {}
	}

func remove_metric(field: String) -> void:
	if not _metrics.has(field):
		return
	for s in (_metrics[field]["series"] as Dictionary).values():
		(s["line"] as Line2D).queue_free()
	_metrics.erase(field)
	if _active_field == field:
		_active_field = ""
	_recompute_data_x_domain()
	_redraw()

func set_active_metric(field: String) -> void:
	_active_field = field
	_redraw()

# Undoes any zoom/pan: fits X back to all data, and every metric's Y range
# back to auto-fit — the escape hatch when a scroll or drag left the view
# somewhere confusing.
func reset_view() -> void:
	_x_auto = true
	_recompute_data_x_domain()
	for field in _metrics:
		_metrics[field]["auto_y"] = true
		_recompute_y_domain(field)
	_redraw()

func set_x_mode(mode: String) -> void:
	if _x_mode == mode:
		return
	_x_mode = mode
	for field in _metrics:
		for key in (_metrics[field]["series"] as Dictionary):
			_redecode(field, key)
		_recompute_y_domain(field)
	_recompute_data_x_domain()
	_redraw()

# raw_data is the full decoded telemetry blob for one driver/lap.
func add_series(field: String, key: String, color: Color, raw_data: PackedByteArray) -> void:
	if not _metrics.has(field):
		add_metric(field)
	var m: Dictionary = _metrics[field]
	if (m["series"] as Dictionary).has(key):
		return
	var line := Line2D.new()
	line.width = 2.0
	line.default_color = color
	_lines_root.add_child(line)
	m["series"][key] = {raw = raw_data, points = [], line = line}
	_redecode(field, key)
	_recompute_y_domain(field)
	_recompute_data_x_domain()
	_redraw()

# Like add_series, but for a delta line: raw_data is the compared lap and
# reference_raw is the pinned reference lap — see TelemetryDecoder's
# to_delta_chart_points for what gets plotted. Adds the "Time Delta" metric
# box itself if this is the first delta series in this pane.
func add_delta_series(key: String, color: Color, raw_data: PackedByteArray, reference_raw: PackedByteArray) -> void:
	if not _metrics.has(DELTA_FIELD):
		add_metric(DELTA_FIELD, "Time Delta")
	var m: Dictionary = _metrics[DELTA_FIELD]
	if (m["series"] as Dictionary).has(key):
		return
	var line := Line2D.new()
	line.width = 2.0
	line.default_color = color
	_lines_root.add_child(line)
	m["series"][key] = {raw = raw_data, reference_raw = reference_raw, points = [], line = line}
	_redecode(DELTA_FIELD, key)
	_recompute_y_domain(DELTA_FIELD)
	_recompute_data_x_domain()
	_redraw()

# Re-points every delta series at a new reference lap (the user pinned a
# different one) and redraws.
func rebase_delta_series(reference_raw: PackedByteArray) -> void:
	if not _metrics.has(DELTA_FIELD):
		return
	var series: Dictionary = _metrics[DELTA_FIELD]["series"]
	for key in series:
		series[key]["reference_raw"] = reference_raw
		_redecode(DELTA_FIELD, key)
	_recompute_y_domain(DELTA_FIELD)
	_recompute_data_x_domain()
	_redraw()

# Removes every delta series (but keeps the "Time Delta" box itself) — used
# when the reference lap is unpinned, since every existing delta line is now
# measured against nothing.
func clear_delta_series() -> void:
	if not _metrics.has(DELTA_FIELD):
		return
	var series: Dictionary = _metrics[DELTA_FIELD]["series"]
	for key in series.keys():
		(series[key]["line"] as Line2D).queue_free()
	series.clear()
	_recompute_data_x_domain()
	_redraw()

# An empty HTTP-format blob (TelemetryDecoder.HTTP_HEADER_SIZE-byte header,
# row_count = 0) — the starting point for a live series, which grows via
# append_series_rows as frames arrive instead of being loaded all at once.
# The field_id bytes are left zero-filled — to_chart_points never inspects
# them, since this client always decodes TelemetryDecoder.DEFAULT_FIELD_NAMES
# regardless of what the header claims.
static func empty_raw_blob() -> PackedByteArray:
	var buf := PackedByteArray()
	buf.resize(TelemetryDecoder.HTTP_HEADER_SIZE)
	buf[0] = 0x54
	buf[1] = 0x4D
	buf[2] = 2
	buf.encode_u32(TelemetryDecoder.HTTP_HEADER_SIZE - 4, 0)
	return buf

# Appends newly-arrived row bytes (raw TelemetryDecoder.ROW_SIZE-byte rows,
# no header) to a live series' buffer and redecodes — used for streaming, so
# points accumulate
# without needing to re-fetch everything already plotted.
func append_series_rows(field: String, key: String, row_bytes: PackedByteArray) -> void:
	if row_bytes.is_empty() or not _metrics.has(field):
		return
	var series: Dictionary = _metrics[field]["series"]
	if not series.has(key):
		return
	var s: Dictionary = series[key]
	var raw: PackedByteArray = s["raw"]
	var old_count: int = raw.decode_u32(4)
	var added: int = row_bytes.size() / TelemetryDecoder.ROW_SIZE
	raw.append_array(row_bytes)
	raw.encode_u32(4, old_count + added)
	s["raw"] = raw
	_redecode(field, key)
	_recompute_y_domain(field)
	_recompute_data_x_domain()
	_redraw()

# Resets a live series back to empty — used on lap rollover, so the chart
# starts fresh for the new lap without needing to be re-added.
func clear_series_data(field: String, key: String) -> void:
	if not _metrics.has(field):
		return
	var series: Dictionary = _metrics[field]["series"]
	if not series.has(key):
		return
	series[key]["raw"] = empty_raw_blob()
	_redecode(field, key)
	_recompute_y_domain(field)
	_recompute_data_x_domain()
	_redraw()

# Updates one series' plotted line color to match its legend swatch.
func set_series_color(field: String, key: String, color: Color) -> void:
	if not _metrics.has(field):
		return
	var series: Dictionary = _metrics[field]["series"]
	if not series.has(key):
		return
	(series[key]["line"] as Line2D).default_color = color

# Shows/hides `key`'s plotted line for `field` — the data stays loaded and
# _redraw() keeps updating its points, it just doesn't render while hidden.
func set_series_visible(field: String, key: String, is_visible: bool) -> void:
	if not _metrics.has(field):
		return
	var series: Dictionary = _metrics[field]["series"]
	if not series.has(key):
		return
	(series[key]["line"] as Line2D).visible = is_visible

func remove_series(field: String, key: String) -> void:
	if not _metrics.has(field):
		return
	var series: Dictionary = _metrics[field]["series"]
	if not series.has(key):
		return
	(series[key]["line"] as Line2D).queue_free()
	series.erase(key)
	_recompute_data_x_domain()
	_redraw()

func remove_series_everywhere(key: String) -> void:
	for field in _metrics.keys():
		remove_series(field, key)

func _redecode(field: String, key: String) -> void:
	var s: Dictionary = _metrics[field]["series"][key]
	if field == DELTA_FIELD:
		s["points"] = TelemetryDecoder.to_delta_chart_points(s["raw"], s["reference_raw"], _x_mode)
	else:
		s["points"] = TelemetryDecoder.to_chart_points(s["raw"], field, _x_mode)

# Recomputes the actual data's full X extent, then either snaps the current
# view to it (if still in auto mode) or just re-clamps the view inside it
# (if the user has manually panned/zoomed).
func _recompute_data_x_domain() -> void:
	var first := true
	for field in _metrics:
		for key in (_metrics[field]["series"] as Dictionary):
			for pt in _metrics[field]["series"][key]["points"]:
				if first:
					_data_min_x = pt.x; _data_max_x = pt.x; first = false
				else:
					if pt.x < _data_min_x: _data_min_x = pt.x
					if pt.x > _data_max_x: _data_max_x = pt.x
	if _data_max_x <= _data_min_x:
		_data_max_x = _data_min_x + 1.0
	if _x_auto:
		_min_x = _data_min_x
		_max_x = _data_max_x
	else:
		_clamp_x_view()

# Recomputes this metric's actual data extent (data_y_min/max — used for
# where ticks land), and snaps the current view (y_min/max) to it only while
# still in auto mode (i.e. before the user has zoomed/panned this metric).
func _recompute_y_domain(field: String) -> void:
	var m: Dictionary = _metrics[field]
	var first := true
	var y_min := 0.0
	var y_max := 1.0
	for key in (m["series"] as Dictionary):
		for pt in m["series"][key]["points"]:
			if first:
				y_min = pt.y; y_max = pt.y; first = false
			else:
				if pt.y < y_min: y_min = pt.y
				if pt.y > y_max: y_max = pt.y
	if y_max <= y_min:
		y_max = y_min + 1.0
	m["data_y_min"] = y_min
	m["data_y_max"] = y_max
	if m["auto_y"]:
		m["y_min"] = y_min
		m["y_max"] = y_max

# ── Rendering ────────────────────────────────────────────────────────────────

func _redraw() -> void:
	_placeholder.visible = _metrics.is_empty()
	var rect := Rect2(LEFT_MARGIN, TOP_MARGIN, max(size.x - LEFT_MARGIN - RIGHT_MARGIN, 1.0), max(size.y - BOTTOM_MARGIN - TOP_MARGIN, 1.0))

	# lines_root clips to exactly the plot rect — without it, a series' points
	# that fall outside the current (possibly zoomed-in) X view still get
	# mapped to real pixel coordinates by _map_point, just ones that land left
	# of LEFT_MARGIN/right of the right edge, so the line would otherwise
	# render straight through the Y-axis divider and tick labels instead of
	# stopping at the plot area's edge.
	_lines_root.position = rect.position
	_lines_root.size = rect.size

	for field in _metrics:
		var m: Dictionary = _metrics[field]
		for key in (m["series"] as Dictionary):
			var s: Dictionary = m["series"][key]
			var line: Line2D = s["line"]
			line.clear_points()
			for pt in s["points"]:
				line.add_point(_map_point(pt, rect, m["y_min"], m["y_max"]) - rect.position)
	_build_x_ticks(rect)
	_build_y_ticks(rect)

func _map_point(pt: Dictionary, rect: Rect2, y_min: float, y_max: float) -> Vector2:
	var dx := _max_x - _min_x
	if dx <= 0.0: dx = 1.0
	var px: float = rect.position.x + (pt.x - _min_x) / dx * rect.size.x
	var py: float = _map_y(pt.y, rect, y_min, y_max)
	return Vector2(px, py)

func _map_y(value: float, rect: Rect2, y_min: float, y_max: float) -> float:
	var dy := y_max - y_min
	if dy <= 0.0: dy = 1.0
	return rect.position.y + rect.size.y - (value - y_min) / dy * rect.size.y

# X ticks are always shown but kept unobtrusive (small, low opacity) — they're
# just a shared reference between however many metrics are overlaid here.
func _build_x_ticks(rect: Rect2) -> void:
	for c in _x_ticks_root.get_children():
		c.queue_free()
	for i in X_TICK_COUNT:
		var t: float = _min_x + (_max_x - _min_x) * float(i) / float(X_TICK_COUNT - 1)
		var lbl := Label.new()
		lbl.text = _format_x(t)
		lbl.add_theme_font_size_override("font_size", 8)
		lbl.modulate = Color(1, 1, 1, 0.35)
		lbl.position = Vector2(
			rect.position.x + rect.size.x * float(i) / float(X_TICK_COUNT - 1) - 12.0,
			rect.position.y + rect.size.y + 1.0
		)
		_x_ticks_root.add_child(lbl)

# Y ticks only show for the active metric, and are spaced across its actual
# data extent (not the current, possibly zoomed-out, view window) — so when
# you zoom out with Alt+wheel, the ticks shrink toward wherever the line
# actually renders instead of still spanning the whole axis.
func _build_y_ticks(rect: Rect2) -> void:
	for c in _y_ticks_root.get_children():
		c.queue_free()
	if _active_field == "" or not _metrics.has(_active_field):
		return
	var m: Dictionary = _metrics[_active_field]
	var data_min: float = m["data_y_min"]
	var data_max: float = m["data_y_max"]
	for i in Y_TICK_COUNT:
		var t: float = data_min + (data_max - data_min) * float(i) / float(Y_TICK_COUNT - 1)
		var y: float = _map_y(t, rect, m["y_min"], m["y_max"])
		if y < rect.position.y - 1.0 or y > rect.position.y + rect.size.y + 1.0:
			continue  # panned/zoomed past this tick — skip it rather than draw off-strip
		var lbl := Label.new()
		lbl.text = "%.1f" % t
		lbl.add_theme_font_size_override("font_size", 9)
		lbl.modulate = Color(1, 1, 1, 0.75)
		lbl.position = Vector2(2.0, y - 6.0)
		_y_ticks_root.add_child(lbl)

func _format_x(v: float) -> String:
	if _x_mode == "time":
		return "%.1fs" % v
	return "%.0fm" % v
