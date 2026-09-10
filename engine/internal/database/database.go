package database

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"NewTelemetryEngine/internal/config"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/001_init.sql
var initSQL string

//go:embed migrations/001_init_local.sql
var initLocalSQL string

//go:embed migrations/002_expand_telemetry.sql
var expandSQL string

//go:embed migrations/003_participants.sql
var participantsSQL string

//go:embed migrations/004_indexes.sql
var indexesSQL string

//go:embed migrations/005_participants_updated_at.sql
var participantsUpdatedAtSQL string

//go:embed migrations/006_lap_times.sql
var lapTimesSQL string

//go:embed migrations/007_backfill_invalid_laps.sql
var backfillInvalidLapsSQL string

type DB struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func New(ctx context.Context, cfg *config.Config) (*DB, error) {
	pool, err := pgxpool.New(ctx, cfg.DSN())
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &DB{pool: pool, cfg: cfg}, nil
}

// Migrate runs all embedded SQL migrations in order. Safe to call on every
// startup — all statements are idempotent (IF NOT EXISTS / ADD COLUMN IF NOT EXISTS).
func (db *DB) Migrate(ctx context.Context) error {
	if _, err := db.pool.Exec(ctx, initSQL); err != nil {
		return err
	}
	if _, err := db.pool.Exec(ctx, expandSQL); err != nil {
		return err
	}
	if _, err := db.pool.Exec(ctx, participantsSQL); err != nil {
		return err
	}
	if _, err := db.pool.Exec(ctx, indexesSQL); err != nil {
		return err
	}
	if _, err := db.pool.Exec(ctx, participantsUpdatedAtSQL); err != nil {
		return err
	}
	if _, err := db.pool.Exec(ctx, lapTimesSQL); err != nil {
		return err
	}
	_, err := db.pool.Exec(ctx, backfillInvalidLapsSQL)
	return err
}

// MigrateLocal is the cmd/engine equivalent of Migrate for the embedded,
// plain-PostgreSQL database (no TimescaleDB extension available) — same
// migrations 002-007, but 001_init_local.sql in place of 001_init.sql (see
// that file for why the telemetry table can't be shared verbatim between
// the two paths).
func (db *DB) MigrateLocal(ctx context.Context) error {
	if _, err := db.pool.Exec(ctx, initLocalSQL); err != nil {
		return err
	}
	if _, err := db.pool.Exec(ctx, expandSQL); err != nil {
		return err
	}
	if _, err := db.pool.Exec(ctx, participantsSQL); err != nil {
		return err
	}
	if _, err := db.pool.Exec(ctx, indexesSQL); err != nil {
		return err
	}
	if _, err := db.pool.Exec(ctx, participantsUpdatedAtSQL); err != nil {
		return err
	}
	if _, err := db.pool.Exec(ctx, lapTimesSQL); err != nil {
		return err
	}
	_, err := db.pool.Exec(ctx, backfillInvalidLapsSQL)
	return err
}

// EnsureCurrentPartitions creates the telemetry partitions (see
// 001_init_local.sql) covering this month and next, if they don't already
// exist — only meaningful against the MigrateLocal schema; calling it
// against a hypertable (Migrate) would fail, since ATTACH/PARTITION OF
// doesn't apply there. Next month is included so a partition always exists
// ahead of the boundary regardless of when the app is next started.
func (db *DB) EnsureCurrentPartitions(ctx context.Context) error {
	now := time.Now().UTC()
	for _, monthStart := range []time.Time{
		time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC),
		time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC),
	} {
		monthEnd := monthStart.AddDate(0, 1, 0)
		name := fmt.Sprintf("telemetry_y%04dm%02d", monthStart.Year(), monthStart.Month())
		sql := fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s PARTITION OF telemetry FOR VALUES FROM ('%s') TO ('%s')`,
			pgx.Identifier{name}.Sanitize(),
			monthStart.Format(time.RFC3339),
			monthEnd.Format(time.RFC3339),
		)
		if _, err := db.pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("ensure partition %s: %w", name, err)
		}
	}
	return nil
}

func (db *DB) Close() {
	db.pool.Close()
}

func (db *DB) NewTelemetryWriter() *TelemetryWriter {
	return newTelemetryWriter(
		db.pool,
		time.Duration(db.cfg.DBWriteFlushIntervalMS)*time.Millisecond,
		db.cfg.DBWriteBatchSize,
	)
}

func (db *DB) NewSessionWriter() *SessionWriter {
	return &SessionWriter{pool: db.pool}
}

func (db *DB) NewReader() *Reader {
	return &Reader{
		pool:       db.pool,
		liveWindow: time.Duration(db.cfg.LiveWindowS) * time.Second,
	}
}
