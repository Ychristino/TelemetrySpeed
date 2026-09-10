-- Precomputed per-driver lap times, written once per lap at the moment a car
-- crosses the line (see SourceHandler.handleLapData). ListLapTimes previously
-- derived this with MAX(lap_time_ms) grouped over raw telemetry, which forces
-- a scan of the entire hypertable for a game+track — across every session
-- ever recorded — on every leaderboard request. This table holds one row per
-- (session, car, lap) instead, so that query becomes a cheap read over a
-- tiny table rather than an aggregate over millions of telemetry rows.
CREATE TABLE IF NOT EXISTS lap_times (
    session_id  UUID     NOT NULL REFERENCES sessions(session_id) ON DELETE CASCADE,
    car_index   SMALLINT NOT NULL,
    lap_number  INT      NOT NULL,
    lap_time_ms INT      NOT NULL,
    lap_invalid SMALLINT NOT NULL DEFAULT 0,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (session_id, car_index, lap_number)
);
