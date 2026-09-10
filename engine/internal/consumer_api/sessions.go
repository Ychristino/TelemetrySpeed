package consumer

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// GET /sessions?game=&track=
func (c *Consumer) handleListSessions(w http.ResponseWriter, r *http.Request) {
	game := r.URL.Query().Get("game")
	track := r.URL.Query().Get("track")

	sessions, err := c.reader.ListSessions(r.Context(), game, track)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		log.Printf("[API] ListSessions: %v", err)
		return
	}

	type sessionJSON struct {
		SessionID string  `json:"session_id"`
		StartedAt string  `json:"started_at"`
		EndedAt   *string `json:"ended_at,omitempty"`
		Game      string  `json:"game"`
		Track     string  `json:"track"`
	}
	out := make([]sessionJSON, len(sessions))
	for i, s := range sessions {
		out[i] = sessionJSON{
			SessionID: s.SessionID.String(),
			StartedAt: s.StartedAt.UTC().Format(time.RFC3339),
			Game:      s.Game,
			Track:     s.Track,
		}
		if s.EndedAt != nil {
			t := s.EndedAt.UTC().Format(time.RFC3339)
			out[i].EndedAt = &t
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// GET /sessions/{id}
func (c *Consumer) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid session id", http.StatusBadRequest)
		return
	}

	detail, err := c.reader.GetSession(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	type lapJSON struct {
		LapID     string  `json:"lap_id"`
		LapNumber int     `json:"lap_number"`
		StartedAt string  `json:"started_at"`
		EndedAt   *string `json:"ended_at,omitempty"`
	}
	type sessionJSON struct {
		SessionID string    `json:"session_id"`
		StartedAt string    `json:"started_at"`
		EndedAt   *string   `json:"ended_at,omitempty"`
		Game      string    `json:"game"`
		Track     string    `json:"track"`
		Laps      []lapJSON `json:"laps"`
	}

	out := sessionJSON{
		SessionID: detail.SessionID.String(),
		StartedAt: detail.StartedAt.UTC().Format(time.RFC3339),
		Game:      detail.Game,
		Track:     detail.Track,
	}
	if detail.EndedAt != nil {
		t := detail.EndedAt.UTC().Format(time.RFC3339)
		out.EndedAt = &t
	}
	for _, l := range detail.Laps {
		lj := lapJSON{
			LapID:     l.LapID.String(),
			LapNumber: l.LapNumber,
			StartedAt: l.StartedAt.UTC().Format(time.RFC3339),
		}
		if l.EndedAt != nil {
			t := l.EndedAt.UTC().Format(time.RFC3339)
			lj.EndedAt = &t
		}
		out.Laps = append(out.Laps, lj)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// GET /sessions/{id}/participants
func (c *Consumer) handleListParticipants(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid session id", http.StatusBadRequest)
		return
	}
	participants, err := c.reader.ListParticipants(r.Context(), id)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		log.Printf("[API] ListParticipants: %v", err)
		return
	}
	type pJSON struct {
		CarIndex   int16  `json:"car_index"`
		Name       string `json:"name"`
		Team       string `json:"team"`
		RaceNumber int16  `json:"race_number"`
	}
	out := make([]pJSON, len(participants))
	for i, p := range participants {
		out[i] = pJSON{CarIndex: p.CarIndex, Name: p.Name, Team: p.Team, RaceNumber: p.RaceNumber}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
