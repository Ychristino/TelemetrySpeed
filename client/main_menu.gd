extends Control

const DriverListItem := preload("res://driver_list_item.tscn")

# Grouped telemetry channels.  field must match TelemetryDecoder.field().
const GROUPS: Array[Dictionary] = [
	{
		label = "Driver Inputs",
		metrics = [
			{label = "Throttle",  field = "throttle",  unit = "0–1"},
			{label = "Brake",     field = "brake",      unit = "0–1"},
			{label = "Steering",  field = "steering",   unit = "-1..1"},
			{label = "Clutch",    field = "clutch",     unit = "0–1"},
			{label = "Gear",      field = "gear",       unit = ""},
			{label = "DRS",       field = "drs",        unit = "0/1"},
		]
	},
	{
		label = "Vehicle",
		metrics = [
			{label = "Speed",     field = "speed",      unit = "km/h"},
			{label = "G-Lat",     field = "g_lat",      unit = "g"},
			{label = "G-Long",    field = "g_long",     unit = "g"},
			{label = "G-Vert",    field = "g_vert",     unit = "g"},
		]
	},
	{
		label = "Power Unit",
		metrics = [
			{label = "RPM",           field = "rpm",              unit = "rpm"},
			{label = "Engine Temp",   field = "eng_temp",         unit = "°C"},
			{label = "ICE Power",     field = "engine_power_ice", unit = "W"},
			{label = "MGU-K Power",   field = "engine_power_mguk",unit = "W"},
			{label = "ERS Store",     field = "ers_store",        unit = "J"},
			{label = "Fuel",          field = "fuel",             unit = "kg"},
			{label = "Fuel (Laps)",   field = "fuel_rem_laps",    unit = "laps"},
		]
	},
	{
		label = "Tyres / Brakes",
		metrics = [
			{label = "Tyre Surf Temp",  field = "tyre_surf_avg",   unit = "°C"},
			{label = "Tyre Inner Temp", field = "tyre_inner_avg",  unit = "°C"},
			{label = "Tyre Pressure",   field = "tyre_press_avg",  unit = "PSI"},
			{label = "Tyre Wear",       field = "tyre_wear_avg",   unit = "%"},
			{label = "Brake Temp",      field = "brake_temp_avg",  unit = "°C"},
		]
	},
	{
		label = "Delta",
		metrics = [
			# "delta" here must match ChartCanvas.DELTA_FIELD — kept as a plain
			# string rather than a cross-script const reference in this array
			# literal, since GROUPS is evaluated as a const at parse time.
			{label = "Time Delta", field = "delta", unit = "s"},
		]
	},
]

const _COLORS: Array[Color] = [
	Color(1.0, 0.25, 0.25), Color(0.2, 0.8, 1.0),  Color(1.0, 0.9, 0.1),
	Color(0.2, 1.0, 0.45),  Color(1.0, 0.5, 0.1),  Color(0.8, 0.2, 1.0),
]

@onready var _top_menu: HBoxContainer      = $HBoxContainer2/main_content/top_menu
@onready var _open_add_dialog_btn: Button  = $HBoxContainer2/main_content/top_menu/open_add_dialog_btn
@onready var _add_dialog: Window           = $HBoxContainer2/main_content/top_menu/add_driver_dialog
@onready var _game_toggle: Button          = $HBoxContainer2/main_content/top_menu/add_driver_dialog/modal_card/margin/vbox/game_toggle
@onready var _game_scroll: ScrollContainer = $HBoxContainer2/main_content/top_menu/add_driver_dialog/modal_card/margin/vbox/game_scroll
@onready var _game_list: VBoxContainer     = $HBoxContainer2/main_content/top_menu/add_driver_dialog/modal_card/margin/vbox/game_scroll/game_list
@onready var _track_toggle: Button         = $HBoxContainer2/main_content/top_menu/add_driver_dialog/modal_card/margin/vbox/track_toggle
@onready var _track_scroll: ScrollContainer = $HBoxContainer2/main_content/top_menu/add_driver_dialog/modal_card/margin/vbox/track_scroll
@onready var _track_list: VBoxContainer    = $HBoxContainer2/main_content/top_menu/add_driver_dialog/modal_card/margin/vbox/track_scroll/track_list
@onready var _driver_lap_label: Label      = $HBoxContainer2/main_content/top_menu/add_driver_dialog/modal_card/margin/vbox/driver_lap_label
@onready var _driver_lap_search: LineEdit  = $HBoxContainer2/main_content/top_menu/add_driver_dialog/modal_card/margin/vbox/search_box
@onready var _results_scroll: ScrollContainer = $HBoxContainer2/main_content/top_menu/add_driver_dialog/modal_card/margin/vbox/results_scroll
@onready var _driver_lap_results: VBoxContainer = $HBoxContainer2/main_content/top_menu/add_driver_dialog/modal_card/margin/vbox/results_scroll/results_list
@onready var _selected_label: Label        = $HBoxContainer2/main_content/top_menu/add_driver_dialog/modal_card/margin/vbox/selected_label
@onready var _dialog_add_btn: Button       = $HBoxContainer2/main_content/top_menu/add_driver_dialog/modal_card/margin/vbox/footer/add_btn
@onready var _dialog_close_btn: Button     = $HBoxContainer2/main_content/top_menu/add_driver_dialog/modal_card/margin/vbox/footer/close_btn
@onready var _tab_bar: TabContainer        = $HBoxContainer2/left_sidebar/sidebar_content/TabContainer
@onready var _tab_label: Label             = $HBoxContainer2/left_sidebar/sidebar_content/tab_label
@onready var _prev_tab_btn: Button         = $HBoxContainer2/left_sidebar/sidebar_content/tab_nav/prev_tab
@onready var _next_tab_btn: Button         = $HBoxContainer2/left_sidebar/sidebar_content/tab_nav/next_tab

var _driver_tab_list: VBoxContainer

var _api: TelemetryAPIClient

# --- Top bar extras: Export/Import (stub), Select Lap (rename of the old
# Add Driver/Lap button), and the Capture modal + its status dot ---
var _capture_dot: Label
var _capture_dot_label: Label
var _capture_modal: Window
var _capture_port_edit: LineEdit
var _capture_play_btn: Button
var _capture_status_label: Label

# --- Update notice (top bar) ---
var _update_btn: Button

# --- Export / Import modal ---
var _export_modal: Window
var _export_scope := "all"  # "all" | "track" | "lap"
var _export_game := ""
var _export_track := ""
var _export_session_id := ""
var _export_car_index := -1
var _export_lap_number := -1
var _export_driver := ""
var _export_status_label: Label
var _export_btn: Button
var _import_btn: Button
var _export_game_toggle: Button
var _export_game_scroll: ScrollContainer
var _export_game_list: VBoxContainer
var _export_track_toggle: Button
var _export_track_scroll: ScrollContainer
var _export_track_list: VBoxContainer
var _export_scope_section: VBoxContainer
var _export_lap_section: VBoxContainer
var _export_lap_results: VBoxContainer
var _export_file_dialog: FileDialog
var _import_file_dialog: FileDialog
# True for exactly one lap_times_received while the export modal's own
# track picker triggered the fetch — routes that one response into the
# export modal's lap list instead of the Add Driver dialog's, so the two
# pickers (each with their own selected game/track) never clobber each other
# even though they share one TelemetryAPIClient.fetch_lap_times call.
var _export_fetching_laps := false
var _export_lap_choices: Array[Dictionary] = []
# Save path chosen via _export_file_dialog before the async export request
# completed — consumed (and cleared) by _on_export_data_received/_on_export_error.
var _export_save_path := ""

var _all_sessions: Array = []
# Fastest-first list of {session_id, driver, lap, time_ms}, one entry per
# valid completed lap, aggregated across every session ever recorded for the
# selected game+track (not just the latest one).
var _lap_time_choices: Array[Dictionary] = []
# Drivers currently producing telemetry: [{session_id, driver, lap_number}], refreshed by polling.
var _live_drivers: Array = []
# Theoretical best lap for the selected game+track — {ideal_time_ms, sector1,
# sector2, sector3}, each a {driver, session_id, lap_number, time_ms}. Empty
# until fetched, or if there isn't enough data yet for all three sectors.
var _ideal_lap_info: Dictionary = {}
# Combined Driver/Lap popup entries (live first, then historical) — the full,
# unfiltered set. _render_driver_lap_list() narrows this down to what's shown.
var _driver_lap_choices: Array[Dictionary] = []
var _current_game       := ""
var _current_track      := ""
# session_id backing whichever lap/live entry is currently selected in the
# Driver/Lap dropdown — required to fetch the right recording once a track
# has more than one session on record.
var _current_session_id := ""
var _current_lap        := 0
var _current_driver     := ""
var _current_is_live    := false
# True when the current Driver/Lap selection is the synthetic Ideal Lap entry
# rather than a real driver/lap — see _on_ideal_lap_row_selected.
var _current_is_ideal   := false
# Finish time of whichever lap is currently selected — 0 for a live entry.
# Carried into _active_series[key]["time_ms"] on add, so _ensure_reference_default
# can find the quickest already-added lap without re-decoding telemetry.
var _current_time_ms    := 0

