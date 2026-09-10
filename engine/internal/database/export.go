package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ExportFormatVersion guards against loading a bundle written by an
// incompatible future export format. Bump whenever ExportBundle's shape
// changes in a way older Import code can't handle.
const ExportFormatVersion = 1

// ExportScope selects how much of the database Export pulls out. Kind is
// one of "all", "track", or "lap" — the other fields are only read for the
// kind that needs them.
type ExportScope struct {
	Kind      string // "all" | "track" | "lap"
	Game      string
	Track     string
	SessionID uuid.UUID
	CarIndex  int16
	LapNumber int
}

// The Export* types mirror their table's columns exactly (see
// migrations/001_init.sql, 003_participants.sql, 006_lap_times.sql) so a
// round trip through Export → JSON → Import reconstructs every row
// faithfully. TelemetryRow (writer.go) already covers every telemetry
// column, so it's reused as-is rather than duplicated here.

type ExportSession struct {
	SessionID uuid.UUID  `json:"session_id"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Game      string     `json:"game"`
	UserID    string     `json:"user_id"`
	Track     string     `json:"track"`
	Car       string     `json:"car"`
}

type ExportParticipant struct {
	SessionID  uuid.UUID `json:"session_id"`
	CarIndex   int16     `json:"car_index"`
	Name       string    `json:"name"`
	Team       string    `json:"team"`
	RaceNumber int16     `json:"race_number"`
}

type ExportLap struct {
	LapID     uuid.UUID  `json:"lap_id"`
	SessionID uuid.UUID  `json:"session_id"`
	LapNumber int        `json:"lap_number"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

type ExportSector struct {
	LapID     uuid.UUID  `json:"lap_id"`
	Sector    int        `json:"sector"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

type ExportLapTime struct {
	SessionID  uuid.UUID `json:"session_id"`
	CarIndex   int16     `json:"car_index"`
	LapNumber  int       `json:"lap_number"`
	LapTimeMS  int32     `json:"lap_time_ms"`
	LapInvalid int16     `json:"lap_invalid"`
	RecordedAt time.Time `json:"recorded_at"`
}

// ExportTelemetryRow is one telemetry row scanned generically via the same
// FieldDef registry (fields.go) the WebSocket stream uses — Values holds
// one entry per name in ExportBundle.TelemetryFields, same order, nil for
// SQL NULL (MotionEx columns on a non-player car). Kept field-list-relative
// rather than a fixed struct so export/import automatically covers every
// registered field without duplicating the ~100-column list a third time.
type ExportTelemetryRow struct {
	SessionID   uuid.UUID `json:"session_id"`
	Ts          time.Time `json:"ts"`
	CarIndex    int16     `json:"car_index"`
	LapNum      int16     `json:"lap_num"`
	LapDistance float32   `json:"lap_distance"`
	Values      []any     `json:"values"`
}

// ExportBundle is the full, self-contained payload written to (and read
// from) an export file — see cmd/engine or consumer_api/export.go for how
// it's serialized (gzip-compressed JSON) and transferred over HTTP.
type ExportBundle struct {
	FormatVersion int    `json:"format_version"`
	Scope         string `json:"scope"`
	Game          string `json:"game,omitempty"`
	Track         string `json:"track,omitempty"`

	Sessions     []ExportSession     `json:"sessions"`
	Participants []ExportParticipant `json:"participants"`
	Laps         []ExportLap         `json:"laps"`
	Sectors      []ExportSector      `json:"sectors"`
	LapTimes     []ExportLapTime     `json:"lap_times"`

	TelemetryFields []string             `json:"telemetry_fields"`
	Telemetry       []ExportTelemetryRow `json:"telemetry"`
}

// Export pulls every row needed to faithfully reconstruct the requested
// scope elsewhere via Import. "lap" scope still includes the laps/sectors
// rows for the session's local player if any exist and the lap number
// matches — laps/sectors are recorded only for whichever car was the
// recording player (see SourceHandler), so they carry no car_index of
// their own to filter by; a lap export for a non-player car simply won't
// have any.
func (r *Reader) Export(ctx context.Context, scope ExportScope) (*ExportBundle, error) {
	var sessionIDs []uuid.UUID
	var err error

	switch scope.Kind {
	case "all":
		sessionIDs, err = r.exportSessionIDs(ctx, "SELECT session_id FROM sessions")
	case "track":
		sessionIDs, err = r.exportSessionIDs(ctx,
			"SELECT session_id FROM sessions WHERE game = $1 AND track = $2", scope.Game, scope.Track)
	case "lap":
		sessionIDs = []uuid.UUID{scope.SessionID}
	default:
		return nil, fmt.Errorf("unknown export scope %q", scope.Kind)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve sessions: %w", err)
	}
	if len(sessionIDs) == 0 {
		return &ExportBundle{FormatVersion: ExportFormatVersion, Scope: scope.Kind, Game: scope.Game, Track: scope.Track}, nil
	}

	bundle := &ExportBundle{FormatVersion: ExportFormatVersion, Scope: scope.Kind, Game: scope.Game, Track: scope.Track}

	bundle.Sessions, err = r.exportSessions(ctx, sessionIDs)
	if err != nil {
		return nil, fmt.Errorf("export sessions: %w", err)
	}

	if scope.Kind == "lap" {
		bundle.Participants, err = r.exportParticipants(ctx, sessionIDs, &scope.CarIndex)
	} else {
		bundle.Participants, err = r.exportParticipants(ctx, sessionIDs, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("export participants: %w", err)
	}

	var lapNumberFilter *int
	if scope.Kind == "lap" {
		lapNumberFilter = &scope.LapNumber
	}
	bundle.Laps, err = r.exportLaps(ctx, sessionIDs, lapNumberFilter)
	if err != nil {
		return nil, fmt.Errorf("export laps: %w", err)
	}

	lapIDs := make([]uuid.UUID, len(bundle.Laps))
	for i, l := range bundle.Laps {
		lapIDs[i] = l.LapID
	}
	bundle.Sectors, err = r.exportSectors(ctx, lapIDs)
	if err != nil {
		return nil, fmt.Errorf("export sectors: %w", err)
	}

	if scope.Kind == "lap" {
		bundle.LapTimes, err = r.exportLapTimes(ctx, sessionIDs, &scope.CarIndex, &scope.LapNumber)
	} else {
		bundle.LapTimes, err = r.exportLapTimes(ctx, sessionIDs, nil, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("export lap_times: %w", err)
	}

	fields := AllFields()
	bundle.TelemetryFields = FieldNames(fields)
	if scope.Kind == "lap" {
		bundle.Telemetry, err = r.exportTelemetry(ctx, sessionIDs, fields, &scope.CarIndex, &scope.LapNumber)
	} else {
		bundle.Telemetry, err = r.exportTelemetry(ctx, sessionIDs, fields, nil, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("export telemetry: %w", err)
	}

	return bundle, nil
}

func (r *Reader) exportSessionIDs(ctx context.Context, query string, args ...any) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *Reader) exportSessions(ctx context.Context, sessionIDs []uuid.UUID) ([]ExportSession, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT session_id, started_at, ended_at, game, user_id, track, car
		FROM sessions WHERE session_id = ANY($1)
		ORDER BY started_at`, sessionIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExportSession
	for rows.Next() {
		var s ExportSession
		if err := rows.Scan(&s.SessionID, &s.StartedAt, &s.EndedAt, &s.Game, &s.UserID, &s.Track, &s.Car); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Reader) exportParticipants(ctx context.Context, sessionIDs []uuid.UUID, carIndex *int16) ([]ExportParticipant, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT session_id, car_index, name, team, race_number
		FROM participants
		WHERE session_id = ANY($1) AND ($2::SMALLINT IS NULL OR car_index = $2)
		ORDER BY session_id, car_index`, sessionIDs, carIndex)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExportParticipant
	for rows.Next() {
		var p ExportParticipant
		if err := rows.Scan(&p.SessionID, &p.CarIndex, &p.Name, &p.Team, &p.RaceNumber); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Reader) exportLaps(ctx context.Context, sessionIDs []uuid.UUID, lapNumber *int) ([]ExportLap, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT lap_id, session_id, lap_number, started_at, ended_at
		FROM laps
		WHERE session_id = ANY($1) AND ($2::INT IS NULL OR lap_number = $2)
		ORDER BY session_id, lap_number`, sessionIDs, lapNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExportLap
	for rows.Next() {
		var l ExportLap
		if err := rows.Scan(&l.LapID, &l.SessionID, &l.LapNumber, &l.StartedAt, &l.EndedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *Reader) exportSectors(ctx context.Context, lapIDs []uuid.UUID) ([]ExportSector, error) {
	if len(lapIDs) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT lap_id, sector, started_at, ended_at
		FROM sectors WHERE lap_id = ANY($1)
		ORDER BY lap_id, sector`, lapIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExportSector
	for rows.Next() {
		var s ExportSector
		if err := rows.Scan(&s.LapID, &s.Sector, &s.StartedAt, &s.EndedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Reader) exportLapTimes(ctx context.Context, sessionIDs []uuid.UUID, carIndex *int16, lapNumber *int) ([]ExportLapTime, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT session_id, car_index, lap_number, lap_time_ms, lap_invalid, recorded_at
		FROM lap_times
		WHERE session_id = ANY($1)
		  AND ($2::SMALLINT IS NULL OR car_index = $2)
		  AND ($3::INT IS NULL OR lap_number = $3)
		ORDER BY session_id, car_index, lap_number`, sessionIDs, carIndex, lapNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExportLapTime
	for rows.Next() {
		var l ExportLapTime
		if err := rows.Scan(&l.SessionID, &l.CarIndex, &l.LapNumber, &l.LapTimeMS, &l.LapInvalid, &l.RecordedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *Reader) exportTelemetry(ctx context.Context, sessionIDs []uuid.UUID, fields []FieldDef, carIndex *int16, lapNumber *int) ([]ExportTelemetryRow, error) {
	var b strings.Builder
	b.WriteString("t.ts, t.session_id, t.car_index, t.lap_num, t.lap_distance")
	for _, f := range fields {
		b.WriteString(", ")
		b.WriteString(f.Col)
	}
	query := fmt.Sprintf(`
		SELECT %s
		FROM telemetry t
		WHERE t.session_id = ANY($1)
		  AND ($2::SMALLINT IS NULL OR t.car_index = $2)
		  AND ($3::INT IS NULL OR t.lap_num = $3)
		ORDER BY t.session_id, t.car_index, t.ts`, b.String())
	rows, err := r.pool.Query(ctx, query, sessionIDs, carIndex, lapNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ExportTelemetryRow
	for rows.Next() {
		var row ExportTelemetryRow
		row.Values = make([]any, len(fields))
		dest := make([]any, 5+len(fields))
		dest[0] = &row.Ts
		dest[1] = &row.SessionID
		dest[2] = &row.CarIndex
		dest[3] = &row.LapNum
		dest[4] = &row.LapDistance
		for i := range fields {
			dest[5+i] = &row.Values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// Import restores everything in bundle, upserting sessions/participants/
// laps/sectors/lap_times by their primary key (safe to re-run the same
// import twice) and replacing telemetry for exactly the (session_id,
// car_index) pairs present in bundle.Participants — narrow enough that
// importing one driver's single lap never touches another driver's data in
// the same session, but still fully idempotent for that driver's own rows.
func (db *DB) Import(ctx context.Context, bundle *ExportBundle) error {
	if bundle.FormatVersion > ExportFormatVersion {
		return fmt.Errorf("export file format v%d is newer than this app supports (v%d) — update the app first", bundle.FormatVersion, ExportFormatVersion)
	}

	for _, s := range bundle.Sessions {
		if _, err := db.pool.Exec(ctx, `
			INSERT INTO sessions (session_id, started_at, ended_at, game, user_id, track, car)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (session_id) DO UPDATE SET
				started_at = EXCLUDED.started_at, ended_at = EXCLUDED.ended_at,
				game = EXCLUDED.game, user_id = EXCLUDED.user_id,
				track = EXCLUDED.track, car = EXCLUDED.car`,
			s.SessionID, s.StartedAt, s.EndedAt, s.Game, s.UserID, s.Track, s.Car,
		); err != nil {
			return fmt.Errorf("import session %s: %w", s.SessionID, err)
		}
	}

	for _, p := range bundle.Participants {
		if _, err := db.pool.Exec(ctx, `
			INSERT INTO participants (session_id, car_index, name, team, race_number, updated_at)
			VALUES ($1,$2,$3,$4,$5,NOW())
			ON CONFLICT (session_id, car_index) DO UPDATE SET
				name = EXCLUDED.name, team = EXCLUDED.team, race_number = EXCLUDED.race_number, updated_at = NOW()`,
			p.SessionID, p.CarIndex, p.Name, p.Team, p.RaceNumber,
		); err != nil {
			return fmt.Errorf("import participant %s/%d: %w", p.SessionID, p.CarIndex, err)
		}
	}

	for _, l := range bundle.Laps {
		if _, err := db.pool.Exec(ctx, `
			INSERT INTO laps (lap_id, session_id, lap_number, started_at, ended_at)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (lap_id) DO UPDATE SET
				session_id = EXCLUDED.session_id, lap_number = EXCLUDED.lap_number,
				started_at = EXCLUDED.started_at, ended_at = EXCLUDED.ended_at`,
			l.LapID, l.SessionID, l.LapNumber, l.StartedAt, l.EndedAt,
		); err != nil {
			return fmt.Errorf("import lap %s: %w", l.LapID, err)
		}
	}

	for _, s := range bundle.Sectors {
		if _, err := db.pool.Exec(ctx, `
			INSERT INTO sectors (lap_id, sector, started_at, ended_at)
			VALUES ($1,$2,$3,$4)
			ON CONFLICT (lap_id, sector) DO UPDATE SET
				started_at = EXCLUDED.started_at, ended_at = EXCLUDED.ended_at`,
			s.LapID, s.Sector, s.StartedAt, s.EndedAt,
		); err != nil {
			return fmt.Errorf("import sector %s/%d: %w", s.LapID, s.Sector, err)
		}
	}

	for _, lt := range bundle.LapTimes {
		if _, err := db.pool.Exec(ctx, `
			INSERT INTO lap_times (session_id, car_index, lap_number, lap_time_ms, lap_invalid, recorded_at)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (session_id, car_index, lap_number) DO UPDATE SET
				lap_time_ms = EXCLUDED.lap_time_ms, lap_invalid = EXCLUDED.lap_invalid, recorded_at = EXCLUDED.recorded_at`,
			lt.SessionID, lt.CarIndex, lt.LapNumber, lt.LapTimeMS, lt.LapInvalid, lt.RecordedAt,
		); err != nil {
			return fmt.Errorf("import lap_time %s/%d/%d: %w", lt.SessionID, lt.CarIndex, lt.LapNumber, err)
		}
	}

	if err := db.importTelemetry(ctx, bundle); err != nil {
		return fmt.Errorf("import telemetry: %w", err)
	}

	return nil
}

// importTelemetry replaces telemetry for every (session_id, car_index) pair
// named in bundle.Participants — deleting first makes re-running the same
// import idempotent instead of accumulating duplicate rows, since the
// telemetry hypertable has no primary key to upsert against.
func (db *DB) importTelemetry(ctx context.Context, bundle *ExportBundle) error {
	type carKey struct {
		SessionID uuid.UUID
		CarIndex  int16
	}
	seen := make(map[carKey]bool, len(bundle.Participants))
	for _, p := range bundle.Participants {
		k := carKey{p.SessionID, p.CarIndex}
		if seen[k] {
			continue
		}
		seen[k] = true
		if _, err := db.pool.Exec(ctx, `DELETE FROM telemetry WHERE session_id = $1 AND car_index = $2`,
			k.SessionID, k.CarIndex,
		); err != nil {
			return fmt.Errorf("clear existing telemetry for %s/%d: %w", k.SessionID, k.CarIndex, err)
		}
	}

	if len(bundle.Telemetry) == 0 {
		return nil
	}

	// Map each real telemetry column (telemetryCols, writer.go — excludes
	// the derived/computed fields like tyre_surf_avg that AllFields() also
	// exposes for read-side convenience but that don't exist as columns) to
	// its position in bundle.TelemetryFields, so a bundle written by an
	// older/newer export — missing a column, or carrying extra derived
	// fields — still imports the columns that do match instead of failing.
	fieldIndex := make(map[string]int, len(bundle.TelemetryFields))
	for i, name := range bundle.TelemetryFields {
		fieldIndex[name] = i
	}

	dataCols := telemetryCols[3:] // everything after ts, session_id, car_index
	copyRows := make([][]any, len(bundle.Telemetry))
	for i, row := range bundle.Telemetry {
		vals := make([]any, 3+len(dataCols))
		vals[0] = row.Ts
		vals[1] = row.SessionID
		vals[2] = row.CarIndex
		for j, col := range dataCols {
			// lap_num and lap_distance are "structural" columns (see
			// fields.go) carried on ExportTelemetryRow itself rather than
			// in Values/TelemetryFields — handle them before the registry
			// lookup below, which doesn't know about either name.
			switch col {
			case "lap_num":
				vals[3+j] = row.LapNum
				continue
			case "lap_distance":
				vals[3+j] = row.LapDistance
				continue
			}
			idx, ok := fieldIndex[col]
			if !ok || idx >= len(row.Values) {
				vals[3+j] = nil
				continue
			}
			ft := TypeF32
			if def, ok := fieldByName[col]; ok {
				ft = def.Type
			}
			vals[3+j] = convertImportedValue(row.Values[idx], ft)
		}
		copyRows[i] = vals
	}

	_, err := db.pool.CopyFrom(ctx, pgx.Identifier{"telemetry"}, telemetryCols, pgx.CopyFromRows(copyRows))
	return err
}

// convertImportedValue re-types a value that came back from JSON.Unmarshal
// (where every JSON number decodes as float64, and a JSON null as a Go nil)
// into the concrete Go type the telemetry table's COPY protocol expects for
// that column, matching how bulkInsert (writer.go) types its own values.
func convertImportedValue(v any, t FieldType) any {
	if v == nil {
		return nil
	}
	f, ok := v.(float64)
	if !ok {
		return v
	}
	switch t {
	case TypeI16:
		return int16(f)
	case TypeI32:
		return int32(f)
	default:
		return float32(f)
	}
}
