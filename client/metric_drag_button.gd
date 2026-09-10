class_name MetricDragButton
extends Button

var field := ""
var metric_label := ""

func _get_drag_data(_at_position: Vector2) -> Variant:
	var preview := Label.new()
	preview.text = metric_label
	preview.add_theme_color_override("font_color", Color(1, 1, 1))
	preview.add_theme_color_override("font_shadow_color", Color(0, 0, 0, 0.8))
	preview.add_theme_constant_override("shadow_offset_x", 1)
	preview.add_theme_constant_override("shadow_offset_y", 1)
	set_drag_preview(preview)
	return {"type": "metric", "field": field, "label": metric_label}
