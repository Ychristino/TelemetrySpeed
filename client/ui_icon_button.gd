class_name UiIconButton
extends Button
# A Button that draws a small vector icon instead of text — used in place of
# emoji glyphs (👁/📌/🚫), which render as full-color OS emoji and clash with
# this app's flat, monochrome dark theme. Colors follow the button's own
# theme font colors, so hover/disabled/pressed states behave exactly like a
# text button would; callers that want a semantic color (e.g. the pinned
# reference button) still just set `modulate`, same as before.

@export var icon_name: String = "eye":
	set(v):
		icon_name = v
		queue_redraw()

func _ready() -> void:
	text = ""
	flat = true

func _draw() -> void:
	var col: Color = get_theme_color("font_disabled_color", "Button") if disabled \
		else get_theme_color("font_color", "Button")
	var c := size / 2.0
	match icon_name:
		"eye":
			_draw_eye(c, col, false)
		"eye_off":
			_draw_eye(c, col, true)
		"pin":
			_draw_pin(c, col)

func _draw_eye(c: Vector2, col: Color, slashed: bool) -> void:
	# Hidden state dims the eye itself (not just a slash through it) so a row
	# reads as "off" at a glance instead of needing to spot one thin line.
	var eye_col := col
	if slashed:
		eye_col = Color(col.r, col.g, col.b, col.a * 0.45)

	var w: float = size.x * 0.32
	var h: float = size.y * 0.20
	var pts := PackedVector2Array()
	const N := 12
	for i in range(N + 1):
		var t: float = float(i) / N
		pts.append(c + Vector2(lerp(-w, w, t), -h * sin(PI * t)))
	for i in range(N + 1):
		var t: float = float(i) / N
		pts.append(c + Vector2(lerp(w, -w, t), h * sin(PI * t)))
	draw_polyline(pts, eye_col, 1.6, true)
	draw_circle(c, h * 0.5, eye_col)
	if slashed:
		draw_line(c + Vector2(-w * 1.1, -h * 1.8), c + Vector2(w * 1.1, h * 1.8), col, 1.7, true)

func _draw_pin(c: Vector2, col: Color) -> void:
	var r: float = size.x * 0.19
	var top: Vector2 = c + Vector2(0, -size.y * 0.20)
	draw_circle(top, r, col)
	var pts := PackedVector2Array([
		top + Vector2(-r * 0.75, r * 0.4),
		top + Vector2(r * 0.75, r * 0.4),
		c + Vector2(0, size.y * 0.34),
	])
	draw_colored_polygon(pts, col)