# The one driver currently being watched live (single WebSocket connection),
# and the lap number its incoming rows currently belong to.
var _live_key := ""
var _live_lap := 0
var _live_poll_timer: Timer

# "{driver}#{lap}" → {color: Color, data: PackedByteArray, hidden: bool, is_live: bool, all_items: Array}
var _active_series: Dictionary = {}

# The series currently pinned as the delta chart's reference lap (empty =
# none pinned). A live series can never be the reference — see
# _make_driver_list_item — so its raw data, once pinned, never changes size.
var _reference_key := ""
# The reference_toggle button for _reference_key, kept so pinning a new one
# can un-press the previous button without re-triggering its toggled signal.
var _reference_btn: Button = null

func _ready() -> void:
	_api = TelemetryAPIClient.new()
	add_child(_api)
	_api.sessions_received.connect(_on_sessions_received)
	_api.lap_times_received.connect(_on_lap_times_received)
	_api.ideal_lap_info_received.connect(_on_ideal_lap_info_received)
	_api.live_drivers_received.connect(_on_live_drivers_received)
	_api.telemetry_received.connect(_on_telemetry_received)
	_api.stream_frame.connect(_on_stream_frame)
	_api.request_error.connect(func(msg: String): push_error("[API] " + msg))
	_api.export_data_received.connect(_on_export_data_received)
	_api.export_error.connect(_on_export_error)
	_api.import_result_received.connect(_on_import_result_received)
	_api.import_error.connect(_on_import_error)

	UpdateChecker.update_available.connect(_on_update_available)
	UpdateChecker.check_failed.connect(func(reason: String): print("[UpdateChecker] ", reason))
	UpdateChecker.check_now()

	_live_poll_timer = Timer.new()
	_live_poll_timer.wait_time = 2.0
	_live_poll_timer.timeout.connect(func():
		if _current_track != "":
			_api.fetch_live_drivers(_current_game, _current_track)
	)
	add_child(_live_poll_timer)
	_live_poll_timer.start()

	# Non-exclusive so it doesn't block the main window from receiving its own
	# close request while open — an exclusive child Window suppresses the
	# owner's OS close button entirely, which would silently orphan
	# engine.exe/postgres.exe if the user closes the app with this dialog up.
	_add_dialog.exclusive = false
	# See _add_square_backdrop's doc comment — same corner-wedge fix as the
	# code-built modals, just applied after the fact since this dialog's
	# modal_card comes from the .tscn instead.
	_add_square_backdrop(_add_dialog)

	_driver_lap_search.text_changed.connect(_render_driver_lap_list)
	_open_add_dialog_btn.pressed.connect(_open_add_dialog)
	_dialog_add_btn.pressed.connect(_on_add_pressed)
	_dialog_close_btn.pressed.connect(_add_dialog.hide)
	_add_dialog.close_requested.connect(_add_dialog.hide)
	_game_toggle.pressed.connect(func(): _game_scroll.visible = not _game_scroll.visible)
	_track_toggle.pressed.connect(func(): _track_scroll.visible = not _track_scroll.visible)
	_prev_tab_btn.pressed.connect(func(): _step_tab(-1))
	_next_tab_btn.pressed.connect(func(): _step_tab(1))
	_tab_bar.tab_changed.connect(_on_tab_changed)
	get_tree().node_added.connect(_on_node_added)
	# The static root pane already entered the tree (and its own _ready()
	# already ran, joining the "chart_panes" group) before this _ready() got
	# a chance to connect node_added above — catch it explicitly here.
	for pane in get_tree().get_nodes_in_group("chart_panes"):
		_connect_pane(pane)

	_show_list_placeholder(_game_list, "No sessions recorded yet")
	_show_list_placeholder(_track_list, "Pick a game first")
	_hide_driver_lap_section()
	_dialog_add_btn.disabled = true

	_build_sidebar_tabs()
	_on_tab_changed(_tab_bar.current_tab)
	_api.fetch_sessions()

	_build_topbar_extras()
	EngineProcess.listener_status_changed.connect(_on_listener_status_changed)
	EngineProcess.listener_error.connect(_on_listener_error)
	_on_listener_status_changed(EngineProcess.is_listener_running(), EngineProcess.get_listener_port())

# ── Top bar: Export/Import (stub), Select Lap, Capture ──────────────────────
# Layout: [Export / Import Laps] [+ Select Lap] ...stretch... [●dot] [Capture]
# open_add_dialog_btn used to stretch to fill the whole bar by itself; now
# several buttons share it, so a dedicated spacer does the stretching and
# each button just takes its natural size.

func _build_topbar_extras() -> void:
	_open_add_dialog_btn.text = "+  Select Lap"
	_open_add_dialog_btn.size_flags_horizontal = 0

	var export_btn := Button.new()
	export_btn.text = "Export / Import"
	export_btn.pressed.connect(func():
		_export_modal.popup_centered()
		# Same staleness issue as _open_add_dialog — re-fetch every open so
		# recently-recorded games/tracks show up in the Track/Lap pickers.
		_api.fetch_sessions()
	)
	_top_menu.add_child(export_btn)
	_top_menu.move_child(export_btn, 0)

	var spacer := Control.new()
	spacer.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	_top_menu.add_child(spacer)

	_update_btn = Button.new()
	_update_btn.visible = false
	_update_btn.pressed.connect(_on_update_btn_pressed)
	_top_menu.add_child(_update_btn)

	var dot_box := VBoxContainer.new()
	dot_box.alignment = BoxContainer.ALIGNMENT_CENTER
	dot_box.add_theme_constant_override("separation", 0)

	_capture_dot = Label.new()
	_capture_dot.text = "●"
	_capture_dot.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_capture_dot.size_flags_horizontal = Control.SIZE_SHRINK_CENTER
	_capture_dot.add_theme_font_size_override("font_size", 16)
	_capture_dot.add_theme_color_override("font_color", Color(0.4, 0.4, 0.4))
	_capture_dot.tooltip_text = "Telemetry capture stopped"
	dot_box.add_child(_capture_dot)

	_capture_dot_label = Label.new()
	_capture_dot_label.text = "Not running"
	_capture_dot_label.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_capture_dot_label.size_flags_horizontal = Control.SIZE_SHRINK_CENTER
	_capture_dot_label.add_theme_font_size_override("font_size", 9)
	_capture_dot_label.modulate = Color(1, 1, 1, 0.35)
	dot_box.add_child(_capture_dot_label)

	_top_menu.add_child(dot_box)

	var capture_btn := Button.new()
	capture_btn.text = "Capture"
	capture_btn.pressed.connect(func(): _capture_modal.popup_centered())
	_top_menu.add_child(capture_btn)

	_build_version_footer()
	_build_capture_modal()
	_build_export_modal()

# Bottom-right corner tag, always visible — otherwise there's no way to
# confirm which build is actually running (came up debugging why the update
# button kept offering "v0.1.0" with no visible confirmation either way).
# Anchored directly on the root Control rather than living in any layout
# container, so it floats in the corner independent of top-bar contents.
func _build_version_footer() -> void:
	var footer := Label.new()
	footer.text = "v" + UpdateChecker.current_version
	footer.modulate = Color(1, 1, 1, 0.35)
	footer.add_theme_font_size_override("font_size", 10)
	footer.mouse_filter = Control.MOUSE_FILTER_IGNORE
	footer.set_anchors_preset(Control.PRESET_BOTTOM_RIGHT)
	footer.grow_horizontal = Control.GROW_DIRECTION_BEGIN
	footer.grow_vertical = Control.GROW_DIRECTION_BEGIN
	footer.offset_left -= 8
	footer.offset_top -= 6
	footer.offset_right -= 8
	footer.offset_bottom -= 6
	add_child(footer)

# ── Update notice ────────────────────────────────────────────────────────────

func _on_update_available(version: String, _download_url: String) -> void:
	_update_btn.text = "⬆ Update to v%s" % version
	_update_btn.visible = true
	_update_btn.tooltip_text = "Download and install the new version — the app will close and restart automatically."

