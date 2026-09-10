package database

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Reader struct {
	pool *pgxpool.Pool
	// liveWindow: how recent a driver's last telemetry row must be to count
	// as "currently live" — see ListLiveDrivers.
	liveWindow time.Duration
}

// TelemetryFilter identifies a specific lap for a specific driver.
// SessionID, when set, pins the lookup to one exact recording — required
// once a game+track has more than one session, since lap numbers and driver
// names alone are only unique within a single session. When SessionID is
// uuid.Nil, the lookup falls back to the most recent session for Game+Track.
type TelemetryFilter struct {
	Game      string
	Track     string
	Lap       int
	Driver    string // participant name, case-insensitive
	SessionID uuid.UUID
}

// SessionRecord is returned by ListSessions.
type SessionRecord struct {
	SessionID uuid.UUID
	StartedAt time.Time
	EndedAt   *time.Time
	Game      string
	Track     string
}

// LapRecord is returned nested in SessionDetail.
type LapRecord struct {
	LapID     uuid.UUID
	LapNumber int
	StartedAt time.Time
	EndedAt   *time.Time
}

// SessionDetail is returned by GetSession.
type SessionDetail struct {
	SessionRecord
	Laps []LapRecord
}

func (r *Reader) ListSessions(ctx context.Context, game, track string) ([]SessionRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT session_id, started_at, ended_at, game, track
		FROM sessions
		WHERE ($1 = '' OR game = $1) AND ($2 = '' OR track = $2)
		ORDER BY started_at DESC
		LIMIT 200`,
		game, track,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SessionRecord
	for rows.Next() {
		var s SessionRecord
		if err := rows.Scan(&s.SessionID, &s.StartedAt, &s.EndedAt, &s.Game, &s.Track); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Reader) GetSession(ctx context.Context, id uuid.UUID) (*SessionDetail, error) {
	var d SessionDetail
	err := r.pool.QueryRow(ctx, `
		SELECT session_id, started_at, ended_at, game, track
		FROM sessions WHERE session_id = $1`, id,
	).Scan(&d.SessionID, &d.StartedAt, &d.EndedAt, &d.Game, &d.Track)
	if err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT lap_id, lap_number, started_at, ended_at
		FROM laps WHERE session_id = $1
		ORDER BY lap_number`, id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var l LapRecord
		if err := rows.Scan(&l.LapID, &l.LapNumber, &l.StartedAt, &l.EndedAt); err != nil {
			return nil, err
		}
		d.Laps = append(d.Laps, l)
	}
	return &d, rows.Err()
}

// LapTimeRecord is returned by ListLapTimes — one completed, valid lap for
// one driver in one specific session.
type LapTimeRecord struct {
	SessionID uuid.UUID
	CarIndex  int16
	Driver    string
	LapNumber int
	LapTimeMS int32
}

