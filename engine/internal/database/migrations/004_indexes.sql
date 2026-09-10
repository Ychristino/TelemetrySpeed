-- sessions has no support for the game+track lookups every read query does
-- (ListSessions, ListLapTimes, ListLiveDrivers, and the telemetry
-- game+track CTEs) — only session_id (PK) was indexed, forcing a seq scan
-- on sessions ahead of every one of those joins.
CREATE INDEX IF NOT EXISTS idx_sessions_game_track_started
    ON sessions (game, track, started_at DESC);

-- Redundant with the UNIQUE (session_id, lap_number) constraint on laps,
-- which already creates an equivalent index.
DROP INDEX IF EXISTS idx_laps_session_num;
