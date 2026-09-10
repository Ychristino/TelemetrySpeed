package consumer

import (
	"encoding/json"
	"log"
	"net/http"

	"NewTelemetryEngine/internal/database"
)

// GET /laptimes?game=&track=
// Returns every valid completed lap across every session ever recorded for
// this game+track, one entry per (session, driver, lap), sorted
// fastest-first. session_id is included so the client can fetch the exact
// recording a lap came from — lap numbers/driver names alone aren't unique
// once a track has more than one session.
func (c *Consumer) handleListLapTimes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	game := q.Get("game")
	track := q.Get("track")
	if game == "" || track == "" {
		http.Error(w, "game and track are required", http.StatusBadRequest)
		return
	}
	laps, err := c.reader.ListLapTimes(r.Context(), game, track)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		log.Printf("[API] ListLapTimes: %v", err)
		return
	}
	type lapTimeJSON struct {
		SessionID string `json:"session_id"`
		CarIndex  int16  `json:"car_index"`
		Driver    string `json:"driver"`
		LapNumber int    `json:"lap_number"`
		LapTimeMS int32  `json:"lap_time_ms"`
	}
	out := make([]lapTimeJSON, len(laps))
	for i, l := range laps {
		out[i] = lapTimeJSON{SessionID: l.SessionID.String(), CarIndex: l.CarIndex, Driver: l.Driver, LapNumber: l.LapNumber, LapTimeMS: l.LapTimeMS}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// GET /idealap?game=&track=
// Returns the theoretical best lap for this game+track: the fastest Sector
// 1, Sector 2, and Sector 3 ever recorded, independently of which lap or
// driver each came from — see database.Reader.BestSectors. Responds with an
// empty object when there isn't yet at least one valid lap contributing a
// candidate for every sector.
func (c *Consumer) handleGetIdealLap(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	game := q.Get("game")
	track := q.Get("track")
	if game == "" || track == "" {
		http.Error(w, "game and track are required", http.StatusBadRequest)
		return
	}
	ideal, err := c.reader.BestSectors(r.Context(), game, track)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		log.Printf("[API] BestSectors: %v", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if ideal == nil {
		json.NewEncoder(w).Encode(map[string]any{})
		return
	}
	type sectorJSON struct {
		Driver    string `json:"driver"`
		SessionID string `json:"session_id"`
		LapNumber int    `json:"lap_number"`
		TimeMS    int32  `json:"time_ms"`
	}
	toJSON := func(s database.SectorSource) sectorJSON {
		return sectorJSON{Driver: s.Driver, SessionID: s.SessionID.String(), LapNumber: s.LapNumber, TimeMS: s.TimeMS}
	}
	json.NewEncoder(w).Encode(map[string]any{
		"ideal_time_ms": ideal.IdealTimeMS,
		"sector1":       toJSON(ideal.Sector1),
		"sector2":       toJSON(ideal.Sector2),
		"sector3":       toJSON(ideal.Sector3),
	})
}

// GET /live?game=&track=
// Returns drivers currently producing telemetry (last few seconds) for this
// game+track, with which session and lap each is currently on — the lap may
// still be in progress.
func (c *Consumer) handleListLive(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	game := q.Get("game")
	track := q.Get("track")
	if game == "" || track == "" {
		http.Error(w, "game and track are required", http.StatusBadRequest)
		return
	}
	live, err := c.reader.ListLiveDrivers(r.Context(), game, track)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		log.Printf("[API] ListLiveDrivers: %v", err)
		return
	}
	type liveJSON struct {
		SessionID string `json:"session_id"`
		Driver    string `json:"driver"`
		LapNumber int    `json:"lap_number"`
	}
	out := make([]liveJSON, len(live))
	for i, l := range live {
		out[i] = liveJSON{SessionID: l.SessionID.String(), Driver: l.Driver, LapNumber: l.LapNumber}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
