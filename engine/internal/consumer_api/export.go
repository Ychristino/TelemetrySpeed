package consumer

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"NewTelemetryEngine/internal/database"

	"github.com/google/uuid"
)

// GET /export?scope=all|track|lap&game=&track=&session_id=&car_index=&lap_number=
// Streams a gzip-compressed JSON snapshot (see database.ExportBundle) as a
// file download. track scope needs game+track; lap scope needs session_id,
// car_index, and lap_number (all three, since laps/times/telemetry are only
// unique per session — see database.Export's docs).
func (c *Consumer) handleExport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	scope := database.ExportScope{Kind: q.Get("scope")}

	switch scope.Kind {
	case "all":
		// no extra params
	case "track":
		scope.Game = q.Get("game")
		scope.Track = q.Get("track")
		if scope.Game == "" || scope.Track == "" {
			http.Error(w, "game and track are required for track scope", http.StatusBadRequest)
			return
		}
	case "lap":
		sid, err := uuid.Parse(q.Get("session_id"))
		if err != nil {
			http.Error(w, "a valid session_id is required for lap scope", http.StatusBadRequest)
			return
		}
		ci, err := strconv.Atoi(q.Get("car_index"))
		if err != nil {
			http.Error(w, "a valid car_index is required for lap scope", http.StatusBadRequest)
			return
		}
		ln, err := strconv.Atoi(q.Get("lap_number"))
		if err != nil {
			http.Error(w, "a valid lap_number is required for lap scope", http.StatusBadRequest)
			return
		}
		scope.SessionID = sid
		scope.CarIndex = int16(ci)
		scope.LapNumber = ln
	default:
		http.Error(w, `scope must be "all", "track", or "lap"`, http.StatusBadRequest)
		return
	}

	bundle, err := c.reader.Export(r.Context(), scope)
	if err != nil {
		log.Printf("[API] Export: %v", err)
		http.Error(w, "export failed", http.StatusInternalServerError)
		return
	}

	payload, err := json.Marshal(bundle)
	if err != nil {
		log.Printf("[API] Export marshal: %v", err)
		http.Error(w, "export failed", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(payload); err != nil {
		log.Printf("[API] Export gzip: %v", err)
		http.Error(w, "export failed", http.StatusInternalServerError)
		return
	}
	if err := gw.Close(); err != nil {
		log.Printf("[API] Export gzip close: %v", err)
		http.Error(w, "export failed", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("telemetry-%s.tvexport", scope.Kind)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// POST /import — body is a .tvexport file exactly as produced by
// handleExport (gzip-compressed JSON). Restores every row it carries; see
// database.DB.Import for the upsert/idempotency rules.
func (c *Consumer) handleImport(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 512<<20)) // 512MB cap
	if err != nil {
		http.Error(w, "failed to read upload", http.StatusBadRequest)
		return
	}

	gr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		http.Error(w, "not a valid export file", http.StatusBadRequest)
		return
	}
	payload, err := io.ReadAll(gr)
	if err != nil {
		http.Error(w, "corrupt export file", http.StatusBadRequest)
		return
	}

	var bundle database.ExportBundle
	if err := json.Unmarshal(payload, &bundle); err != nil {
		http.Error(w, "corrupt export file", http.StatusBadRequest)
		return
	}

	if err := c.db.Import(r.Context(), &bundle); err != nil {
		log.Printf("[API] Import: %v", err)
		http.Error(w, "import failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	summary := map[string]any{
		"sessions":     len(bundle.Sessions),
		"participants": len(bundle.Participants),
		"laps":         len(bundle.Laps),
		"lap_times":    len(bundle.LapTimes),
		"telemetry":    len(bundle.Telemetry),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(summary)
}
