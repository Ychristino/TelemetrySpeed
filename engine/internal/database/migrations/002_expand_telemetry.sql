-- Expands the telemetry table with all channels available from F1 UDP packets.
-- Each statement is idempotent (ADD COLUMN IF NOT EXISTS).

-- Driver inputs
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS clutch          REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS drs             SMALLINT;

-- Velocity / orientation (from Motion packet)
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS vel_x           REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS vel_y           REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS vel_z           REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS yaw             REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS pitch           REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS roll            REAL;

-- G-forces
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS g_lat           REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS g_long          REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS g_vert          REAL;

-- Engine / fuel
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS fuel_rem_laps   REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS fuel_mix        SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS pit_limiter     SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS engine_power_ice  REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS engine_power_mguk REAL;

-- ERS
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS ers_store       REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS ers_deploy_mode SMALLINT;

-- Driver aids
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS traction_control SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS abs             SMALLINT;

-- Tyre (scalars)
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_compound   SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_age_laps   SMALLINT;

-- Tyre surface temperature (RL, RR, FL, FR)
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_surf_temp_rl  REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_surf_temp_rr  REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_surf_temp_fl  REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_surf_temp_fr  REAL;

-- Tyre inner temperature
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_inner_temp_rl REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_inner_temp_rr REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_inner_temp_fl REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_inner_temp_fr REAL;

-- Tyre pressure (PSI)
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_pressure_rl   REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_pressure_rr   REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_pressure_fl   REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_pressure_fr   REAL;

-- Tyre wear (0–100%)
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_wear_rl    REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_wear_rr    REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_wear_fl    REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_wear_fr    REAL;

-- Tyre blisters (F1 26 only; 0 for F1 25)
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_blister_rl SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_blister_rr SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_blister_fl SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS tyre_blister_fr SMALLINT;

-- Brake temperature (celsius)
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS brake_temp_rl   REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS brake_temp_rr   REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS brake_temp_fl   REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS brake_temp_fr   REAL;

-- Brake damage (0–100%)
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS brake_damage_rl SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS brake_damage_rr SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS brake_damage_fl SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS brake_damage_fr SMALLINT;

-- Lap state
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS lap_time_ms     INTEGER;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS lap_distance     REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS car_position    SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS lap_num         SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS sector          SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS pit_status      SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS driver_status   SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS lap_invalid     SMALLINT;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS penalties       SMALLINT;

-- MotionEx: suspension (player car only — NULL for other cars)
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS susp_pos_rl     REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS susp_pos_rr     REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS susp_pos_fl     REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS susp_pos_fr     REAL;

ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS susp_vel_rl     REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS susp_vel_rr     REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS susp_vel_fl     REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS susp_vel_fr     REAL;

ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS susp_accel_rl   REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS susp_accel_rr   REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS susp_accel_fl   REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS susp_accel_fr   REAL;

-- MotionEx: wheel dynamics
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_speed_rl  REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_speed_rr  REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_speed_fl  REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_speed_fr  REAL;

ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_slip_ratio_rl REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_slip_ratio_rr REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_slip_ratio_fl REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_slip_ratio_fr REAL;

ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_slip_angle_rl REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_slip_angle_rr REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_slip_angle_fl REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_slip_angle_fr REAL;

ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_lat_force_rl  REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_lat_force_rr  REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_lat_force_fl  REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_lat_force_fr  REAL;

ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_long_force_rl REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_long_force_rr REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_long_force_fl REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_long_force_fr REAL;

ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_vert_force_rl REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_vert_force_rr REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_vert_force_fl REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_vert_force_fr REAL;

-- MotionEx: body dynamics
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS local_vel_x     REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS local_vel_y     REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS local_vel_z     REAL;

ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS ang_vel_x       REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS ang_vel_y       REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS ang_vel_z       REAL;

ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS front_wheels_angle REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS front_aero_height  REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS rear_aero_height   REAL;

-- MotionEx: camber (F1 26 active camber; 0 for F1 25)
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_camber_rl REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_camber_rr REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_camber_fl REAL;
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wheel_camber_fr REAL;