func _on_update_btn_pressed() -> void:
	var confirm := ConfirmationDialog.new()
	confirm.title = "Update Available"
	confirm.dialog_text = "%s\n\nThe app will close, install the update, and you'll need to reopen it." % _update_btn.text.trim_prefix("⬆ ")
	confirm.confirmed.connect(func():
		_update_btn.disabled = true
		_update_btn.text = "Downloading update..."
		UpdateChecker.download_and_install()
	)
	add_child(confirm)
	confirm.popup_centered()

# The modal where the UDP port is set and capture is started/stopped —
# opened from the "Capture" button; the top bar itself only shows the dot.
func _build_capture_modal() -> void:
	_capture_modal = Window.new()
	_capture_modal.title = "Telemetry Capture"
	_capture_modal.size = Vector2i(420, 220)
	_capture_modal.unresizable = true
	_capture_modal.visible = false
	# See the matching comment on _add_dialog.exclusive above — same reasoning.
	_capture_modal.exclusive = false
	_capture_modal.close_requested.connect(_capture_modal.hide)
	add_child(_capture_modal)

	# The Window's own embedded chrome can't reliably render corner_radius/
	# border (Godot only applies shadow/bg_color to it) — see the matching
	# comment on add_driver_dialog's "modal_card" in main_menu.tscn. This
	# panel is the actual visible rounded card; the Window's chrome behind it
	# is set to a flat, borderless style that matches the app background.
	# A rounded StyleBoxFlat simply doesn't paint its own corner pixels, so
	# whatever the engine renders behind the card (not reachable through any
	# window theme property — confirmed by forcing embedded_border to a
	# glaring color and seeing no change) shows through as a small gray
	# wedge at each corner. _add_square_backdrop fixes that the same way for
	# every card-style modal: a flat, non-rounded panel in the same color,
	# placed behind the card, so those corner pixels show our own matching
	# fill instead of whatever the engine's default is.
	_add_square_backdrop(_capture_modal)
	var card := PanelContainer.new()
	card.set_anchors_preset(Control.PRESET_FULL_RECT)
	card.add_theme_stylebox_override("panel", load("res://modal_card_style.tres"))
	_capture_modal.add_child(card)

	var margin := MarginContainer.new()
	for side in ["left", "top", "right", "bottom"]:
		margin.add_theme_constant_override("margin_" + side, 20)
	card.add_child(margin)

	var vbox := VBoxContainer.new()
	vbox.add_theme_constant_override("separation", 14)
	margin.add_child(vbox)

	var desc := Label.new()
	desc.text = "Listens for UDP telemetry from the game and records it to this session."
	desc.autowrap_mode = TextServer.AUTOWRAP_WORD
	desc.modulate = Color(1, 1, 1, 0.6)
	desc.add_theme_font_size_override("font_size", 11)
	vbox.add_child(desc)

	var port_row := HBoxContainer.new()
	port_row.add_theme_constant_override("separation", 10)
	vbox.add_child(port_row)

	var port_label := Label.new()
	port_label.text = "UDP Port"
	port_row.add_child(port_label)

	_capture_port_edit = LineEdit.new()
	_capture_port_edit.text = str(EngineProcess.get_listener_port())
	_capture_port_edit.custom_minimum_size = Vector2(110, 0)
	port_row.add_child(_capture_port_edit)

	vbox.add_child(HSeparator.new())

	var btn_row := HBoxContainer.new()
	btn_row.add_theme_constant_override("separation", 10)
	vbox.add_child(btn_row)

	_capture_play_btn = Button.new()
	_capture_play_btn.text = "Play"
	_capture_play_btn.pressed.connect(_on_capture_play_pressed)
	btn_row.add_child(_capture_play_btn)

	_capture_status_label = Label.new()
	_capture_status_label.text = "Stopped"
	btn_row.add_child(_capture_status_label)

	var close_btn := Button.new()
	close_btn.text = "Close"
	close_btn.pressed.connect(_capture_modal.hide)
	vbox.add_child(close_btn)

# See the call sites above for why this exists. Adds a flat, square,
# same-color panel as the first child of a card-style modal Window, so the
# few pixels a rounded StyleBoxFlat leaves unpainted at each corner show our
# own fill instead of whatever the engine draws there by default.
func _add_square_backdrop(window: Window) -> void:
	var backdrop := PanelContainer.new()
	backdrop.set_anchors_preset(Control.PRESET_FULL_RECT)
	backdrop.mouse_filter = Control.MOUSE_FILTER_IGNORE
	var flat := StyleBoxFlat.new()
	flat.bg_color = Color(0.117, 0.117, 0.14, 1)
	backdrop.add_theme_stylebox_override("panel", flat)
	window.add_child(backdrop)
	window.move_child(backdrop, 0) # must render behind the rounded card on top of it

