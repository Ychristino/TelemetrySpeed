-- Variant of 001_init.sql for the embedded (bundled, single-exe) engine,
-- which runs against a plain PostgreSQL install with no TimescaleDB
-- extension available (see cmd/engine). Schema is identical to 001_init.sql
-- except telemetry uses native PostgreSQL declarative partitioning instead
-- of a TimescaleDB hypertable — every query already filters by session_id
-- first (see idx_telemetry_session_car_ts/idx_telemetry_session_ts below),
-- so partition pruning on ts isn't load-bearing for reads; it mainly buys
-- cheap bulk eviction of old partitions later if this ever needs retention.
-- Keep this file's schema in sync with 001_init.sql — deliberately
-- duplicated rather than shared, since a partitioned CREATE TABLE can't be
-- expressed with the same statement as the hypertable path.

CREATE TABLE IF NOT EXISTS sessions (
    session_id   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    started_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at     TIMESTAMPTZ,
    game         TEXT        NOT NULL DEFAULT '',
    user_id      TEXT        NOT NULL DEFAULT '',
    track        TEXT        NOT NULL DEFAULT '',
    car          TEXT        NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS laps (
    lap_id       UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id   UUID        NOT NULL REFERENCES sessions(session_id) ON DELETE CASCADE,
    lap_number   INT         NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL,
    ended_at     TIMESTAMPTZ,
    UNIQUE (session_id, lap_number)
);

CREATE TABLE IF NOT EXISTS sectors (
    lap_id       UUID        NOT NULL REFERENCES laps(lap_id) ON DELETE CASCADE,
    sector       INT         NOT NULL CHECK (sector BETWEEN 1 AND 3),
    started_at   TIMESTAMPTZ NOT NULL,
    ended_at     TIMESTAMPTZ,
    PRIMARY KEY (lap_id, sector)
);

CREATE TABLE IF NOT EXISTS telemetry (
    ts           TIMESTAMPTZ NOT NULL,
    session_id   UUID        NOT NULL REFERENCES sessions(session_id) ON DELETE CASCADE,
    car_index    SMALLINT    NOT NULL,

    -- Driver inputs
    speed        REAL,
    throttle     REAL,
    brake        REAL,
    steering     REAL,
    gear         SMALLINT,
    clutch       REAL,
    drs          SMALLINT,

    -- Position / orientation
    pos_x        REAL,
    pos_y        REAL,
    pos_z        REAL,
    vel_x        REAL,
    vel_y        REAL,
    vel_z        REAL,
    yaw          REAL,
    pitch        REAL,
    roll         REAL,

    -- G-forces
    g_lat        REAL,
    g_long       REAL,
    g_vert       REAL,

    -- Engine / fuel
    rpm          INT,
    eng_temp     REAL,
    oil_pressure REAL,
    fuel         REAL,
    fuel_rem_laps    REAL,
    fuel_mix         SMALLINT,
    pit_limiter      SMALLINT,
    engine_power_ice  REAL,
    engine_power_mguk REAL,

    -- ERS
    ers_store        REAL,
    ers_deploy_mode  SMALLINT,

    -- Driver aids
    traction_control SMALLINT,
    abs              SMALLINT,

    -- Tyres (scalars)
    tyre_compound    SMALLINT,
    tyre_age_laps    SMALLINT,

    -- Tyre surface temperature (RL, RR, FL, FR)
    tyre_surf_temp_rl  REAL,
    tyre_surf_temp_rr  REAL,
    tyre_surf_temp_fl  REAL,
    tyre_surf_temp_fr  REAL,

    -- Tyre inner temperature
    tyre_inner_temp_rl REAL,
    tyre_inner_temp_rr REAL,
    tyre_inner_temp_fl REAL,
    tyre_inner_temp_fr REAL,

    -- Tyre pressure (PSI)
    tyre_pressure_rl   REAL,
    tyre_pressure_rr   REAL,
    tyre_pressure_fl   REAL,
    tyre_pressure_fr   REAL,

    -- Tyre wear
    tyre_wear_rl    REAL,
    tyre_wear_rr    REAL,
    tyre_wear_fl    REAL,
    tyre_wear_fr    REAL,

    -- Tyre blisters (F1 26 only)
    tyre_blister_rl SMALLINT,
    tyre_blister_rr SMALLINT,
    tyre_blister_fl SMALLINT,
    tyre_blister_fr SMALLINT,

    -- Brake temperature
    brake_temp_rl   REAL,
    brake_temp_rr   REAL,
    brake_temp_fl   REAL,
    brake_temp_fr   REAL,

    -- Brake damage
    brake_damage_rl SMALLINT,
    brake_damage_rr SMALLINT,
    brake_damage_fl SMALLINT,
    brake_damage_fr SMALLINT,

    -- Lap state
    lap_time_ms     INTEGER,
    lap_distance     REAL,
    car_position    SMALLINT,
    lap_num         SMALLINT,
    sector          SMALLINT,
    pit_status      SMALLINT,
    driver_status   SMALLINT,
    lap_invalid     SMALLINT,
    penalties       SMALLINT,

    -- MotionEx: suspension (player car only — NULL for other cars)
    susp_pos_rl     REAL,
    susp_pos_rr     REAL,
    susp_pos_fl     REAL,
    susp_pos_fr     REAL,

    susp_vel_rl     REAL,
    susp_vel_rr     REAL,
    susp_vel_fl     REAL,
    susp_vel_fr     REAL,

    susp_accel_rl   REAL,
    susp_accel_rr   REAL,
    susp_accel_fl   REAL,
    susp_accel_fr   REAL,

    -- MotionEx: wheel dynamics
    wheel_speed_rl  REAL,
    wheel_speed_rr  REAL,
    wheel_speed_fl  REAL,
    wheel_speed_fr  REAL,

    wheel_slip_ratio_rl REAL,
    wheel_slip_ratio_rr REAL,
    wheel_slip_ratio_fl REAL,
    wheel_slip_ratio_fr REAL,

    wheel_slip_angle_rl REAL,
    wheel_slip_angle_rr REAL,
    wheel_slip_angle_fl REAL,
    wheel_slip_angle_fr REAL,

    wheel_lat_force_rl  REAL,
    wheel_lat_force_rr  REAL,
    wheel_lat_force_fl  REAL,
    wheel_lat_force_fr  REAL,

    wheel_long_force_rl REAL,
    wheel_long_force_rr REAL,
    wheel_long_force_fl REAL,
    wheel_long_force_fr REAL,

    wheel_vert_force_rl REAL,
    wheel_vert_force_rr REAL,
    wheel_vert_force_fl REAL,
    wheel_vert_force_fr REAL,

    -- MotionEx: body dynamics
    local_vel_x     REAL,
    local_vel_y     REAL,
    local_vel_z     REAL,

    ang_vel_x       REAL,
    ang_vel_y       REAL,
    ang_vel_z       REAL,

    front_wheels_angle REAL,
    front_aero_height  REAL,
    rear_aero_height   REAL,

    -- MotionEx: camber (F1 26 active camber; 0 for F1 25)
    wheel_camber_rl REAL,
    wheel_camber_rr REAL,
    wheel_camber_fl REAL,
    wheel_camber_fr REAL
) PARTITION BY RANGE (ts);

-- Catch-all so inserts never fail for a ts outside the partitions
-- EnsureCurrentPartitions (database.go) maintains — mainly historical rows
-- from before that maintenance ran, or a gap if the app wasn't started for
-- a full month.
CREATE TABLE IF NOT EXISTS telemetry_default PARTITION OF telemetry DEFAULT;

CREATE INDEX IF NOT EXISTS idx_telemetry_session_car_ts ON telemetry (session_id, car_index, ts DESC);
CREATE INDEX IF NOT EXISTS idx_telemetry_session_ts     ON telemetry (session_id, ts DESC);
CREATE INDEX IF NOT EXISTS idx_laps_session_num         ON laps (session_id, lap_number);
