package database

import "fmt"

// FieldType is the wire/SQL scalar type of one selectable telemetry field.
type FieldType uint8

const (
	TypeF32 FieldType = iota
	TypeI16
	TypeI32
)

// Size returns the encoded width in bytes for the type.
func (t FieldType) Size() int {
	if t == TypeI16 {
		return 2
	}
	return 4
}

// FieldDef describes one field a caller can request via QueryTelemetry /
// PollTelemetry's `fields` list. ID is the stable wire identifier sent in
// the HTTP response header — never renumber or reuse an ID once shipped,
// since client decoders key off it. New fields must be appended to
// fieldRegistry below (never inserted), so IDs stay stable across releases.
//
// Col is a SQL expression (already qualified, e.g. "t.speed" or an average
// over the four corners) selected verbatim — safe because it always comes
// from this fixed registry, never from request input.
type FieldDef struct {
	ID   uint8
	Name string
	Col  string
	Type FieldType
}

// fieldRegistry enumerates every optional telemetry field. ts, car_index,
// lap_num, and lap_distance are NOT here — they're structural (needed for
// the time/distance X axis and for detecting lap rollover in a live stream)
// and are always included ahead of these in every row, regardless of what a
// caller requests.
var fieldRegistry = []FieldDef{
	{Name: "speed", Col: "t.speed", Type: TypeF32},
	{Name: "throttle", Col: "t.throttle", Type: TypeF32},
	{Name: "brake", Col: "t.brake", Type: TypeF32},
	{Name: "steering", Col: "t.steering", Type: TypeF32},
	{Name: "gear", Col: "t.gear", Type: TypeI16},
	{Name: "clutch", Col: "t.clutch", Type: TypeF32},
	{Name: "drs", Col: "t.drs", Type: TypeI16},

	{Name: "pos_x", Col: "t.pos_x", Type: TypeF32},
	{Name: "pos_y", Col: "t.pos_y", Type: TypeF32},
	{Name: "pos_z", Col: "t.pos_z", Type: TypeF32},
	{Name: "vel_x", Col: "t.vel_x", Type: TypeF32},
	{Name: "vel_y", Col: "t.vel_y", Type: TypeF32},
	{Name: "vel_z", Col: "t.vel_z", Type: TypeF32},
	{Name: "yaw", Col: "t.yaw", Type: TypeF32},
	{Name: "pitch", Col: "t.pitch", Type: TypeF32},
	{Name: "roll", Col: "t.roll", Type: TypeF32},

	{Name: "g_lat", Col: "t.g_lat", Type: TypeF32},
	{Name: "g_long", Col: "t.g_long", Type: TypeF32},
	{Name: "g_vert", Col: "t.g_vert", Type: TypeF32},

	{Name: "rpm", Col: "t.rpm", Type: TypeI32},
	{Name: "eng_temp", Col: "t.eng_temp", Type: TypeF32},
	{Name: "fuel", Col: "t.fuel", Type: TypeF32},
	{Name: "fuel_rem_laps", Col: "t.fuel_rem_laps", Type: TypeF32},
	{Name: "fuel_mix", Col: "t.fuel_mix", Type: TypeI16},
	{Name: "pit_limiter", Col: "t.pit_limiter", Type: TypeI16},
	{Name: "engine_power_ice", Col: "t.engine_power_ice", Type: TypeF32},
	{Name: "engine_power_mguk", Col: "t.engine_power_mguk", Type: TypeF32},

	{Name: "ers_store", Col: "t.ers_store", Type: TypeF32},
	{Name: "ers_deploy_mode", Col: "t.ers_deploy_mode", Type: TypeI16},

	{Name: "traction_control", Col: "t.traction_control", Type: TypeI16},
	{Name: "abs", Col: "t.abs", Type: TypeI16},

	{Name: "tyre_compound", Col: "t.tyre_compound", Type: TypeI16},
	{Name: "tyre_age_laps", Col: "t.tyre_age_laps", Type: TypeI16},

	{Name: "tyre_surf_temp_rl", Col: "t.tyre_surf_temp_rl", Type: TypeF32},
	{Name: "tyre_surf_temp_rr", Col: "t.tyre_surf_temp_rr", Type: TypeF32},
	{Name: "tyre_surf_temp_fl", Col: "t.tyre_surf_temp_fl", Type: TypeF32},
	{Name: "tyre_surf_temp_fr", Col: "t.tyre_surf_temp_fr", Type: TypeF32},
	{Name: "tyre_inner_temp_rl", Col: "t.tyre_inner_temp_rl", Type: TypeF32},
	{Name: "tyre_inner_temp_rr", Col: "t.tyre_inner_temp_rr", Type: TypeF32},
	{Name: "tyre_inner_temp_fl", Col: "t.tyre_inner_temp_fl", Type: TypeF32},
	{Name: "tyre_inner_temp_fr", Col: "t.tyre_inner_temp_fr", Type: TypeF32},
	{Name: "tyre_pressure_rl", Col: "t.tyre_pressure_rl", Type: TypeF32},
	{Name: "tyre_pressure_rr", Col: "t.tyre_pressure_rr", Type: TypeF32},
	{Name: "tyre_pressure_fl", Col: "t.tyre_pressure_fl", Type: TypeF32},
	{Name: "tyre_pressure_fr", Col: "t.tyre_pressure_fr", Type: TypeF32},
	{Name: "tyre_wear_rl", Col: "t.tyre_wear_rl", Type: TypeF32},
	{Name: "tyre_wear_rr", Col: "t.tyre_wear_rr", Type: TypeF32},
	{Name: "tyre_wear_fl", Col: "t.tyre_wear_fl", Type: TypeF32},
	{Name: "tyre_wear_fr", Col: "t.tyre_wear_fr", Type: TypeF32},
	{Name: "tyre_blister_rl", Col: "t.tyre_blister_rl", Type: TypeI16},
	{Name: "tyre_blister_rr", Col: "t.tyre_blister_rr", Type: TypeI16},
	{Name: "tyre_blister_fl", Col: "t.tyre_blister_fl", Type: TypeI16},
	{Name: "tyre_blister_fr", Col: "t.tyre_blister_fr", Type: TypeI16},

	// Averaged-across-4-corners convenience fields — computed in SQL so a
	// caller that only wants "the tyre surf temp line" doesn't have to pay
	// for four raw columns (and four decodes) to get it. The trailing
	// ::real cast matters: Postgres promotes `real / integer` to `double
	// precision`, so without it pgx scans these as Go float64 instead of
	// the float32 this registry (and the wire format) declares.
	{Name: "tyre_surf_avg", Col: "((t.tyre_surf_temp_rl + t.tyre_surf_temp_rr + t.tyre_surf_temp_fl + t.tyre_surf_temp_fr) / 4)::real", Type: TypeF32},
	{Name: "tyre_inner_avg", Col: "((t.tyre_inner_temp_rl + t.tyre_inner_temp_rr + t.tyre_inner_temp_fl + t.tyre_inner_temp_fr) / 4)::real", Type: TypeF32},
	{Name: "tyre_press_avg", Col: "((t.tyre_pressure_rl + t.tyre_pressure_rr + t.tyre_pressure_fl + t.tyre_pressure_fr) / 4)::real", Type: TypeF32},
	{Name: "tyre_wear_avg", Col: "((t.tyre_wear_rl + t.tyre_wear_rr + t.tyre_wear_fl + t.tyre_wear_fr) / 4)::real", Type: TypeF32},
	{Name: "brake_temp_avg", Col: "((t.brake_temp_rl + t.brake_temp_rr + t.brake_temp_fl + t.brake_temp_fr) / 4)::real", Type: TypeF32},

	{Name: "brake_temp_rl", Col: "t.brake_temp_rl", Type: TypeF32},
	{Name: "brake_temp_rr", Col: "t.brake_temp_rr", Type: TypeF32},
	{Name: "brake_temp_fl", Col: "t.brake_temp_fl", Type: TypeF32},
	{Name: "brake_temp_fr", Col: "t.brake_temp_fr", Type: TypeF32},
	{Name: "brake_damage_rl", Col: "t.brake_damage_rl", Type: TypeI16},
	{Name: "brake_damage_rr", Col: "t.brake_damage_rr", Type: TypeI16},
	{Name: "brake_damage_fl", Col: "t.brake_damage_fl", Type: TypeI16},
	{Name: "brake_damage_fr", Col: "t.brake_damage_fr", Type: TypeI16},

	{Name: "lap_time_ms", Col: "t.lap_time_ms", Type: TypeI32},
	{Name: "car_position", Col: "t.car_position", Type: TypeI16},
	{Name: "sector", Col: "t.sector", Type: TypeI16},
	{Name: "pit_status", Col: "t.pit_status", Type: TypeI16},
	{Name: "driver_status", Col: "t.driver_status", Type: TypeI16},
	{Name: "lap_invalid", Col: "t.lap_invalid", Type: TypeI16},
	{Name: "penalties", Col: "t.penalties", Type: TypeI16},

	// MotionEx — player car only; NULL for other cars. Scanned generically
	// (see scanTelemetryRow) so a NULL here just encodes as zero, same as
	// the old fixed-format encoder did.
	{Name: "susp_pos_rl", Col: "t.susp_pos_rl", Type: TypeF32},
	{Name: "susp_pos_rr", Col: "t.susp_pos_rr", Type: TypeF32},
	{Name: "susp_pos_fl", Col: "t.susp_pos_fl", Type: TypeF32},
	{Name: "susp_pos_fr", Col: "t.susp_pos_fr", Type: TypeF32},
	{Name: "susp_vel_rl", Col: "t.susp_vel_rl", Type: TypeF32},
	{Name: "susp_vel_rr", Col: "t.susp_vel_rr", Type: TypeF32},
	{Name: "susp_vel_fl", Col: "t.susp_vel_fl", Type: TypeF32},
	{Name: "susp_vel_fr", Col: "t.susp_vel_fr", Type: TypeF32},
	{Name: "susp_accel_rl", Col: "t.susp_accel_rl", Type: TypeF32},
	{Name: "susp_accel_rr", Col: "t.susp_accel_rr", Type: TypeF32},
	{Name: "susp_accel_fl", Col: "t.susp_accel_fl", Type: TypeF32},
	{Name: "susp_accel_fr", Col: "t.susp_accel_fr", Type: TypeF32},
	{Name: "wheel_speed_rl", Col: "t.wheel_speed_rl", Type: TypeF32},
	{Name: "wheel_speed_rr", Col: "t.wheel_speed_rr", Type: TypeF32},
	{Name: "wheel_speed_fl", Col: "t.wheel_speed_fl", Type: TypeF32},
	{Name: "wheel_speed_fr", Col: "t.wheel_speed_fr", Type: TypeF32},
	{Name: "wheel_slip_ratio_rl", Col: "t.wheel_slip_ratio_rl", Type: TypeF32},
	{Name: "wheel_slip_ratio_rr", Col: "t.wheel_slip_ratio_rr", Type: TypeF32},
	{Name: "wheel_slip_ratio_fl", Col: "t.wheel_slip_ratio_fl", Type: TypeF32},
	{Name: "wheel_slip_ratio_fr", Col: "t.wheel_slip_ratio_fr", Type: TypeF32},
	{Name: "wheel_slip_angle_rl", Col: "t.wheel_slip_angle_rl", Type: TypeF32},
	{Name: "wheel_slip_angle_rr", Col: "t.wheel_slip_angle_rr", Type: TypeF32},
	{Name: "wheel_slip_angle_fl", Col: "t.wheel_slip_angle_fl", Type: TypeF32},
	{Name: "wheel_slip_angle_fr", Col: "t.wheel_slip_angle_fr", Type: TypeF32},
	{Name: "wheel_lat_force_rl", Col: "t.wheel_lat_force_rl", Type: TypeF32},
	{Name: "wheel_lat_force_rr", Col: "t.wheel_lat_force_rr", Type: TypeF32},
	{Name: "wheel_lat_force_fl", Col: "t.wheel_lat_force_fl", Type: TypeF32},
	{Name: "wheel_lat_force_fr", Col: "t.wheel_lat_force_fr", Type: TypeF32},
	{Name: "wheel_long_force_rl", Col: "t.wheel_long_force_rl", Type: TypeF32},
	{Name: "wheel_long_force_rr", Col: "t.wheel_long_force_rr", Type: TypeF32},
	{Name: "wheel_long_force_fl", Col: "t.wheel_long_force_fl", Type: TypeF32},
	{Name: "wheel_long_force_fr", Col: "t.wheel_long_force_fr", Type: TypeF32},
	{Name: "wheel_vert_force_rl", Col: "t.wheel_vert_force_rl", Type: TypeF32},
	{Name: "wheel_vert_force_rr", Col: "t.wheel_vert_force_rr", Type: TypeF32},
	{Name: "wheel_vert_force_fl", Col: "t.wheel_vert_force_fl", Type: TypeF32},
	{Name: "wheel_vert_force_fr", Col: "t.wheel_vert_force_fr", Type: TypeF32},
	{Name: "local_vel_x", Col: "t.local_vel_x", Type: TypeF32},
	{Name: "local_vel_y", Col: "t.local_vel_y", Type: TypeF32},
	{Name: "local_vel_z", Col: "t.local_vel_z", Type: TypeF32},
	{Name: "ang_vel_x", Col: "t.ang_vel_x", Type: TypeF32},
	{Name: "ang_vel_y", Col: "t.ang_vel_y", Type: TypeF32},
	{Name: "ang_vel_z", Col: "t.ang_vel_z", Type: TypeF32},
	{Name: "front_wheels_angle", Col: "t.front_wheels_angle", Type: TypeF32},
	{Name: "front_aero_height", Col: "t.front_aero_height", Type: TypeF32},
	{Name: "rear_aero_height", Col: "t.rear_aero_height", Type: TypeF32},
	{Name: "wheel_camber_rl", Col: "t.wheel_camber_rl", Type: TypeF32},
	{Name: "wheel_camber_rr", Col: "t.wheel_camber_rr", Type: TypeF32},
	{Name: "wheel_camber_fl", Col: "t.wheel_camber_fl", Type: TypeF32},
	{Name: "wheel_camber_fr", Col: "t.wheel_camber_fr", Type: TypeF32},
}