// ListLapTimes returns every valid completed lap across every session ever
// recorded for this game+track, sorted fastest-first. A new session row is
// created each time recording (re)starts, so aggregating across all of them
// — not just the latest — is what makes historical laps from earlier
// recordings show up instead of only whatever's currently active.
// lap_times is a small precomputed summary table (one row per car per lap,
// written at lap rollover — see SourceHandler.recordLapTimes), not the raw
// telemetry hypertable, so this stays cheap regardless of how much history
// has accumulated for this game+track.
//
// lt.lap_invalid = 0 alone isn't enough: it reflects the game's own
// m_currentLapInvalid flag (rule violations), which recordLapTimes already
// forces to invalid for a lap that never reached the final sector, but
// older rows recorded before that check existed can still say lap_invalid=0
// without ever having covered all three sectors. The coverage CTE below
// re-derives that independently from the raw telemetry rows, so this query
// stays correct regardless of what's stored on the summary row.
//
// A listener restart mid-drive (reconnect after a crash, re-opening the
// capture screen) starts a brand-new session without the driver doing
// anything different, so the same physical lap can legitimately end up
// recorded under two different session_ids. Two real laps landing on the
// exact same millisecond is effectively impossible, so DISTINCT ON
// (driver, lap_number, lap_time_ms) collapses those into one entry —
// keeping the most recently-started session's copy, since that's the one
// most likely to still have its full telemetry retained — while genuinely
// different laps (any different time) still all show up.
func (r *Reader) ListLapTimes(ctx context.Context, game, track string) ([]LapTimeRecord, error) {
	rows, err := r.pool.Query(ctx, `
		WITH coverage AS (
			SELECT t.session_id, t.car_index, t.lap_num,
			       bool_or(t.sector = 0) AS has_sector1,
			       bool_or(t.sector = 1) AS has_sector2,
			       bool_or(t.sector = 2) AS has_sector3
			FROM telemetry t
			JOIN sessions s ON s.session_id = t.session_id
			WHERE s.game = $1 AND s.track = $2
			GROUP BY t.session_id, t.car_index, t.lap_num
		),
		deduped AS (
			SELECT DISTINCT ON (p.name, lt.lap_number, lt.lap_time_ms)
			       s.session_id, lt.car_index, p.name AS driver, lt.lap_number, lt.lap_time_ms
			FROM lap_times lt
			JOIN sessions s ON s.session_id = lt.session_id
			JOIN participants p ON p.session_id = lt.session_id AND p.car_index = lt.car_index
			JOIN coverage c ON c.session_id = lt.session_id AND c.car_index = lt.car_index AND c.lap_num = lt.lap_number
			WHERE s.game = $1 AND s.track = $2 AND lt.lap_invalid = 0
			  AND c.has_sector1 AND c.has_sector2 AND c.has_sector3
			ORDER BY p.name, lt.lap_number, lt.lap_time_ms, s.started_at DESC
		)
		SELECT session_id, car_index, driver, lap_number, lap_time_ms
		FROM deduped
		ORDER BY lap_time_ms ASC`,
		game, track,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LapTimeRecord
	for rows.Next() {
		var l LapTimeRecord
		if err := rows.Scan(&l.SessionID, &l.CarIndex, &l.Driver, &l.LapNumber, &l.LapTimeMS); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SectorSource identifies the lap that produced one winning sector time in
// BestSectors — which session/car/lap it came from, and its driver name for
// display.
type SectorSource struct {
	SessionID uuid.UUID
	CarIndex  int16
	Driver    string
	LapNumber int
	TimeMS    int32
}

// IdealLap is the theoretical best lap for a game+track: the fastest Sector
// 1, Sector 2, and Sector 3 ever recorded, independently of which lap (or
// driver) each came from.
type IdealLap struct {
	Sector1     SectorSource
	Sector2     SectorSource
	Sector3     SectorSource
	IdealTimeMS int32
}

// BestSectors computes the IdealLap for a game+track by deriving each
// recorded lap's three sector durations from the telemetry hypertable's
// per-row `sector` column (0/1/2) and that lap's finishing time in
// lap_times, then picking the fastest of each sector independently.
//
// s2_start_ms/s3_start_ms are the lap_time_ms value at the first row of
// sector 2/3 — i.e. how far into the lap sector 1 (resp. sector 1+2) had
// run when the car crossed into the next sector. Sector durations follow by
// subtraction, using lap_times.lap_time_ms (captured at the exact rollover
// frame, not a telemetry sample) as the lap's true finish time.
//
// Returns nil, nil if fewer than one valid lap contributes a candidate for
// every sector (not enough data yet to build an ideal lap).
func (r *Reader) BestSectors(ctx context.Context, game, track string) (*IdealLap, error) {
	rows, err := r.pool.Query(ctx, `
		WITH bounds AS (
			SELECT t.session_id, t.car_index, t.lap_num,
			       bool_or(t.sector = 0) AS has_sector1,
			       MIN(t.lap_time_ms) FILTER (WHERE t.sector = 1) AS s2_start_ms,
			       MIN(t.lap_time_ms) FILTER (WHERE t.sector = 2) AS s3_start_ms
			FROM telemetry t
			JOIN sessions s ON s.session_id = t.session_id
			WHERE s.game = $1 AND s.track = $2
			GROUP BY t.session_id, t.car_index, t.lap_num
		)
		SELECT b.session_id, b.car_index, p.name, b.lap_num,
		       b.s2_start_ms AS sector1_ms,
		       b.s3_start_ms - b.s2_start_ms AS sector2_ms,
		       lt.lap_time_ms - b.s3_start_ms AS sector3_ms
		FROM bounds b
		JOIN lap_times lt ON lt.session_id = b.session_id AND lt.car_index = b.car_index AND lt.lap_number = b.lap_num
		JOIN participants p ON p.session_id = b.session_id AND p.car_index = b.car_index
		WHERE lt.lap_invalid = 0
		  AND b.has_sector1
		  AND b.s2_start_ms > 0 AND b.s3_start_ms > b.s2_start_ms AND lt.lap_time_ms > b.s3_start_ms`,
		game, track,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var best1, best2, best3 *SectorSource
	for rows.Next() {
		var sessionID uuid.UUID
		var carIndex int16
		var driver string
		var lapNum int
		var s1, s2, s3 int32
		if err := rows.Scan(&sessionID, &carIndex, &driver, &lapNum, &s1, &s2, &s3); err != nil {
			return nil, err
		}
		if best1 == nil || s1 < best1.TimeMS {
			best1 = &SectorSource{SessionID: sessionID, CarIndex: carIndex, Driver: driver, LapNumber: lapNum, TimeMS: s1}
		}
		if best2 == nil || s2 < best2.TimeMS {
			best2 = &SectorSource{SessionID: sessionID, CarIndex: carIndex, Driver: driver, LapNumber: lapNum, TimeMS: s2}
		}
		if best3 == nil || s3 < best3.TimeMS {
			best3 = &SectorSource{SessionID: sessionID, CarIndex: carIndex, Driver: driver, LapNumber: lapNum, TimeMS: s3}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if best1 == nil || best2 == nil || best3 == nil {
		return nil, nil
	}
	return &IdealLap{
		Sector1:     *best1,
		Sector2:     *best2,
		Sector3:     *best3,
		IdealTimeMS: best1.TimeMS + best2.TimeMS + best3.TimeMS,
	}, nil
}

// LiveDriverRecord is returned by ListLiveDrivers — a driver currently
// producing telemetry, the session it's landing in, and the lap they're on
// right now.
type LiveDriverRecord struct {
	SessionID uuid.UUID
	Driver    string
	LapNumber int
}

// ListLiveDrivers returns every driver whose most recent telemetry row for
// this game+track arrived within the last few seconds, along with which
// session it's in and the lap they're currently on (which may still be in
// progress / incomplete). Scoped by game+track rather than a single session
// ID for the same reason as ListLapTimes — but live rows are by definition
// recent, so in practice this only ever matches whichever session is
// actively recording right now.
//
// telemetry is a TimescaleDB hypertable partitioned on ts, so the time bound
// here isn't just an optimization — without it, Postgres has to scan every
// chunk for every session at this track (which only grows as a live session
// runs) and pick the top row per driver from that, rather than pruning
// straight to the last few seconds. On an actively-recording session that
// scan can take long enough that the "is it recent" check fails by the time
// it finishes, making live drivers falsely disappear.
func (r *Reader) ListLiveDrivers(ctx context.Context, game, track string) ([]LiveDriverRecord, error) {
	cutoff := time.Now().Add(-r.liveWindow)
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (p.name) s.session_id, p.name, t.lap_num, t.ts
		FROM telemetry t
		JOIN participants p ON p.session_id = t.session_id AND p.car_index = t.car_index
		JOIN sessions s ON s.session_id = t.session_id
		WHERE s.game = $1 AND s.track = $2 AND t.ts > $3
		ORDER BY p.name, t.ts DESC`,
		game, track, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LiveDriverRecord
	for rows.Next() {
		var sessionID uuid.UUID
		var driver string
		var lapNum int16
		var ts time.Time
		if err := rows.Scan(&sessionID, &driver, &lapNum, &ts); err != nil {
			return nil, err
		}
		out = append(out, LiveDriverRecord{SessionID: sessionID, Driver: driver, LapNumber: int(lapNum)})
	}
	return out, rows.Err()
}

// ParticipantRecord is returned by ListParticipants.
type ParticipantRecord struct {
	CarIndex   int16
	Name       string
	Team       string
	RaceNumber int16
}

// ListParticipants returns one row per distinct driver name in the session.
// A name can be attached to more than one car_index — see ResolveCarIndex's
// doc comment for why — so this dedupes on lower(name), keeping only the
// most recently updated row per driver, the same rule ResolveCarIndex uses.
// Without it, a driver who moved slots would show up twice in the roster.
func (r *Reader) ListParticipants(ctx context.Context, sessionID uuid.UUID) ([]ParticipantRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT car_index, name, team, race_number FROM (
			SELECT DISTINCT ON (lower(name)) car_index, name, team, race_number
			FROM participants
			WHERE session_id = $1
			ORDER BY lower(name), updated_at DESC
		) AS deduped
		ORDER BY car_index`, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ParticipantRecord
	for rows.Next() {
		var p ParticipantRecord
		if err := rows.Scan(&p.CarIndex, &p.Name, &p.Team, &p.RaceNumber); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ResolveSessionID returns the most recent session for game+track. Used once
// per WebSocket connection (when the caller didn't already pin a
// session_id) so the poll loop can filter telemetry directly instead of
// re-resolving game/track on every tick — see PollTelemetryPinned.
func (r *Reader) ResolveSessionID(ctx context.Context, game, track string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT session_id FROM sessions
		WHERE game = $1 AND track = $2
		ORDER BY started_at DESC LIMIT 1`, game, track,
	).Scan(&id)
	return id, err
}

// ResolveCarIndex returns the car_index for a driver name (case-insensitive)
// within one session. Used once per WebSocket connection alongside
// ResolveSessionID — see PollTelemetryPinned.
//
// A driver's name can end up attached to more than one car_index within the
// same session: the Participants packet is re-sent periodically, keyed by
// slot index, and a slot that goes empty (driver reconnects into a
// different slot, say) is skipped rather than cleared — see
// SourceHandler.handleParticipants — so the old row lingers with the same
// name. ORDER BY updated_at DESC picks whichever row was most recently
// confirmed, i.e. the driver's current car, instead of an arbitrary match.
func (r *Reader) ResolveCarIndex(ctx context.Context, sessionID uuid.UUID, driver string) (int16, error) {
	var idx int16
	err := r.pool.QueryRow(ctx, `
		SELECT car_index FROM participants
		WHERE session_id = $1 AND lower(name) = lower($2)
		ORDER BY updated_at DESC LIMIT 1`, sessionID, driver,
	).Scan(&idx)
	return idx, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

// TelemetryQueryRow is one row of query results scoped to a caller-chosen
// field list. Ts, CarIndex, LapNum, and LapDistance are always present
// (needed for the time/distance X axis and live-stream lap-rollover
// detection regardless of what was requested); Values holds one entry per
// field in the []FieldDef the caller resolved, in the same order — nil for
// a NULL MotionEx cell on a non-player car.
type TelemetryQueryRow struct {
	Ts          time.Time
	CarIndex    int16
	LapNum      int16
	LapDistance float32
	Values      []any
}

// scanTelemetryRow scans the fixed header columns plus one column per
// requested field. Scanning into `any` lets pgx pick the right Go type per
// column (float32 for REAL, int16 for SMALLINT, int32 for INTEGER, nil for
// SQL NULL) without this code needing to special-case each one.
func scanTelemetryRow(s rowScanner, fields []FieldDef) (TelemetryQueryRow, error) {
	var row TelemetryQueryRow
	row.Values = make([]any, len(fields))
	dest := make([]any, 4+len(fields))
	dest[0] = &row.Ts
	dest[1] = &row.CarIndex
	dest[2] = &row.LapNum
	dest[3] = &row.LapDistance
	for i := range fields {
		dest[4+i] = &row.Values[i]
	}
	if err := s.Scan(dest...); err != nil {
		return TelemetryQueryRow{}, err
	}
	return row, nil
}

// selectCols builds the SELECT list: the four fixed header columns plus one
// per requested field. fields is always a []FieldDef resolved through
// ResolveFields/AllFields, so every Col here comes from the fixed registry —
// never from unvalidated request input.
func selectCols(fields []FieldDef) string {
	var b strings.Builder
	b.WriteString("t.ts, t.car_index, t.lap_num, t.lap_distance")
	for _, f := range fields {
		b.WriteString(", ")
		b.WriteString(f.Col)
	}
	return b.String()
}

func telemetryBaseQuery(fields []FieldDef) string {
	return `
	WITH session AS (
		SELECT s.session_id
		FROM sessions s
		WHERE s.game = $1 AND s.track = $2
		ORDER BY s.started_at DESC
		LIMIT 1
	)
	SELECT ` + selectCols(fields) + `
	FROM telemetry t
	JOIN participants p ON p.session_id = t.session_id AND p.car_index = t.car_index
	JOIN session ON session.session_id = t.session_id
	WHERE t.lap_num = $3 AND lower(p.name) = lower($4)`
}

// telemetryLiveQuery is like telemetryBaseQuery but without a fixed lap_num
// filter — used for live-follow streaming, where the caller doesn't know in
// advance which lap the driver will be on and wants every row as it arrives,
// across lap boundaries.
func telemetryLiveQuery(fields []FieldDef) string {
	return `
	WITH session AS (
		SELECT s.session_id
		FROM sessions s
		WHERE s.game = $1 AND s.track = $2
		ORDER BY s.started_at DESC
		LIMIT 1
	)
	SELECT ` + selectCols(fields) + `
	FROM telemetry t
	JOIN participants p ON p.session_id = t.session_id AND p.car_index = t.car_index
	JOIN session ON session.session_id = t.session_id
	WHERE lower(p.name) = lower($3)`
}

// telemetryQuery builds the WHERE-scoped query + args for a filter. When
// f.SessionID is set it pins to that exact recording (needed once a
// game+track has more than one session); otherwise it falls back to
// resolving the most recent session for Game+Track, same as before.
func telemetryQuery(f TelemetryFilter, fields []FieldDef) (string, []any) {
	if f.SessionID != uuid.Nil {
		base := `
			SELECT ` + selectCols(fields) + `
			FROM telemetry t
			JOIN participants p ON p.session_id = t.session_id AND p.car_index = t.car_index
			WHERE t.session_id = $1 AND lower(p.name) = lower($2)`
		if f.Lap > 0 {
			return base + ` AND t.lap_num = $3`, []any{f.SessionID, f.Driver, f.Lap}
		}
		return base, []any{f.SessionID, f.Driver}
	}
	if f.Lap > 0 {
		return telemetryBaseQuery(fields), []any{f.Game, f.Track, f.Lap, f.Driver}
	}
	return telemetryLiveQuery(fields), []any{f.Game, f.Track, f.Driver}
}

// QueryTelemetry returns telemetry for a lap in one shot, scoped to fields.
func (r *Reader) QueryTelemetry(ctx context.Context, f TelemetryFilter, fields []FieldDef) ([]TelemetryQueryRow, error) {
	query, args := telemetryQuery(f, fields)
	rows, err := r.pool.Query(ctx, query+` ORDER BY t.ts ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TelemetryQueryRow
	for rows.Next() {
		row, err := scanTelemetryRow(rows, fields)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// QueryTelemetrySector returns the telemetry rows for one specific lap that
// fall within one specific sector (0/1/2) — the building block for stitching
// an IdealLap's three winning segments into one virtual lap. No participants
// join is needed since the caller already knows carIndex (from BestSectors).
func (r *Reader) QueryTelemetrySector(ctx context.Context, sessionID uuid.UUID, carIndex int16, lapNumber, sector int, fields []FieldDef) ([]TelemetryQueryRow, error) {
	query := `SELECT ` + selectCols(fields) + `
		FROM telemetry t
		WHERE t.session_id = $1 AND t.car_index = $2 AND t.lap_num = $3 AND t.sector = $4
		ORDER BY t.ts ASC`
	rows, err := r.pool.Query(ctx, query, sessionID, carIndex, lapNumber, sector)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TelemetryQueryRow
	for rows.Next() {
		row, err := scanTelemetryRow(rows, fields)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// BuildIdealLapRows concatenates three sector segments (each already ordered
// by ts ASC, from three possibly-different laps/drivers) into one continuous
// virtual lap. Segment 1 anchors the start; segment 2 and 3 are shifted so
// their first row lands exactly on the previous segment's last row (in both
// Ts and LapDistance), then that duplicate leading row is dropped — keeping
// the whole stream monotonically increasing, so it charts and deltas exactly
// like a real single-lap fetch.
func BuildIdealLapRows(seg1, seg2, seg3 []TelemetryQueryRow) []TelemetryQueryRow {
	out := make([]TelemetryQueryRow, 0, len(seg1)+len(seg2)+len(seg3))
	out = append(out, seg1...)
	for _, seg := range [][]TelemetryQueryRow{seg2, seg3} {
		if len(seg) == 0 {
			continue
		}
		if len(out) == 0 {
			out = append(out, seg...)
			continue
		}
		prevLast := out[len(out)-1]
		tsOffset := prevLast.Ts.Sub(seg[0].Ts)
		distOffset := prevLast.LapDistance - seg[0].LapDistance
		rest := seg[1:] // drop the row that would duplicate prevLast
		for _, row := range rest {
			row.Ts = row.Ts.Add(tsOffset)
			row.LapDistance += distOffset
			out = append(out, row)
		}
	}
	return out
}

// telemetryPinnedQuery builds a join-free poll query filtered directly by
// session_id + car_index (and lap_num, if hasLap) — an index range scan on
// idx_telemetry_session_car_ts, nothing else. $1=session_id, $2=car_index,
// [$3=lap_num,] last placeholder=since (appended by the caller).
func telemetryPinnedQuery(fields []FieldDef, hasLap bool) string {
	q := `SELECT ` + selectCols(fields) + ` FROM telemetry t WHERE t.session_id = $1 AND t.car_index = $2`
	if hasLap {
		q += ` AND t.lap_num = $3`
	}
	return q
}

// PollTelemetryPinned is PollTelemetry's join-free counterpart: it takes an
// already-resolved session_id + car_index (see ResolveSessionID /
// ResolveCarIndex) instead of game/track/driver, so a tight polling loop —
// a live WebSocket stream ticking every 100-500ms — doesn't re-run the
// participants/sessions joins on every single tick, just resolves the
// driver once at connect time and then hits the (session_id, car_index, ts)
// index directly from then on. lap <= 0 means "live-follow": stream every
// row for the car regardless of lap.
func (r *Reader) PollTelemetryPinned(ctx context.Context, sessionID uuid.UUID, carIndex int16, lap int, fields []FieldDef, since time.Time) ([]TelemetryQueryRow, time.Time, error) {
	args := []any{sessionID, carIndex}
	if lap > 0 {
		args = append(args, lap)
	}
	args = append(args, since)
	tsPlaceholder := fmt.Sprintf("$%d", len(args))
	query := telemetryPinnedQuery(fields, lap > 0)
	rows, err := r.pool.Query(ctx, query+` AND t.ts > `+tsPlaceholder+` ORDER BY t.ts ASC`, args...)
	if err != nil {
		return nil, since, err
	}
	defer rows.Close()

	var out []TelemetryQueryRow
	hwm := since
	for rows.Next() {
		row, err := scanTelemetryRow(rows, fields)
		if err != nil {
			return nil, since, err
		}
		out = append(out, row)
		if row.Ts.After(hwm) {
			hwm = row.Ts
		}
	}
	return out, hwm, rows.Err()
}

// Binary encoding — little-endian, variable-width per requested field set.
//
// Every row starts with the fixed header (ts int64, car_index int16,
// lap_num int16, lap_distance float32 = 16 bytes), followed by one value
// per field in `fields`, each width-per-FieldType.Size().
//
// HTTP response:   [0x54, 0x4D, version=2, field_count u8, field_id×N u8,
//                   row_count u32 LE, rows...]
// WebSocket frame: [row_count u32 LE, rows...] — the field list is sent once
// as a JSON ack ({"fields":[...]}) right after the stream filter is
// accepted, since it doesn't change for the life of the connection.

// EncodeTelemetryBinary encodes rows as a full HTTP binary response.
func EncodeTelemetryBinary(rows []TelemetryQueryRow, fields []FieldDef) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, 8+len(fields)+len(rows)*rowSize(fields)))
	buf.WriteByte(0x54) // 'T'
	buf.WriteByte(0x4D) // 'M'
	buf.WriteByte(2)    // version
	buf.WriteByte(byte(len(fields)))
	for _, f := range fields {
		buf.WriteByte(f.ID)
	}
	writeU32(buf, uint32(len(rows)))
	for i := range rows {
		encodeTelemetryRow(buf, &rows[i], fields)
	}
	return buf.Bytes()
}

// EncodeTelemetryFrame encodes rows as a WebSocket binary frame — no field
// header, since the client already has the layout from the stream's ack.
func EncodeTelemetryFrame(rows []TelemetryQueryRow, fields []FieldDef) []byte {
	var buf bytes.Buffer
	buf.Grow(4 + len(rows)*rowSize(fields))
	EncodeTelemetryFrameInto(&buf, rows, fields)
	return buf.Bytes()
}

// EncodeTelemetryFrameInto is EncodeTelemetryFrame but writes into a
// caller-owned buffer instead of allocating a new one — for a tight polling
// loop (e.g. a live WebSocket stream ticking every 100-500ms) reusing one
// buffer across ticks avoids allocating and immediately discarding a fresh
// one on every single frame. Callers must Reset buf before each call.
func EncodeTelemetryFrameInto(buf *bytes.Buffer, rows []TelemetryQueryRow, fields []FieldDef) {
	writeU32(buf, uint32(len(rows)))
	for i := range rows {
		encodeTelemetryRow(buf, &rows[i], fields)
	}
}

func rowSize(fields []FieldDef) int {
	n := 16
	for _, f := range fields {
		n += f.Type.Size()
	}
	return n
}

func writeU32(w *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	w.Write(b[:])
}

func encodeTelemetryRow(w *bytes.Buffer, r *TelemetryQueryRow, fields []FieldDef) {
	var b [8]byte
	le := binary.LittleEndian

	le.PutUint64(b[:8], uint64(r.Ts.UnixNano()))
	w.Write(b[:8])
	le.PutUint16(b[:2], uint16(r.CarIndex))
	w.Write(b[:2])
	le.PutUint16(b[:2], uint16(r.LapNum))
	w.Write(b[:2])
	le.PutUint32(b[:4], math.Float32bits(r.LapDistance))
	w.Write(b[:4])

	for i, f := range fields {
		v := r.Values[i]
		switch f.Type {
		case TypeF32:
			var x float32
			if v != nil {
				x = v.(float32)
			}
			le.PutUint32(b[:4], math.Float32bits(x))
			w.Write(b[:4])
		case TypeI32:
			var x int32
			if v != nil {
				x = v.(int32)
			}
			le.PutUint32(b[:4], uint32(x))
			w.Write(b[:4])
		case TypeI16:
			var x int16
			if v != nil {
				x = v.(int16)
			}
			le.PutUint16(b[:2], uint16(x))
			w.Write(b[:2])
		}
	}
}
