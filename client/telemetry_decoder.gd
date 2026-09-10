class_name TelemetryDecoder

# Binary format produced by the Go backend (little-endian).
#
# Every row starts with a fixed header — ts int64, car_index int16,
# lap_num int16, lap_distance float32 (16 bytes) — followed by one value per
# field in DEFAULT_FIELD_NAMES, in that order. This client always requests
# exactly DEFAULT_FIELD_NAMES (see api_client.gd), so the response field
# order always matches the request order and ROW_SIZE below is fixed.
#
# HTTP response:    [0x54, 0x4D, version=2, field_count u8, field_id×N u8,
#                     row_count u32, rows...]
# WebSocket frame:  [row_count u32, rows...] — the field list was already
#                     confirmed once via the stream's JSON ack (see
#                     api_client.gd's stream_fields_received).
#
# If DEFAULT_FIELD_NAMES ever changes, the offsets and ROW_SIZE below must
# be recomputed to match — see internal/database/fields.go on the backend
# for each field's type/width.

# The exact field set (and order) this client requests for every lap fetch
# and live stream — matches every metric field referenced by main_menu.gd's
# GROUPS, so any of them can be charted from a single fetch. Nothing else
# (MotionEx, raw per-wheel breakdowns, position/velocity, etc.) is requested,
# since nothing here plots it.
const DEFAULT_FIELD_NAMES: PackedStringArray = [
	"throttle", "brake", "steering", "clutch", "gear", "drs",
	"speed", "g_lat", "g_long", "g_vert",
	"rpm", "eng_temp", "engine_power_ice", "engine_power_mguk", "ers_store", "fuel", "fuel_rem_laps",
	"tyre_surf_avg", "tyre_inner_avg", "tyre_press_avg", "tyre_wear_avg", "brake_temp_avg",
]

const ROW_SIZE := 100

# HTTP header size: magic(2) + version(1) + field_count(1) + field_id×N +
# row_count(4), where N = DEFAULT_FIELD_NAMES.size() (22 entries) since this
# client always requests exactly that set — kept as a literal rather than a
# computed const so it can't silently drift if DEFAULT_FIELD_NAMES changes
# without updating this. WS frames have no field header (just row_count,
# 4 bytes) — the field list arrives once as the stream's JSON ack instead.
const HTTP_HEADER_SIZE := 30  # 8 + DEFAULT_FIELD_NAMES.size()

# Fixed header — always present regardless of which fields were requested.
const F_TS           := 0   # int64  — unix nanoseconds
const F_CAR_INDEX    := 8   # int16
const F_LAP_NUM      := 10  # int16
const F_LAP_DISTANCE := 12  # float32

# Offsets of DEFAULT_FIELD_NAMES within the row, starting after the header.
const F_THROTTLE          := 16  # float32 — 0..1
const F_BRAKE             := 20  # float32 — 0..1
const F_STEERING          := 24  # float32 — -1..1
const F_CLUTCH            := 28  # float32 — 0..1
const F_GEAR              := 32  # int16
const F_DRS               := 34  # int16
const F_SPEED             := 36  # float32 — km/h
const F_G_LAT             := 40  # float32
const F_G_LONG            := 44  # float32
const F_G_VERT            := 48  # float32
const F_RPM               := 52  # int32
const F_ENG_TEMP          := 56  # float32
const F_ENGINE_POWER_ICE  := 60  # float32
const F_ENGINE_POWER_MGUK := 64  # float32
const F_ERS_STORE         := 68  # float32
const F_FUEL              := 72  # float32
const F_FUEL_REM_LAPS     := 76  # float32
const F_TYRE_SURF_AVG     := 80  # float32
const F_TYRE_INNER_AVG    := 84  # float32
const F_TYRE_PRESS_AVG    := 88  # float32
const F_TYRE_WEAR_AVG     := 92  # float32
const F_BRAKE_TEMP_AVG    := 96  # float32

# Extract a single field value from raw bytes by field name.
static func field(data: PackedByteArray, base: int, field_name: String) -> float:
	match field_name:
		"speed":             return data.decode_float(base + F_SPEED)
		"throttle":          return data.decode_float(base + F_THROTTLE)
		"brake":              return data.decode_float(base + F_BRAKE)
		"steering":          return data.decode_float(base + F_STEERING)
		"clutch":            return data.decode_float(base + F_CLUTCH)
		"gear":              return float(data.decode_s16(base + F_GEAR))
		"drs":               return float(data.decode_s16(base + F_DRS))
		"g_lat":             return data.decode_float(base + F_G_LAT)
		"g_long":            return data.decode_float(base + F_G_LONG)
		"g_vert":            return data.decode_float(base + F_G_VERT)
		"rpm":               return float(data.decode_s32(base + F_RPM))
		"eng_temp":          return data.decode_float(base + F_ENG_TEMP)
		"engine_power_ice":  return data.decode_float(base + F_ENGINE_POWER_ICE)
		"engine_power_mguk": return data.decode_float(base + F_ENGINE_POWER_MGUK)
		"ers_store":         return data.decode_float(base + F_ERS_STORE)
		"fuel":              return data.decode_float(base + F_FUEL)
		"fuel_rem_laps":     return data.decode_float(base + F_FUEL_REM_LAPS)
		"tyre_surf_avg":     return data.decode_float(base + F_TYRE_SURF_AVG)
		"tyre_inner_avg":    return data.decode_float(base + F_TYRE_INNER_AVG)
		"tyre_press_avg":    return data.decode_float(base + F_TYRE_PRESS_AVG)
		"tyre_wear_avg":     return data.decode_float(base + F_TYRE_WEAR_AVG)
		"brake_temp_avg":    return data.decode_float(base + F_BRAKE_TEMP_AVG)
		_:
			return 0.0