var fieldByName map[string]FieldDef

func init() {
	fieldByName = make(map[string]FieldDef, len(fieldRegistry))
	for i := range fieldRegistry {
		fieldRegistry[i].ID = uint8(i + 1) // 0 is reserved/invalid
		fieldByName[fieldRegistry[i].Name] = fieldRegistry[i]
	}
}

// ResolveFields looks up each name in the registry, preserving order and
// dropping duplicates. Returns an error naming the first unknown field —
// the registry is the whitelist that keeps caller-supplied names from ever
// reaching SQL directly.
func ResolveFields(names []string) ([]FieldDef, error) {
	seen := make(map[string]bool, len(names))
	out := make([]FieldDef, 0, len(names))
	for _, n := range names {
		if seen[n] {
			continue
		}
		seen[n] = true
		f, ok := fieldByName[n]
		if !ok {
			return nil, fmt.Errorf("unknown field: %q", n)
		}
		out = append(out, f)
	}
	return out, nil
}

// AllFields returns every selectable field, in registry order — the default
// when a caller doesn't pass a `fields` list, preserving old full-row
// behavior for anyone not opting into a subset.
func AllFields() []FieldDef {
	out := make([]FieldDef, len(fieldRegistry))
	copy(out, fieldRegistry)
	return out
}

// FieldNames returns the Name of each field, in the given order.
func FieldNames(fields []FieldDef) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = f.Name
	}
	return out
}
