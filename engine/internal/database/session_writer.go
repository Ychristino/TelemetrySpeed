package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ParticipantEntry carries per-car participant data for one session.
type ParticipantEntry struct {
	CarIndex   int16
	Name       string
	Team       string
	RaceNumber int16
}

type SessionWriter struct {
	pool *pgxpool.Pool
}

func (sw *SessionWriter) CreateSession(ctx context.Context, game string) (uuid.UUID, error) {
	var id uuid.UUID
	err := sw.pool.QueryRow(ctx,
		`INSERT INTO sessions (game) VALUES ($1) RETURNING session_id`,
		game,
	).Scan(&id)
	return id, err
}

func (sw *SessionWriter) UpdateSessionTrack(ctx context.Context, sessionID uuid.UUID, track string) error {
	_, err := sw.pool.Exec(ctx,
		`UPDATE sessions SET track = $1 WHERE session_id = $2`,
		track, sessionID,
	)
	return err
}

func (sw *SessionWriter) UpdateSessionParticipant(ctx context.Context, sessionID uuid.UUID, userID, car string) error {
	_, err := sw.pool.Exec(ctx,
		`UPDATE sessions SET user_id = $1, car = $2 WHERE session_id = $3`,
		userID, car, sessionID,
	)
	return err
}

func (sw *SessionWriter) CloseSession(ctx context.Context, sessionID uuid.UUID) error {
	_, err := sw.pool.Exec(ctx,
		`UPDATE sessions SET ended_at = NOW() WHERE session_id = $1`,
		sessionID,
	)
	return err
}

func (sw *SessionWriter) OpenLap(ctx context.Context, sessionID uuid.UUID, lapNumber uint8, startedAt time.Time) (uuid.UUID, error) {
	var id uuid.UUID
	err := sw.pool.QueryRow(ctx,
		`INSERT INTO laps (session_id, lap_number, started_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (session_id, lap_number) DO NOTHING
		 RETURNING lap_id`,
		sessionID, int(lapNumber), startedAt,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("open lap %d: %w", lapNumber, err)
	}
	return id, nil
}

func (sw *SessionWriter) CloseLap(ctx context.Context, lapID uuid.UUID, endedAt time.Time) error {
	_, err := sw.pool.Exec(ctx,
		`UPDATE laps SET ended_at = $1 WHERE lap_id = $2`,
		endedAt, lapID,
	)
	return err
}

func (sw *SessionWriter) OpenSector(ctx context.Context, lapID uuid.UUID, sector uint8, startedAt time.Time) error {
	_, err := sw.pool.Exec(ctx,
		`INSERT INTO sectors (lap_id, sector, started_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (lap_id, sector) DO NOTHING`,
		lapID, int(sector), startedAt,
	)
	return err
}

func (sw *SessionWriter) CloseSector(ctx context.Context, lapID uuid.UUID, sector uint8, endedAt time.Time) error {
	_, err := sw.pool.Exec(ctx,
		`UPDATE sectors SET ended_at = $1 WHERE lap_id = $2 AND sector = $3`,
		endedAt, lapID, int(sector),
	)
	return err
}

// RecordLapTime upserts one driver's finishing time for a completed lap into
// the lap_times summary table — see migrations/006_lap_times.sql for why this
// exists separately from the laps table (which only tracks the local
// player's lap/sector boundaries, not every car's finishing time). Called
// once per car per lap rollover from SourceHandler.handleLapData.
func (sw *SessionWriter) RecordLapTime(ctx context.Context, sessionID uuid.UUID, carIndex int16, lapNumber int, lapTimeMS int32, lapInvalid uint8) error {
	_, err := sw.pool.Exec(ctx,
		`INSERT INTO lap_times (session_id, car_index, lap_number, lap_time_ms, lap_invalid)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (session_id, car_index, lap_number) DO UPDATE
		 SET lap_time_ms = EXCLUDED.lap_time_ms, lap_invalid = EXCLUDED.lap_invalid`,
		sessionID, carIndex, lapNumber, lapTimeMS, int16(lapInvalid),
	)
	return err
}

func (sw *SessionWriter) UpsertParticipants(ctx context.Context, sessionID uuid.UUID, entries []ParticipantEntry) error {
	batch := &pgx.Batch{}
	for _, e := range entries {
		batch.Queue(
			`INSERT INTO participants (session_id, car_index, name, team, race_number, updated_at)
			 VALUES ($1, $2, $3, $4, $5, NOW())
			 ON CONFLICT (session_id, car_index) DO UPDATE
			 SET name = EXCLUDED.name, team = EXCLUDED.team, race_number = EXCLUDED.race_number, updated_at = NOW()`,
			sessionID, e.CarIndex, e.Name, e.Team, e.RaceNumber,
		)
	}
	br := sw.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range entries {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}
