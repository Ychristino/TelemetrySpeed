-- Backfills lap_invalid for laps recorded before SourceHandler.recordLapTimes
-- started forcing invalid on any lap whose counter advanced without the car
-- ever reaching the final sector (a restarted attempt, or a garage/pit
-- return mid-lap). The game's own m_currentLapInvalid flag never caught
-- this — it's for rule violations, not incomplete laps — so rows written
-- before that fix can still say lap_invalid = 0 despite never having
-- covered all three sectors.
--
-- Idempotent: only touches rows still marked lap_invalid = 0, so re-running
-- this on every startup (see database.go's Migrate) converges to a no-op.
UPDATE lap_times lt
SET lap_invalid = 1
WHERE lt.lap_invalid = 0
  AND (
    NOT EXISTS (
      SELECT 1 FROM telemetry t
      WHERE t.session_id = lt.session_id AND t.car_index = lt.car_index
        AND t.lap_num = lt.lap_number AND t.sector = 0
    )
    OR NOT EXISTS (
      SELECT 1 FROM telemetry t
      WHERE t.session_id = lt.session_id AND t.car_index = lt.car_index
        AND t.lap_num = lt.lap_number AND t.sector = 1
    )
    OR NOT EXISTS (
      SELECT 1 FROM telemetry t
      WHERE t.session_id = lt.session_id AND t.car_index = lt.car_index
        AND t.lap_num = lt.lap_number AND t.sector = 2
    )
  );