# ── Export / Import modal ────────────────────────────────────────────────────
# Lets the user pull recorded laps out to a file (the whole database, one
# track, or a single lap) and load one back in elsewhere — see
# database.ExportBundle / database.DB.Import on the backend for the actual
# scope semantics and idempotency guarantees.
func _build_export_modal() -> void:
	_export_modal = Window.new()
	_export_modal.title = "Export / Import"
	_export_modal.size = Vector2i(520, 560)
	_export_modal.min_size = Vector2i(440, 320)
	_export_modal.max_size = Vector2i(700, 800)
	_export_modal.unresizable = false
	_export_modal.visible = false
	_export_modal.exclusive = false
	_export_modal.close_requested.connect(_export_modal.hide)
	add_child(_export_modal)

	_add_square_backdrop(_export_modal)
	var card := PanelContainer.new()
	card.set_anchors_preset(Control.PRESET_FULL_RECT)
	card.add_theme_stylebox_override("panel", load("res://modal_card_style.tres"))
	_export_modal.add_child(card)

	var margin := MarginContainer.new()
	for side in ["left", "top", "right", "bottom"]:
		margin.add_theme_constant_override("margin_" + side, 20)
	card.add_child(margin)

	# Single Lap scope adds enough content (game/track pickers + lap list)
	# that the whole card can exceed the window's fixed height — wrapping in
	# a ScrollContainer means that content scrolls instead of the footer
	# (Import section, Close button) getting clipped off the bottom with no
	# way to reach it.
	var outer_scroll := ScrollContainer.new()
	outer_scroll.horizontal_scroll_mode = ScrollContainer.SCROLL_MODE_DISABLED
	margin.add_child(outer_scroll)

	var vbox := VBoxContainer.new()
	vbox.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	vbox.add_theme_constant_override("separation", 12)
	outer_scroll.add_child(vbox)

	var export_heading := Label.new()
	export_heading.text = "Export"
	export_heading.add_theme_color_override("font_color", Color(0.87, 0.33, 0.24, 1))
	export_heading.add_theme_font_size_override("font_size", 13)
	vbox.add_child(export_heading)

	var desc := Label.new()
	desc.text = "Export recorded laps to a file — the whole database, one track, or a single lap."
	desc.autowrap_mode = TextServer.AUTOWRAP_WORD
	desc.modulate = Color(1, 1, 1, 0.6)
	desc.add_theme_font_size_override("font_size", 11)
	vbox.add_child(desc)

	var scope_group := ButtonGroup.new()
	var scope_row := HBoxContainer.new()
	scope_row.add_theme_constant_override("separation", 6)
	vbox.add_child(scope_row)

	var scope_all_btn := Button.new()
	scope_all_btn.text = "Entire Database"
	scope_all_btn.toggle_mode = true
	scope_all_btn.button_group = scope_group
	scope_all_btn.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	scope_all_btn.button_pressed = true
	scope_all_btn.pressed.connect(_on_export_scope_selected.bind("all"))
	scope_row.add_child(scope_all_btn)

	var scope_track_btn := Button.new()
	scope_track_btn.text = "This Track"
	scope_track_btn.toggle_mode = true
	scope_track_btn.button_group = scope_group
	scope_track_btn.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	scope_track_btn.pressed.connect(_on_export_scope_selected.bind("track"))
	scope_row.add_child(scope_track_btn)

	var scope_lap_btn := Button.new()
	scope_lap_btn.text = "Single Lap"
	scope_lap_btn.toggle_mode = true
	scope_lap_btn.button_group = scope_group
	scope_lap_btn.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	scope_lap_btn.pressed.connect(_on_export_scope_selected.bind("lap"))
	scope_row.add_child(scope_lap_btn)

	_export_scope_section = VBoxContainer.new()
	_export_scope_section.add_theme_constant_override("separation", 8)
	_export_scope_section.visible = false
	vbox.add_child(_export_scope_section)

	_export_game_toggle = Button.new()
	_export_game_toggle.alignment = HORIZONTAL_ALIGNMENT_LEFT
	_export_game_toggle.text = "Game: (choose)  ▾"
	_export_game_toggle.pressed.connect(func(): _export_game_scroll.visible = not _export_game_scroll.visible)
	_export_scope_section.add_child(_export_game_toggle)

	_export_game_scroll = ScrollContainer.new()
	_export_game_scroll.visible = false
	_export_game_scroll.horizontal_scroll_mode = ScrollContainer.SCROLL_MODE_DISABLED
	_export_game_scroll.custom_minimum_size = Vector2(0, 56)
	_export_scope_section.add_child(_export_game_scroll)
	_export_game_list = VBoxContainer.new()
	_export_game_list.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	_export_game_list.add_theme_constant_override("separation", 6)
	_export_game_scroll.add_child(_export_game_list)

	_export_track_toggle = Button.new()
	_export_track_toggle.alignment = HORIZONTAL_ALIGNMENT_LEFT
	_export_track_toggle.text = "Track: (choose)  ▾"
	_export_track_toggle.pressed.connect(func(): _export_track_scroll.visible = not _export_track_scroll.visible)
	_export_scope_section.add_child(_export_track_toggle)

	_export_track_scroll = ScrollContainer.new()
	_export_track_scroll.visible = false
	_export_track_scroll.horizontal_scroll_mode = ScrollContainer.SCROLL_MODE_DISABLED
	_export_track_scroll.custom_minimum_size = Vector2(0, 56)
	_export_scope_section.add_child(_export_track_scroll)
	_export_track_list = VBoxContainer.new()
	_export_track_list.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	_export_track_list.add_theme_constant_override("separation", 6)
	_export_track_scroll.add_child(_export_track_list)

	_export_lap_section = VBoxContainer.new()
	_export_lap_section.add_theme_constant_override("separation", 6)
	_export_lap_section.visible = false
	vbox.add_child(_export_lap_section)

	var lap_label := Label.new()
	lap_label.text = "Lap"
	_export_lap_section.add_child(lap_label)

	var lap_scroll := ScrollContainer.new()
	lap_scroll.horizontal_scroll_mode = ScrollContainer.SCROLL_MODE_DISABLED
	lap_scroll.custom_minimum_size = Vector2(0, 180)
	lap_scroll.size_flags_vertical = Control.SIZE_EXPAND_FILL
	_export_lap_section.add_child(lap_scroll)
	_export_lap_results = VBoxContainer.new()
	_export_lap_results.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	_export_lap_results.add_theme_constant_override("separation", 6)
	lap_scroll.add_child(_export_lap_results)

	var export_row := HBoxContainer.new()
	vbox.add_child(export_row)
	_export_btn = Button.new()
	_export_btn.text = "Export..."
	_export_btn.pressed.connect(_on_export_pressed)
	export_row.add_child(_export_btn)

	vbox.add_child(HSeparator.new())

	var import_heading := Label.new()
	import_heading.text = "Import"
	import_heading.add_theme_color_override("font_color", Color(0.87, 0.33, 0.24, 1))
	import_heading.add_theme_font_size_override("font_size", 13)
	vbox.add_child(import_heading)

	var import_desc := Label.new()
	import_desc.text = "Import a file someone exported to you — everything it contains is restored as-is."
	import_desc.autowrap_mode = TextServer.AUTOWRAP_WORD
	import_desc.modulate = Color(1, 1, 1, 0.6)
	import_desc.add_theme_font_size_override("font_size", 11)
	vbox.add_child(import_desc)

	var import_row := HBoxContainer.new()
	vbox.add_child(import_row)
	_import_btn = Button.new()
	_import_btn.text = "Import..."
	_import_btn.pressed.connect(_on_import_pressed)
	import_row.add_child(_import_btn)

	vbox.add_child(HSeparator.new())

	var footer := HBoxContainer.new()
	vbox.add_child(footer)
	var spacer := Control.new()
	spacer.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	footer.add_child(spacer)
	var export_close_btn := Button.new()
	export_close_btn.text = "Close"
	export_close_btn.pressed.connect(_export_modal.hide)
	footer.add_child(export_close_btn)

	_export_status_label = Label.new()
	_export_status_label.text = ""
	_export_status_label.autowrap_mode = TextServer.AUTOWRAP_WORD
	_export_status_label.modulate = Color(1, 1, 1, 0.6)
	_export_status_label.add_theme_font_size_override("font_size", 11)
	vbox.add_child(_export_status_label)

	_export_file_dialog = FileDialog.new()
	_export_file_dialog.access = FileDialog.ACCESS_FILESYSTEM
	# Use the real OS file picker (Explorer on Windows) instead of Godot's
	# built-in browser — lets the user navigate by clicking through folders
	# the way they're used to, rather than typing/pasting a path.
	_export_file_dialog.use_native_dialog = true
	_export_file_dialog.add_filter("*.tvexport", "Telemetry Export")
	_export_file_dialog.file_selected.connect(_on_export_save_path_chosen)
	add_child(_export_file_dialog)

	_import_file_dialog = FileDialog.new()
	_import_file_dialog.access = FileDialog.ACCESS_FILESYSTEM
	_import_file_dialog.use_native_dialog = true
	_import_file_dialog.add_filter("*.tvexport", "Telemetry Export")
	_import_file_dialog.file_selected.connect(_on_import_path_chosen)
	add_child(_import_file_dialog)

func _on_export_scope_selected(scope: String) -> void:
	_export_scope = scope
	_export_scope_section.visible = (scope != "all")
	_export_lap_section.visible = (scope == "lap")
	_export_status_label.text = ""
	_update_export_button_state()

func _render_export_game_list() -> void:
	if _export_game_list == null:
		return
	var seen := {}
	var games: Array[String] = []
	for s in _all_sessions:
		if not seen.has(s.game):
			seen[s.game] = true
			games.append(s.game)
	_render_simple_list(_export_game_list, games, "No sessions recorded yet", _on_export_game_row_selected)

func _render_export_track_list() -> void:
	var seen := {}
	var tracks: Array[String] = []
	for s in _all_sessions:
		if s.game == _export_game and not seen.has(s.track):
			seen[s.track] = true
			tracks.append(s.track)
	_render_simple_list(_export_track_list, tracks, "No tracks recorded for this game yet", _on_export_track_row_selected)

func _on_export_game_row_selected(game: String) -> void:
	_export_game = game
	_export_game_toggle.text = "Game: %s  ▾" % game
	_export_game_scroll.visible = false
	_export_track = ""
	_export_track_toggle.text = "Track: (choose)  ▾"
	_render_export_track_list()
	_export_lap_choices.clear()
	_render_export_lap_list()
	_reset_export_lap_selection()

func _on_export_track_row_selected(track: String) -> void:
	_export_track = track
	_export_track_toggle.text = "Track: %s  ▾" % track
	_export_track_scroll.visible = false
	_reset_export_lap_selection()
	if _export_scope == "lap":
		_export_fetching_laps = true
		_api.fetch_lap_times(_export_game, _export_track)
	_update_export_button_state()

func _reset_export_lap_selection() -> void:
	_export_session_id = ""
	_export_car_index = -1
	_export_lap_number = -1
	_export_driver = ""
	_update_export_button_state()

func _render_export_lap_list() -> void:
	for c in _export_lap_results.get_children():
		c.queue_free()
	if _export_lap_choices.is_empty():
		var lbl := Label.new()
		lbl.text = "No recorded laps for this track yet"
		lbl.modulate = Color(1, 1, 1, 0.4)
		_export_lap_results.add_child(lbl)
		return
	for entry in _export_lap_choices:
		var btn := Button.new()
		btn.alignment = HORIZONTAL_ALIGNMENT_LEFT
		btn.size_flags_horizontal = Control.SIZE_EXPAND_FILL
		btn.clip_text = true
		btn.text = "%s — Lap %d — %s" % [entry["driver"], entry["lap"], _format_lap_time(entry["time_ms"])]
		btn.tooltip_text = btn.text
		btn.pressed.connect(_on_export_lap_row_selected.bind(entry))
		_style_list_row(btn)
		_export_lap_results.add_child(btn)

func _on_export_lap_row_selected(entry: Dictionary) -> void:
	_export_session_id = entry["session_id"]
	_export_car_index  = entry["car_index"]
	_export_lap_number = entry["lap"]
	_export_driver      = entry["driver"]
	_export_status_label.text = "Selected: %s — Lap %d — %s" % [entry["driver"], entry["lap"], _format_lap_time(entry["time_ms"])]
	_update_export_button_state()

