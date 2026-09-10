class_name ChartPane
extends Control

# Emitted when a metric button from the sidebar is dropped onto this pane.
signal metric_dropped(field: String, label: String)

const ChartPaneScene          := preload("res://chart_pane.tscn")
const SelectedDriverItemScene := preload("res://selected_driver_item.tscn")
const MetricBoxScene          := preload("res://metric_box.tscn")

@onready var _split_h_btn: Button         = $toolbar/split_h
@onready var _split_v_btn: Button         = $toolbar/split_v
@onready var _close_btn: Button           = $toolbar/close_btn
@onready var _reset_view_btn: Button      = $toolbar/reset_view_btn
@onready var _x_mode_switch: Button       = $toolbar/x_mode_switch
@onready var _legend_stack: VBoxContainer = $content/legend_stack
@onready var _canvas: ChartCanvas         = $content/canvas

# field → the MetricBox currently showing that data type in this pane.
var _metric_boxes: Dictionary[String, MetricBox] = {}
var _active_field := ""

# key → its legend row in the "Time Delta" box, if any. Tracked separately
# from other fields' legend rows because delta series can be individually
# removed and re-added as the reference lap changes (see main_menu.gd's
# _set_reference) — every other field only ever removes a key's row as part
# of removing the whole series, never on its own.
var _delta_legend_items: Dictionary[String, Control] = {}

func _ready() -> void:
	add_to_group("chart_panes")
	_split_h_btn.pressed.connect(func(): _split(false))
	_split_v_btn.pressed.connect(func(): _split(true))
	_close_btn.pressed.connect(_close)
	_reset_view_btn.pressed.connect(func(): _canvas.reset_view())
	_x_mode_switch.toggled.connect(func(is_time: bool) -> void:
		_x_mode_switch.text = "Time" if is_time else "Dist"
		_canvas.set_x_mode("time" if is_time else "distance")
	)
	_legend_stack.minimum_size_changed.connect(_update_legend_size)
	_update_close_enabled()
	_update_legend_size()

func _update_close_enabled() -> void:
	# The root pane (direct child of the workspace, not a SplitContainer) is
	# the last one left and can't be closed.
	_close_btn.disabled = not (get_parent() is SplitContainer)

# ── Drag and drop: sidebar metric buttons drop onto the pane to add them ────

func _can_drop_data(_at_position: Vector2, data: Variant) -> bool:
	return typeof(data) == TYPE_DICTIONARY and data.get("type") == "metric"

func _drop_data(_at_position: Vector2, data: Variant) -> void:
	metric_dropped.emit(data["field"], data["label"])

# ── Per-metric floating boxes, stacked top-right ─────────────────────────────

func has_metric(field: String) -> bool:
	return _metric_boxes.has(field)

func get_metric_fields() -> Array:
	return _metric_boxes.keys()

func add_metric_box(field: String, label: String) -> void:
	if _metric_boxes.has(field):
		return
	var box: MetricBox = MetricBoxScene.instantiate()
	_legend_stack.add_child(box)
	box.set_label(label)
	box.close_pressed.connect(func(): remove_metric_box(field))
	box.activated.connect(func(): _set_active_metric(field))
	_metric_boxes[field] = box
	_canvas.add_metric(field, label)
	# First metric dropped on an empty pane becomes active by default.
	if _active_field == "":
		_set_active_metric(field)

func remove_metric_box(field: String) -> void:
	if not _metric_boxes.has(field):
		return
	_metric_boxes[field].queue_free()
	_metric_boxes.erase(field)
	_canvas.remove_metric(field)
	# The box's own queue_free() above already recursively frees its legend
	# rows — this just drops the now-stale references so a later re-drop of
	# "Time Delta" (add_delta_series) doesn't think those keys are still here.
	if field == ChartCanvas.DELTA_FIELD:
		_delta_legend_items.clear()
	if _active_field == field:
		_active_field = ""
		if not _metric_boxes.is_empty():
			_set_active_metric(_metric_boxes.keys()[0])

func _set_active_metric(field: String) -> void:
	_active_field = field
	for f in _metric_boxes:
		_metric_boxes[f].set_active(f == field)
	_canvas.set_active_metric(field)

# Adds a driver's plotted line (via the raw telemetry blob) to the given
# metric, plus the matching "color swatch + name" row in its floating box.
# Returns null if that metric hasn't been dropped onto this pane.
func add_metric_series(field: String, key: String, label: String, color: Color, raw_data: PackedByteArray) -> Control:
	if not _metric_boxes.has(field):
		return null
	var item: Control = SelectedDriverItemScene.instantiate()
	_metric_boxes[field].driver_list.add_child(item)
	item.get_node("driver_name").text = label
	var cpb: ColorPickerButton = item.get_node("color_selection")
	cpb.color = color
	# This picker only ever affects its own line — no cross-box/global sync.
	cpb.color_changed.connect(func(c: Color) -> void:
		_canvas.set_series_color(field, key, c)
	)
	_canvas.add_series(field, key, color, raw_data)
	return item

