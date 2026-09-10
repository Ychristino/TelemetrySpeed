CREATE TABLE IF NOT EXISTS participants (
    session_id  UUID     NOT NULL REFERENCES sessions(session_id) ON DELETE CASCADE,
    car_index   SMALLINT NOT NULL,
    name        TEXT     NOT NULL DEFAULT '',
    team        TEXT     NOT NULL DEFAULT '',
    race_number SMALLINT NOT NULL DEFAULT 0,
    PRIMARY KEY (session_id, car_index)
);

CREATE INDEX IF NOT EXISTS idx_participants_name ON participants(session_id, lower(name));