func _update_export_button_state() -> void:
	if _export_btn == null:
		return
	match _export_scope:
		"all":
			_export_btn.disabled = false
		"track":
			_export_btn.disabled = (_export_game == "" or _export_track == "")
		"lap":
			_export_btn.disabled = (_export_session_id == "" or _export_car_index < 0 or _export_lap_number < 0)

func _on_export_pressed() -> void:
	var default_name := "telemetry-all.tvexport"
	match _export_scope:
		"track":
			default_name = "telemetry-%s-%s.tvexport" % [_export_game, _export_track]
		"lap":
			default_name = "telemetry-%s-L%d.tvexport" % [_export_driver, _export_lap_number]
	_export_file_dialog.current_file = default_name.replace(" ", "_")
	_export_file_dialog.file_mode = FileDialog.FILE_MODE_SAVE_FILE
	_export_file_dialog.popup_centered(Vector2i(700, 500))

func _on_export_save_path_chosen(path: String) -> void:
	_export_save_path = path
	_export_status_label.text = "Exporting..."
	match _export_scope:
		"all":
			_api.fetch_export("all")
		"track":
			_api.fetch_export("track", _export_game, _export_track)
		"lap":
			_api.fetch_export("lap", _export_game, _export_track, _export_session_id, _export_car_index, _export_lap_number)

func _on_export_data_received(data: PackedByteArray) -> void:
	if _export_save_path == "":
		return
	var f := FileAccess.open(_export_save_path, FileAccess.WRITE)
	if f == null:
		_export_status_label.text = "Failed to write file: error %d" % FileAccess.get_open_error()
		_export_save_path = ""
		return
	f.store_buffer(data)
	f.close()
	_export_status_label.text = "Exported %d KB to %s" % [data.size() / 1024, _export_save_path.get_file()]
	_export_save_path = ""

func _on_export_error(msg: String) -> void:
	_export_status_label.text = "Export failed: " + msg
	_export_save_path = ""

func _on_import_pressed() -> void:
	_import_file_dialog.file_mode = FileDialog.FILE_MODE_OPEN_FILE
	_import_file_dialog.popup_centered(Vector2i(700, 500))

func _on_import_path_chosen(path: String) -> void:
	var f := FileAccess.open(path, FileAccess.READ)
	if f == null:
		_export_status_label.text = "Failed to read file: error %d" % FileAccess.get_open_error()
		return
	var bytes := f.get_buffer(f.get_length())
	f.close()
	_export_status_label.text = "Importing..."
	_api.upload_import(bytes)

func _on_import_result_received(summary: Dictionary) -> void:
	_export_status_label.text = "Imported %d sessions, %d laps, %d telemetry rows" % [
		int(summary.get("sessions", 0)), int(summary.get("lap_times", 0)), int(summary.get("telemetry", 0))
	]
	# Pick up newly imported games/tracks/laps wherever they're cached.
	_api.fetch_sessions()
	if _current_track != "":
		_api.fetch_lap_times(_current_game, _current_track)

func _on_import_error(msg: String) -> void:
	_export_status_label.text = "Import failed: " + msg

func _on_capture_play_pressed() -> void:
	if EngineProcess.is_listener_running():
		_capture_play_btn.disabled = true
		EngineProcess.stop_listener()
		return
	var port := _capture_port_edit.text.to_int()
	if port < 1 or port > 65535:
		_capture_status_label.text = "Invalid port"
		return
	_capture_play_btn.disabled = true
	EngineProcess.start_listener(port)

func _on_listener_status_changed(running: bool, port: int) -> void:
	_capture_play_btn.disabled = false
	_capture_play_btn.text = "Stop" if running else "Play"
	_capture_status_label.text = ("Listening on :%d" % port) if running else "Stopped"
	if not _capture_port_edit.has_focus():
		_capture_port_edit.text = str(port)
	_capture_dot.add_theme_color_override("font_color", Color(1.0, 0.15, 0.15) if running else Color(0.4, 0.4, 0.4))
	_capture_dot.tooltip_text = ("Capturing telemetry on UDP :%d" % port) if running else "Telemetry capture stopped"
	_capture_dot_label.text = ("Running on: %d" % port) if running else "Not running"

func _on_listener_error(message: String) -> void:
	_capture_play_btn.disabled = false
	_capture_status_label.text = "Error: " + message

# ── Tab / metric-button setup ────────────────────────────────────────────────

# Sidebar tabs: page 0 is the "Drivers" list (drivers currently on the board),
# followed by one page per metric group (the data-type tree).
func _build_sidebar_tabs() -> void:
	for child in _tab_bar.get_children():
		child.free()

	_build_driver_tab()
	_build_metric_tab_pages()

	_tab_bar.set_tab_title(0, "Drivers")
	for i in GROUPS.size():
		_tab_bar.set_tab_title(i + 1, GROUPS[i]["label"])

# The TabBar's built-in overflow arrows only scroll the tab strip into view —
# they don't change the current tab. These buttons do that directly.
func _step_tab(delta: int) -> void:
	var count := _tab_bar.get_tab_count()
	if count == 0:
		return
	_tab_bar.current_tab = (_tab_bar.current_tab + delta + count) % count

# tabs_visible is off on the TabContainer, so this label is the only place
# the current page's name is shown.
#
# idx can be -1 transiently while _build_sidebar_tabs() is rebuilding pages —
# freeing the TabContainer's existing children drops current_tab to -1 and
# fires tab_changed synchronously before any new tabs are added back, so this
# must tolerate that instead of indexing get_tab_title with it.
func _on_tab_changed(idx: int) -> void:
	if idx < 0 or idx >= _tab_bar.get_tab_count():
		return
	_tab_label.text = _tab_bar.get_tab_title(idx)

func _build_driver_tab() -> void:
	var scroll := ScrollContainer.new()
	scroll.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	scroll.size_flags_vertical   = Control.SIZE_EXPAND_FILL

	_driver_tab_list = VBoxContainer.new()
	_driver_tab_list.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	_driver_tab_list.add_theme_constant_override("separation", 7)
	scroll.add_child(_driver_tab_list)

	_tab_bar.add_child(scroll)

func _build_metric_tab_pages() -> void:
	for group in GROUPS:
		var group_label: String = group["label"]
		var group_metrics: Array = group["metrics"]

		# Each tab page = scrollable list of metric buttons.
		var scroll := ScrollContainer.new()
		scroll.size_flags_horizontal = Control.SIZE_EXPAND_FILL
		scroll.size_flags_vertical   = Control.SIZE_EXPAND_FILL

		var vbox := VBoxContainer.new()
		vbox.size_flags_horizontal = Control.SIZE_EXPAND_FILL
		vbox.add_theme_constant_override("separation", 6)
		scroll.add_child(vbox)

		for m in group_metrics:
			var m_label: String = m["label"]
			var m_field: String = m["field"]
			var m_unit: String  = m["unit"]

			var btn := MetricDragButton.new()
			btn.field = m_field
			btn.metric_label = m_label
			btn.text = m_label if m_unit == "" else "%s  (%s)" % [m_label, m_unit]
			btn.size_flags_horizontal = Control.SIZE_EXPAND_FILL
			btn.alignment = HORIZONTAL_ALIGNMENT_LEFT
			btn.clip_text = true
			btn.add_theme_font_size_override("font_size", 10)
			vbox.add_child(btn)

		_tab_bar.add_child(scroll)

# Every pane's metric_dropped signal is wired up here, whether it's the
# static root pane or one created later by splitting.
func _on_node_added(node: Node) -> void:
	if node is ChartPane:
		_connect_pane(node)

func _connect_pane(pane: ChartPane) -> void:
	pane.metric_dropped.connect(_on_pane_metric_dropped.bind(pane))

# A metric was dropped onto a pane: add its box (a no-op if already present),
# plot it, and backfill it with whatever drivers/laps are already on the board.
func _on_pane_metric_dropped(field: String, label: String, pane: ChartPane) -> void:
	if pane.has_metric(field):
		return
	pane.add_metric_box(field, label)
	if field == ChartCanvas.DELTA_FIELD:
		_backfill_delta_pane(pane)
		return
	for raw_key in _active_series:
		var key: String = str(raw_key)
		var s: Dictionary = _active_series[key]
		var item := pane.add_metric_series(field, key, _format_driver_label(key), s["color"], s["data"])
		if item:
			(s["all_items"] as Array).append(item)
		if s.get("hidden", false):
			pane.set_series_visible(key, false)