# Like add_metric_series, but for a delta line: raw_data is the compared
# lap, reference_raw is the pinned reference lap. Returns null if the "Time
# Delta" box hasn't been dropped onto this pane, or if `key` already has a
# delta line here.
func add_delta_series(key: String, label: String, color: Color, raw_data: PackedByteArray, reference_raw: PackedByteArray) -> Control:
	if not _metric_boxes.has(ChartCanvas.DELTA_FIELD) or _delta_legend_items.has(key):
		return null
	var item: Control = SelectedDriverItemScene.instantiate()
	_metric_boxes[ChartCanvas.DELTA_FIELD].driver_list.add_child(item)
	item.get_node("driver_name").text = label
	var cpb: ColorPickerButton = item.get_node("color_selection")
	cpb.color = color
	cpb.color_changed.connect(func(c: Color) -> void:
		_canvas.set_series_color(ChartCanvas.DELTA_FIELD, key, c)
	)
	_canvas.add_delta_series(key, color, raw_data, reference_raw)
	_delta_legend_items[key] = item
	return item

# Re-points every delta series in this pane at a new reference lap.
func rebase_delta_series(reference_raw: PackedByteArray) -> void:
	_canvas.rebase_delta_series(reference_raw)

# Removes every delta line and its legend row (but keeps the "Time Delta"
# box itself) — used when the reference lap is unpinned.
func clear_all_delta_series() -> void:
	_canvas.clear_delta_series()
	for item in _delta_legend_items.values():
		if is_instance_valid(item):
			item.queue_free()
	_delta_legend_items.clear()

# Removes one driver's plotted line from every metric in this pane (the
# legend rows themselves are freed by the caller, which tracks them directly).
func remove_series_everywhere(key: String) -> void:
	_canvas.remove_series_everywhere(key)
	_delta_legend_items.erase(key)

# Appends newly-arrived live rows to `key`'s line in every metric box here.
func append_series_rows(key: String, row_bytes: PackedByteArray) -> void:
	for field in get_metric_fields():
		_canvas.append_series_rows(field, key, row_bytes)

# Clears `key`'s plotted data (but keeps it on the board) — used on lap
# rollover so a live watch keeps going without being re-added.
func clear_series_data(key: String) -> void:
	for field in get_metric_fields():
		_canvas.clear_series_data(field, key)

# Shows/hides `key`'s plotted line in every metric box here — the sidebar's
# eye toggle, so hiding a driver declutters the chart without removing it.
func set_series_visible(key: String, is_visible: bool) -> void:
	for field in get_metric_fields():
		_canvas.set_series_visible(field, key, is_visible)

# The legend stack is a corner-anchored box, not a stretched rect, so its
# height doesn't follow its children automatically — keep it in sync by hand.
func _update_legend_size() -> void:
	_legend_stack.offset_bottom = _legend_stack.offset_top + _legend_stack.get_combined_minimum_size().y

# false = side by side, true = stacked top/bottom.
func _split(vertical: bool) -> void:
	var parent := get_parent()
	var idx := get_index()

	var split: SplitContainer = VSplitContainer.new() if vertical else HSplitContainer.new()
	# `parent` may be a plain Control (only anchors matter) or another
	# SplitContainer (only size flags matter) — set both so the new split
	# fills its slot either way. Without this it collapses to zero size and
	# the drag handle has nothing to drag.
	split.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	split.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	split.size_flags_vertical   = Control.SIZE_EXPAND_FILL
	split.add_theme_constant_override("separation", 6)

	parent.remove_child(self)
	parent.add_child(split)
	parent.move_child(split, idx)

	split.add_child(self)
	split.add_child(ChartPaneScene.instantiate())

	_update_close_enabled()

# Collapses this pane's SplitContainer, leaving only its sibling behind.
func _close() -> void:
	var parent := get_parent()
	if not (parent is SplitContainer):
		return
	var split := parent as SplitContainer

	var sibling: Node = null
	for c in split.get_children():
		if c != self:
			sibling = c
			break
	if sibling == null:
		return

	var grandparent := split.get_parent()
	var split_idx := split.get_index()

	split.remove_child(sibling)
	grandparent.remove_child(split)
	grandparent.add_child(sibling)
	grandparent.move_child(sibling, split_idx)
	# While inside `split`, its layout kept overwriting sibling's offset_*
	# to match its half of the divider. Those stale pixel offsets would
	# otherwise linger and get reinterpreted once sibling is anchor-laid-out
	# again, making it look like it "didn't resize" to fill the freed space.
	if sibling is Control:
		(sibling as Control).set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	if sibling is ChartPane:
		sibling._update_close_enabled()

	split.queue_free()
	queue_free()
