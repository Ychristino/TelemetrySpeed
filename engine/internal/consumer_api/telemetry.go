package consumer

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"NewTelemetryEngine/internal/database"

	"github.com/google/uuid"
)

// GET /telemetry?game=&track=&lap=&driver=&session_id=&fields=
// session_id is optional — when omitted, falls back to the most recent
// session for game+track (only correct once a track has just one session).
// fields is an optional comma-separated list of field names (see
// database.ResolveFields) selecting which optional columns to return —
// omit it to get every field, same as before this parameter existed.
// Returns binary: [0x54, 0x4D, version=2, field_count u8, field_id×N u8,
// row_count u32 LE, rows...] — see database.EncodeTelemetryBinary.
func (c *Consumer) handleGetTelemetry(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	game := q.Get("game")
	track := q.Get("track")
	driver := q.Get("driver")
	lapStr := q.Get("lap")

	if game == "" || track == "" || driver == "" || lapStr == "" {
		http.Error(w, "game, track, lap, and driver are required", http.StatusBadRequest)
		return
	}
	lap, err := strconv.Atoi(lapStr)
	if err != nil || lap < 1 {
		http.Error(w, "lap must be a positive integer", http.StatusBadRequest)
		return
	}

	var sessionID uuid.UUID
	if sidStr := q.Get("session_id"); sidStr != "" {
		sessionID, err = uuid.Parse(sidStr)
		if err != nil {
			http.Error(w, "invalid session_id", http.StatusBadRequest)
			return
		}
	}

	fields, err := resolveFieldsParam(q.Get("fields"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rows, err := c.reader.QueryTelemetry(r.Context(), database.TelemetryFilter{
		Game: game, Track: track, Lap: lap, Driver: driver, SessionID: sessionID,
	}, fields)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		log.Printf("[API] QueryTelemetry: %v", err)
		return
	}

	data := database.EncodeTelemetryBinary(rows, fields)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

// GET /telemetry/ideal?game=&track=&fields=
// Returns the theoretical best lap for this game+track — the fastest
// Sector 1/2/3 ever recorded (see database.Reader.BestSectors), stitched
// into one continuous virtual lap (database.BuildIdealLapRows) and encoded
// exactly like a real /telemetry response, so the client needs no special
// handling to chart it. 404 if there isn't enough data yet for all three
// sectors.
func (c *Consumer) handleGetIdealLapTelemetry(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	game := q.Get("game")
	track := q.Get("track")
	if game == "" || track == "" {
		http.Error(w, "game and track are required", http.StatusBadRequest)
		return
	}

	fields, err := resolveFieldsParam(q.Get("fields"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ideal, err := c.reader.BestSectors(r.Context(), game, track)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		log.Printf("[API] BestSectors: %v", err)
		return
	}
	if ideal == nil {
		http.Error(w, "not enough data for an ideal lap yet", http.StatusNotFound)
		return
	}

	seg1, err := c.reader.QueryTelemetrySector(r.Context(), ideal.Sector1.SessionID, ideal.Sector1.CarIndex, ideal.Sector1.LapNumber, 0, fields)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		log.Printf("[API] QueryTelemetrySector s1: %v", err)
		return
	}
	seg2, err := c.reader.QueryTelemetrySector(r.Context(), ideal.Sector2.SessionID, ideal.Sector2.CarIndex, ideal.Sector2.LapNumber, 1, fields)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		log.Printf("[API] QueryTelemetrySector s2: %v", err)
		return
	}
	seg3, err := c.reader.QueryTelemetrySector(r.Context(), ideal.Sector3.SessionID, ideal.Sector3.CarIndex, ideal.Sector3.LapNumber, 2, fields)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		log.Printf("[API] QueryTelemetrySector s3: %v", err)
		return
	}

	rows := database.BuildIdealLapRows(seg1, seg2, seg3)
	data := database.EncodeTelemetryBinary(rows, fields)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

// resolveFieldsParam parses a comma-separated `fields` query/filter value.
// An empty string means "no selection given" — returns every field, so
// omitting it keeps the pre-selection full-row behavior.
func resolveFieldsParam(raw string) ([]database.FieldDef, error) {
	if raw == "" {
		return database.AllFields(), nil
	}
	names := strings.Split(raw, ",")
	for i, n := range names {
		names[i] = strings.TrimSpace(n)
	}
	return database.ResolveFields(names)
}