# Backfills a freshly-dropped "Time Delta" box with every already-added
# series, including the reference itself — its own line plots flat at zero
# (delta from itself), which is what makes it read as "the baseline" rather
# than just vanishing. No-op until a reference is pinned.
func _backfill_delta_pane(pane: ChartPane) -> void:
	if _reference_key == "" or not _active_series.has(_reference_key):
		return
	var reference_raw: PackedByteArray = _active_series[_reference_key]["data"]
	for raw_key in _active_series:
		var key: String = str(raw_key)
		var s: Dictionary = _active_series[key]
		var item: Control = pane.add_delta_series(key, _format_driver_label(key), s["color"], s["data"], reference_raw)
		if item:
			(s["all_items"] as Array).append(item)
		if s.get("hidden", false):
			pane.set_series_visible(key, false)

# Plots `key` on every metric box, in every pane, currently on screen.
# Each row's color swatch is independent from here on — it only sets the
# initial color, it doesn't stay linked to this series or any other row.
# `key` is always freshly added here, so it can never already be
# _reference_key (that's only ever set by pinning an existing series).
func _add_series_to_panes(key: String, color: Color, raw_data: PackedByteArray) -> Array:
	var items: Array = []
	for pane in get_tree().get_nodes_in_group("chart_panes"):
		for field in pane.get_metric_fields():
			var item: Control
			if field == ChartCanvas.DELTA_FIELD:
				if _reference_key == "" or not _active_series.has(_reference_key):
					continue
				item = pane.add_delta_series(key, _format_driver_label(key), color, raw_data, _active_series[_reference_key]["data"])
			else:
				item = pane.add_metric_series(field, key, _format_driver_label(key), color, raw_data)
			if item:
				items.append(item)
	return items

func _format_driver_label(key: String) -> String:
	var parts  := key.split("#")
	var driver := parts[0]
	if driver == "Ideal Lap":
		return "★ Ideal Lap"
	if parts.size() > 1 and parts[1] == "live":
		return "%s  ● LIVE" % driver
	var lap := parts[1].to_int()
	return "%s  L%d" % [driver, lap]

# ── API responses ────────────────────────────────────────────────────────────

func _on_sessions_received(sessions: Array) -> void:
	_all_sessions = sessions
	_render_game_list()
	_render_export_game_list()

# Server already returns laps sorted fastest-first; re-sort defensively so the
# top of the list is always the best lap to compare against.
func _on_lap_times_received(laps: Array) -> void:
	if _export_fetching_laps:
		_export_fetching_laps = false
		_export_lap_choices.clear()
		for l in laps:
			_export_lap_choices.append({
				session_id = str(l.get("session_id", "")),
				car_index  = int(l.get("car_index", 0)),
				driver     = str(l.get("driver", "")),
				lap        = int(l.get("lap_number", 0)),
				time_ms    = int(l.get("lap_time_ms", 0)),
			})
		_render_export_lap_list()
		return

	_lap_time_choices.clear()
	for l in laps:
		_lap_time_choices.append({
			session_id = str(l.get("session_id", "")),
			driver     = str(l.get("driver", "")),
			lap        = int(l.get("lap_number", 0)),
			time_ms    = int(l.get("lap_time_ms", 0)),
		})
	_lap_time_choices.sort_custom(func(a, b): return a["time_ms"] < b["time_ms"])
	_rebuild_driver_lap_menu()

# {} when there isn't enough recorded data yet for all three sectors — the
# Ideal Lap row is simply omitted from the picker in that case.
func _on_ideal_lap_info_received(info: Dictionary) -> void:
	_ideal_lap_info = info
	if _add_dialog.visible:
		_render_driver_lap_list(_driver_lap_search.text)

# Polled every ~2s while a track is selected. Drivers here are actively
# producing telemetry right now (see backend liveWindow) — the lap number is
# whatever they're currently on, possibly still in progress.
func _on_live_drivers_received(drivers: Array) -> void:
	_live_drivers = drivers
	_rebuild_driver_lap_menu()

# Combines live drivers (marked ●) with historical fastest-first laps into
# the single Driver/Lap choice set, backing the searchable popup below.
func _rebuild_driver_lap_menu() -> void:
	_driver_lap_choices.clear()
	for l in _live_drivers:
		_driver_lap_choices.append({
			is_live    = true,
			session_id = str(l.get("session_id", "")),
			driver     = str(l.get("driver", "")),
			lap        = int(l.get("lap_number", 0)),
			time_ms    = 0,
		})
	for entry in _lap_time_choices:
		_driver_lap_choices.append({
			is_live    = false,
			session_id = entry["session_id"],
			driver     = entry["driver"],
			lap        = entry["lap"],
			time_ms    = entry["time_ms"],
		})

	# Keep an already-open dialog's list live too, so a driver going on/off
	# track shows up without the user having to close and reopen it.
	if _add_dialog.visible:
		_render_driver_lap_list(_driver_lap_search.text)

func _open_add_dialog() -> void:
	# No explicit size: keeps whatever size the Window is currently at (its
	# scene default the first time, or whatever the user last resized it to,
	# since the dialog is resizable) rather than resetting it every open.
	_add_dialog.popup_centered()
	_render_driver_lap_list(_driver_lap_search.text)
	# _all_sessions is otherwise only fetched once at startup, so any game/
	# track recorded since then wouldn't show up in the pickers below without
	# this — re-fetching on every open keeps it current.
	_api.fetch_sessions()

# Rebuilds the dialog's list from _driver_lap_choices, filtered by driver name
# (case-insensitive substring match) and grouped live-first, historical
# fastest-first — matching the underlying array's own ordering.
func _render_driver_lap_list(filter: String) -> void:
	for c in _driver_lap_results.get_children():
		c.queue_free()

	var needle := filter.to_lower()
	var live: Array[Dictionary] = []
	var historical: Array[Dictionary] = []
	for entry in _driver_lap_choices:
		if needle != "" and not (needle in String(entry["driver"]).to_lower()):
			continue
		if entry["is_live"]:
			live.append(entry)
		else:
			historical.append(entry)

	var has_ideal := not _ideal_lap_info.is_empty()

	if not has_ideal and live.is_empty() and historical.is_empty():
		var empty_lbl := Label.new()
		empty_lbl.text = "No matches" if needle != "" else "No live drivers or recorded laps yet"
		empty_lbl.modulate = Color(1, 1, 1, 0.4)
		_driver_lap_results.add_child(empty_lbl)
		return

	if has_ideal:
		_add_driver_lap_header("THEORETICAL BEST")
		_add_ideal_lap_row(_ideal_lap_info)
		if not live.is_empty() or not historical.is_empty():
			_driver_lap_results.add_child(HSeparator.new())
	if not live.is_empty():
		_add_driver_lap_header("LIVE")
		for entry in live:
			_add_driver_lap_row(entry)
	if not historical.is_empty():
		if not live.is_empty():
			_driver_lap_results.add_child(HSeparator.new())
		_add_driver_lap_header("LAP TIMES — fastest first")
		for entry in historical:
			_add_driver_lap_row(entry)

# Gives a dynamically-built list-row Button a soft rounded card background
# instead of the plain flat/text-only look a bare Button defaults to — rows
# in the driver/lap picker otherwise read as a wall of plain text lines.
func _style_list_row(btn: Button) -> void:
	btn.flat = false
	btn.custom_minimum_size.y = 30
	var normal := StyleBoxFlat.new()
	normal.bg_color = Color(1, 1, 1, 0.035)
	normal.set_corner_radius_all(7)
	normal.content_margin_left = 10.0
	normal.content_margin_right = 10.0
	normal.content_margin_top = 5.0
	normal.content_margin_bottom = 5.0
	btn.add_theme_stylebox_override("normal", normal)
	var hover := normal.duplicate()
	hover.bg_color = Color(1, 1, 1, 0.085)
	btn.add_theme_stylebox_override("hover", hover)
	btn.add_theme_stylebox_override("pressed", hover)

func _add_driver_lap_header(text: String) -> void:
	var lbl := Label.new()
	lbl.text = text
	lbl.add_theme_font_size_override("font_size", 10)
	lbl.modulate = Color(1, 1, 1, 0.45)
	_driver_lap_results.add_child(lbl)