# Convert raw bytes into a chart-ready {x, y} array for a given field.
# x_mode "distance" (default) uses lap_distance in meters; "time" uses
# elapsed seconds since the first row, from the raw ts timestamps.
static func to_chart_points(data: PackedByteArray, field_name: String, x_mode: String = "distance", is_http: bool = true) -> Array:
	var header := HTTP_HEADER_SIZE if is_http else 4
	if data.size() < header:
		return []
	var row_count: int = data.decode_u32(header - 4)
	var points: Array = []
	points.resize(row_count)
	var t0: int = 0
	if x_mode == "time" and row_count > 0:
		t0 = data.decode_s64(header + F_TS)
	for i in range(row_count):
		var base := header + i * ROW_SIZE
		var x: float
		if x_mode == "time":
			x = float(data.decode_s64(base + F_TS) - t0) / 1_000_000_000.0
		else:
			x = data.decode_float(base + F_LAP_DISTANCE)
		points[i] = {
			x = x,
			y = field(data, base, field_name)
		}
	return points

# Computes time delta (seconds) between `data` (a compared lap) and
# `reference` (the pinned reference lap), sampled at each row of `data` and
# interpolated against reference's time-at-distance curve. Positive means
# `data` is slower than reference at that point on track (time lost so far);
# negative means faster (time gained). Both blobs must be full HTTP-format
# lap fetches — a still-growing live blob can be the compared lap but never
# the reference, since the reference curve is built once up front here and
# never rechecked mid-walk.
static func to_delta_chart_points(data: PackedByteArray, reference: PackedByteArray, x_mode: String = "distance") -> Array:
	if data.size() < HTTP_HEADER_SIZE or reference.size() < HTTP_HEADER_SIZE:
		return []
	var row_count: int = data.decode_u32(HTTP_HEADER_SIZE - 4)
	var ref_count: int = reference.decode_u32(HTTP_HEADER_SIZE - 4)
	if ref_count < 2 or row_count == 0:
		return []

	# Reference lap's time-at-distance curve, as parallel arrays so the walk
	# below can interpolate without re-decoding reference bytes per sample.
	var ref_dist := PackedFloat32Array()
	var ref_time := PackedFloat32Array()
	ref_dist.resize(ref_count)
	ref_time.resize(ref_count)
	var ref_t0: int = reference.decode_s64(HTTP_HEADER_SIZE + F_TS)
	for i in range(ref_count):
		var rb := HTTP_HEADER_SIZE + i * ROW_SIZE
		ref_dist[i] = reference.decode_float(rb + F_LAP_DISTANCE)
		ref_time[i] = float(reference.decode_s64(rb + F_TS) - ref_t0) / 1_000_000_000.0

	var data_t0: int = data.decode_s64(HTTP_HEADER_SIZE + F_TS)
	var points: Array = []
	points.resize(row_count)
	# Walks forward only: both curves progress with distance, so once row i
	# is bracketed, row i+1 (a larger or equal distance) can only need the
	# same bracket or one further along — O(n+m), not O(n·m).
	var ref_i := 0
	for i in range(row_count):
		var base := HTTP_HEADER_SIZE + i * ROW_SIZE
		var dist: float = data.decode_float(base + F_LAP_DISTANCE)
		var data_time: float = float(data.decode_s64(base + F_TS) - data_t0) / 1_000_000_000.0

		while ref_i < ref_count - 2 and ref_dist[ref_i + 1] < dist:
			ref_i += 1
		var d0: float = ref_dist[ref_i]
		var d1: float = ref_dist[ref_i + 1]
		var ref_time_at_dist: float
		if d1 > d0:
			var t: float = clamp((dist - d0) / (d1 - d0), 0.0, 1.0)
			ref_time_at_dist = lerp(ref_time[ref_i], ref_time[ref_i + 1], t)
		else:
			ref_time_at_dist = ref_time[ref_i]

		var x: float = data_time if x_mode == "time" else dist
		points[i] = { x = x, y = data_time - ref_time_at_dist }
	return points
