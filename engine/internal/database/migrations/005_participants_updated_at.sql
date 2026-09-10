-- A driver can occupy different car_index slots within the same session_id
-- (reconnect, grid reassignment) — the old slot's row is never deleted, just
-- left stale, so a lookup by name alone can match more than one row.
-- updated_at lets ResolveCarIndex (reader.go) pick the most recently
-- confirmed car_index for that name instead of an arbitrary match.
ALTER TABLE participants ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