func _add_ideal_lap_row(info: Dictionary) -> void:
	var s1: Dictionary = info.get("sector1", {})
	var s2: Dictionary = info.get("sector2", {})
	var s3: Dictionary = info.get("sector3", {})
	var breakdown := "S1 %s L%d · S2 %s L%d · S3 %s L%d" % [
		str(s1.get("driver", "?")), int(s1.get("lap_number", 0)),
		str(s2.get("driver", "?")), int(s2.get("lap_number", 0)),
		str(s3.get("driver", "?")), int(s3.get("lap_number", 0)),
	]

	# Two lines (time, then the per-sector breakdown) rather than one long
	# clipped Button label — the breakdown is the whole point of this row,
	# so it stays readable even in a narrower dialog.
	var box := VBoxContainer.new()
	box.add_theme_constant_override("separation", 0)

	var btn := Button.new()
	btn.alignment = HORIZONTAL_ALIGNMENT_LEFT
	btn.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	btn.clip_text = true
	btn.text = "★ Ideal Lap — %s" % _format_lap_time(int(info.get("ideal_time_ms", 0)))
	btn.tooltip_text = breakdown
	btn.pressed.connect(_on_ideal_lap_row_selected.bind(info))
	_style_list_row(btn)
	box.add_child(btn)

	var detail := Label.new()
	detail.text = "   " + breakdown
	detail.add_theme_font_size_override("font_size", 10)
	detail.modulate = Color(1, 1, 1, 0.5)
	detail.clip_text = true
	box.add_child(detail)

	_driver_lap_results.add_child(box)

func _on_ideal_lap_row_selected(info: Dictionary) -> void:
	_current_driver      = "Ideal Lap"
	_current_lap         = 0
	_current_is_live     = false
	_current_is_ideal    = true
	_current_session_id  = ""
	_current_time_ms     = int(info.get("ideal_time_ms", 0))
	_selected_label.text = "Selected: ★ Ideal Lap  %s" % _format_lap_time(_current_time_ms)
	_check_add_ready()

func _add_driver_lap_row(entry: Dictionary) -> void:
	var btn := Button.new()
	btn.alignment = HORIZONTAL_ALIGNMENT_LEFT
	btn.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	btn.clip_text = true
	if entry["is_live"]:
		btn.text = "● %s — Lap %d (live)" % [entry["driver"], entry["lap"]]
	else:
		btn.text = "%s — Lap %d — %s" % [entry["driver"], entry["lap"], _format_lap_time(entry["time_ms"])]
	btn.tooltip_text = btn.text
	btn.pressed.connect(_on_driver_lap_row_selected.bind(entry))
	_style_list_row(btn)
	_driver_lap_results.add_child(btn)

func _on_driver_lap_row_selected(entry: Dictionary) -> void:
	_current_driver      = entry["driver"]
	_current_lap         = entry["lap"]
	_current_is_live     = entry["is_live"]
	_current_is_ideal    = false
	_current_session_id  = entry["session_id"]
	_current_time_ms     = int(entry["time_ms"])
	if _current_is_live:
		_selected_label.text = "Selected: ● %s  L%d (live)" % [entry["driver"], entry["lap"]]
	else:
		_selected_label.text = "Selected: %s  L%d  %s" % [entry["driver"], entry["lap"], _format_lap_time(entry["time_ms"])]
	# Dialog stays open — Add commits this pick without closing, so several
	# drivers/laps can be added in one sitting (see _on_telemetry_received).
	_check_add_ready()

func _on_telemetry_received(data: PackedByteArray) -> void:
	var driver := _current_driver
	var lap    := _current_lap
	var key    := "%s#%d" % [driver, lap]
	var color  := _COLORS[_active_series.size() % _COLORS.size()]

	var list_item := _make_driver_list_item(key, false)
	_driver_tab_list.add_child(list_item)
	var ref_btn: Button = list_item.get_node("col/reference_toggle")
	var all_items: Array = [list_item]
	all_items.append_array(_add_series_to_panes(key, color, data))

	_active_series[key] = {
		color = color, data = data, hidden = false, is_live = false, all_items = all_items,
		time_ms = _current_time_ms, ref_btn = ref_btn,
	}
	_ensure_reference_default()

	# Keep game; reset track/lap/driver for the next comparison.
	_current_track = ""
	_current_session_id = ""
	_track_toggle.text = "Track: (choose)  ▾"
	_render_track_list()
	_hide_driver_lap_section()
	_reset_driver_lap_selection()

# ── Dropdown selections ──────────────────────────────────────────────────────

# Renders the distinct games seen across every recorded session as clickable
# rows inside the collapsible game_scroll section (toggled by game_toggle).
# Not a MenuButton — a MenuButton's dropdown is itself a popup Window, and
# nesting one inside the add_driver_dialog Window was unreliable.
func _render_game_list() -> void:
	var seen := {}
	var games: Array[String] = []
	for s in _all_sessions:
		if not seen.has(s.game):
			seen[s.game] = true
			games.append(s.game)
	_render_simple_list(_game_list, games, "No sessions recorded yet", _on_game_row_selected)

func _on_game_row_selected(game: String) -> void:
	_current_game = game
	_game_toggle.text = "Game: %s  ▾" % game
	_game_scroll.visible = false
	_track_toggle.text = "Track: (choose)  ▾"
	_render_track_list()
	_hide_driver_lap_section()
	_reset_driver_lap_selection()

# Tracks are just distinct names for the current game — session resolution
# happens per-lap now (see _on_lap_times_received), not per-track, since a
# track can have many recorded sessions.
func _render_track_list() -> void:
	var seen := {}
	var tracks: Array[String] = []
	for s in _all_sessions:
		if s.game == _current_game and not seen.has(s.track):
			seen[s.track] = true
			tracks.append(s.track)
	_render_simple_list(_track_list, tracks, "No tracks recorded for this game yet", _on_track_row_selected)

func _on_track_row_selected(track: String) -> void:
	_current_track = track
	_track_toggle.text = "Track: %s  ▾" % track
	_track_scroll.visible = false
	_show_driver_lap_section()
	_reset_driver_lap_selection()

	_api.fetch_lap_times(_current_game, _current_track)
	_api.fetch_live_drivers(_current_game, _current_track)
	_api.fetch_ideal_lap_info(_current_game, _current_track)

# The Driver/Lap section (search + results) only makes sense once a track is
# picked — kept hidden until then, and re-hidden whenever the track resets
# (game change, or a completed Add — see _on_telemetry_received), so the
# dialog doesn't show a search box with nothing meaningful to search yet.
func _show_driver_lap_section() -> void:
	_driver_lap_label.visible = true
	_driver_lap_search.visible = true
	_results_scroll.visible = true
	_driver_lap_search.grab_focus()

func _hide_driver_lap_section() -> void:
	_driver_lap_label.visible = false
	_driver_lap_search.visible = false
	_results_scroll.visible = false

# Shared renderer for the Game/Track lists — same flat "list of clickable rows"
# pattern as the Driver/Lap results list below, just without search filtering.
func _render_simple_list(container: VBoxContainer, items: Array[String], empty_text: String, on_pick: Callable) -> void:
	if items.is_empty():
		_show_list_placeholder(container, empty_text)
		return
	for c in container.get_children():
		c.queue_free()
	for item in items:
		var btn := Button.new()
		btn.alignment = HORIZONTAL_ALIGNMENT_LEFT
		btn.size_flags_horizontal = Control.SIZE_EXPAND_FILL
		btn.clip_text = true
		btn.text = item
		btn.pressed.connect(on_pick.bind(item))
		_style_list_row(btn)
		container.add_child(btn)

func _show_list_placeholder(container: VBoxContainer, text: String) -> void:
	for c in container.get_children():
		c.queue_free()
	var lbl := Label.new()
	lbl.text = text
	lbl.modulate = Color(1, 1, 1, 0.4)
	container.add_child(lbl)

func _on_add_pressed() -> void:
	if _current_is_ideal:
		_api.fetch_ideal_lap_telemetry(_current_game, _current_track)
	elif _current_is_live:
		_start_live_watch(_current_driver, _current_lap, _current_session_id)
	else:
		_api.fetch_telemetry(_current_game, _current_track, _current_lap, _current_driver, _current_session_id)

# ── Live streaming ───────────────────────────────────────────────────────────

# Only one live watch at a time — the WebSocket client supports a single
# stream. Switching to a new live driver drops whichever one was running.
func _start_live_watch(driver: String, lap: int, session_id: String) -> void:
	if _live_key != "":
		_remove_driver(_live_key)

	var key := "%s#live" % driver
	_live_key = key
	_live_lap = lap
	var color := _COLORS[_active_series.size() % _COLORS.size()]

	var list_item := _make_driver_list_item(key, true)
	_driver_tab_list.add_child(list_item)
	var all_items: Array = [list_item]
	all_items.append_array(_add_series_to_panes(key, color, ChartCanvas.empty_raw_blob()))

	_active_series[key] = {color = color, data = ChartCanvas.empty_raw_blob(), hidden = false, is_live = true, all_items = all_items}

	_api.connect_stream(_current_game, _current_track, 0, driver, session_id)  # lap 0 = live-follow
	_reset_driver_lap_selection()

