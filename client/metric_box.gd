class_name MetricBox
extends PanelContainer

signal close_pressed
signal activated

@onready var _title: Label              = $VBoxContainer/header/title
@onready var _close_btn: Button         = $VBoxContainer/header/close_btn
@onready var driver_list: VBoxContainer = $VBoxContainer/driver_list

func _ready() -> void:
	_close_btn.pressed.connect(func(): close_pressed.emit())

# Clicking anywhere on the box (except the close button, which consumes its
# own click) makes this the active metric.
func _gui_input(event: InputEvent) -> void:
	if event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT:
		activated.emit()

func set_label(text: String) -> void:
	_title.text = text

func set_active(active: bool) -> void:
	modulate = Color(1, 1, 1, 1.0) if active else Color(1, 1, 1, 0.55)