# Incoming rows carry lap_num directly, so rollover is detected client-side —
# no need for the backend to understand "current lap" as a concept.
func _on_stream_frame(data: PackedByteArray) -> void:
	if _live_key == "" or data.size() < 4:
		return
	var row_count: int = data.decode_u32(0)
	if row_count == 0:
		return

	var header := 4
	var split_i := row_count  # first row index belonging to a new lap; == row_count means no rollover
	for i in range(row_count):
		var base := header + i * TelemetryDecoder.ROW_SIZE
		var lap_num: int = data.decode_s16(base + TelemetryDecoder.F_LAP_NUM)
		if lap_num != _live_lap:
			split_i = i
			break

	if split_i > 0:
		_append_live_bytes(data.slice(header, header + split_i * TelemetryDecoder.ROW_SIZE))

	if split_i < row_count:
		_clear_live_series()
		var new_base := header + split_i * TelemetryDecoder.ROW_SIZE
		_live_lap = data.decode_s16(new_base + TelemetryDecoder.F_LAP_NUM)
		_append_live_bytes(data.slice(new_base, data.size()))

func _append_live_bytes(row_bytes: PackedByteArray) -> void:
	if row_bytes.is_empty() or not _active_series.has(_live_key):
		return
	for pane in get_tree().get_nodes_in_group("chart_panes"):
		pane.append_series_rows(_live_key, row_bytes)

# Lap ended: clear this driver's plotted data everywhere and keep streaming
# into the same board — no need to re-add or reconfigure anything.
func _clear_live_series() -> void:
	if not _active_series.has(_live_key):
		return
	for pane in get_tree().get_nodes_in_group("chart_panes"):
		pane.clear_series_data(_live_key)

# ── Helpers ──────────────────────────────────────────────────────────────────

const _REF_ACTIVE_COLOR   := Color(1.0, 0.85, 0.2, 1.0)
const _REF_INACTIVE_COLOR := Color(1.0, 1.0, 1.0, 0.4)

# Makes the pin readable at a glance: bright gold while pinned, dim otherwise
# (theme_override_styles on reference_toggle are all empty, same as
# visibility_toggle, so there's no built-in pressed-state visual difference).
func _style_reference_button(btn: Button, active: bool) -> void:
	btn.modulate = _REF_ACTIVE_COLOR if active else _REF_INACTIVE_COLOR

# Auto-picks the quickest non-live active series as the reference if none is
# pinned yet, so a "Time Delta" box doesn't sit empty until the user manually
# picks one. No-op once any reference exists — including a previously
# auto-picked one, which stays sticky rather than getting silently replaced
# by a faster lap added later — or if nothing eligible is on the board yet.
func _ensure_reference_default() -> void:
	if _reference_key != "":
		return
	var best_key := ""
	var best_time := 0
	for raw_key in _active_series:
		var key: String = str(raw_key)
		var s: Dictionary = _active_series[key]
		if s["is_live"]:
			continue
		var t: int = int(s.get("time_ms", 0))
		if t <= 0:
			continue
		if best_key == "" or t < best_time:
			best_key = key
			best_time = t
	if best_key != "":
		_set_reference(best_key, _active_series[best_key]["ref_btn"])

# Row shown in the sidebar's Drivers tab: show/hide toggle, name, remove, and
# (for a non-live series) a "Reference lap" toggle to set it as the delta
# chart's baseline. It's the one place a series can be taken off (or hidden
# from) the board.
func _make_driver_list_item(key: String, is_live: bool) -> Control:
	var item: Control = DriverListItem.instantiate()
	item.get_node("col/row/driver_name").text = _format_driver_label(key)

	var vis_btn: UiIconButton = item.get_node("col/row/visibility_toggle")
	vis_btn.button_pressed = false  # not pressed = visible
	vis_btn.toggled.connect(func(hidden: bool) -> void:
		vis_btn.icon_name = "eye_off" if hidden else "eye"
		_on_series_visibility_toggled(key, hidden)
	)

	item.get_node("remove_button").pressed.connect(func() -> void:
		_remove_driver(key)
	)

	var ref_btn: Button = item.get_node("col/reference_toggle")
	if is_live:
		# A live lap keeps growing, and the delta reference curve is only
		# ever built once (see ChartCanvas.add_delta_series) — it can be
		# compared against a reference, but it can't be one.
		ref_btn.disabled = true
		ref_btn.tooltip_text = "A live lap can't be the delta reference"
	else:
		_style_reference_button(ref_btn, false)
		ref_btn.toggled.connect(func(pressed: bool) -> void:
			if pressed:
				_set_reference(key, ref_btn)
			elif _reference_key == key:
				# Re-clicking the current reference's own button shouldn't
				# unpin it — that would clear every delta line for no
				# reason. Pin a different driver to change the reference.
				ref_btn.set_pressed_no_signal(true)
		)
	return item

# Pins `key` as the delta chart's reference lap. Every series stays plotted
# on the delta chart, including the reference itself — its line just
# collapses to flat zero, which is what makes it read as "the baseline" the
# other lines are measured against, rather than one of them dropping off.
func _set_reference(key: String, btn: Button) -> void:
	if _reference_key == key:
		return
	var had_reference := _reference_key != ""
	if _reference_btn != null and is_instance_valid(_reference_btn):
		_reference_btn.set_pressed_no_signal(false)
		_style_reference_button(_reference_btn, false)
	_reference_key = key
	_reference_btn = btn
	btn.set_pressed_no_signal(true)
	_style_reference_button(btn, true)

	var new_ref_raw: PackedByteArray = _active_series[key]["data"]
	for pane: ChartPane in get_tree().get_nodes_in_group("chart_panes"):
		if not pane.has_metric(ChartCanvas.DELTA_FIELD):
			continue
		if had_reference:
			# Every series already has a delta line (including `key`, whose
			# own line just collapses to flat zero once re-based onto itself).
			pane.rebase_delta_series(new_ref_raw)
		else:
			# First pin ever for this pane: nothing has a delta line yet.
			_backfill_delta_pane(pane)

# Unpins the reference lap and clears every delta line, since they're all
# measured against a reference that no longer applies.
func _clear_reference() -> void:
	_reference_key = ""
	_reference_btn = null
	for pane in get_tree().get_nodes_in_group("chart_panes"):
		if pane.has_metric(ChartCanvas.DELTA_FIELD):
			pane.clear_all_delta_series()

# Removes a driver/lap series from the Drivers tab, every chart pane's legend,
# and every plotted line for it. Disconnects the stream if it's the live one.
func _remove_driver(key: String) -> void:
	if not _active_series.has(key):
		return
	for item in _active_series[key]["all_items"]:
		if is_instance_valid(item):
			item.queue_free()
	for pane in get_tree().get_nodes_in_group("chart_panes"):
		pane.remove_series_everywhere(key)
	_active_series.erase(key)
	if key == _live_key:
		_api.disconnect_stream()
		_live_key = ""
		_live_lap = 0
	if key == _reference_key:
		_clear_reference()

func _on_series_visibility_toggled(key: String, hidden: bool) -> void:
	if not _active_series.has(key):
		return
	_active_series[key]["hidden"] = hidden
	for pane in get_tree().get_nodes_in_group("chart_panes"):
		pane.set_series_visible(key, not hidden)

func _check_add_ready() -> void:
	_dialog_add_btn.disabled = (
		_current_game.is_empty() or _current_track.is_empty()
		or (_current_lap == 0 and not _current_is_ideal) or _current_driver.is_empty()
	)

func _reset_driver_lap_selection() -> void:
	_current_lap = 0
	_current_driver = ""
	_current_is_live = false
	_current_is_ideal = false
	_current_session_id = ""
	_current_time_ms = 0
	_lap_time_choices.clear()
	_live_drivers.clear()
	_driver_lap_choices.clear()
	_ideal_lap_info = {}
	_driver_lap_search.text = ""
	for c in _driver_lap_results.get_children():
		c.queue_free()
	_selected_label.text = "No driver/lap selected"
	_dialog_add_btn.disabled = true

func _format_lap_time(ms: int) -> String:
	if ms <= 0:
		return "--:--.---"
	var minutes := ms / 60000
	var seconds := (ms / 1000) % 60
	var millis  := ms % 1000
	return "%d:%02d.%03d" % [minutes, seconds, millis]
