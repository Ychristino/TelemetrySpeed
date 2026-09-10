package consumer

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"NewTelemetryEngine/internal/database"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// GET /ws/stream
// Client sends one JSON message:
// {"game":"...","track":"...","lap":3,"driver":"...","fields":["speed",...]}
// lap <= 0 means "live-follow": stream the driver's telemetry across lap
// boundaries instead of being locked to one lap. fields is optional —
// omit it to get every field.
// Server replies with one JSON ack — {"fields":["speed",...]} — naming the
// resolved field list and order, then streams binary frames at
// streamPollInterval: [row_count uint32 LE, rows...], each row laid out per
// database.EncodeTelemetryFrame (fixed header + one value per acked field).
func (c *Consumer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[API] WebSocket upgrade: %v", err)
		return
	}
	defer conn.Close()

	var filter struct {
		Game      string   `json:"game"`
		Track     string   `json:"track"`
		Lap       int      `json:"lap"`
		Driver    string   `json:"driver"`
		SessionID string   `json:"session_id"`
		Fields    []string `json:"fields"`
	}
	if err := conn.ReadJSON(&filter); err != nil {
		log.Printf("[API] WebSocket read filter: %v", err)
		return
	}
	if filter.Game == "" || filter.Track == "" || filter.Driver == "" {
		conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"game, track, driver required"}`))
		return
	}

	var sessionID uuid.UUID
	if filter.SessionID != "" {
		var err error
		sessionID, err = uuid.Parse(filter.SessionID)
		if err != nil {
			conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"invalid session_id"}`))
			return
		}
	}

	var fields []database.FieldDef
	if len(filter.Fields) == 0 {
		fields = database.AllFields()
	} else {
		fields, err = database.ResolveFields(filter.Fields)
		if err != nil {
			conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"`+err.Error()+`"}`))
			return
		}
	}

	ack, err := json.Marshal(map[string]any{"fields": database.FieldNames(fields)})
	if err != nil {
		log.Printf("[API] WebSocket marshal ack: %v", err)
		return
	}
	if err := conn.WriteMessage(websocket.TextMessage, ack); err != nil {
		return
	}

	// Resolve session_id and car_index once, up front, instead of on every
	// poll tick — this is what lets the poll loop below skip the
	// participants/sessions joins entirely and hit the (session_id,
	// car_index, ts) index directly, so it stays cheap even at a fast
	// STREAM_POLL_INTERVAL_MS.
	if sessionID == uuid.Nil {
		sessionID, err = c.reader.ResolveSessionID(r.Context(), filter.Game, filter.Track)
		if err != nil {
			conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"no session found for game/track"}`))
			return
		}
	}
	carIndex, err := c.reader.ResolveCarIndex(r.Context(), sessionID, filter.Driver)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"driver not found in session"}`))
		return
	}

	hwm := time.Now().Add(-5 * time.Second) // slight lookback to catch in-flight rows
	ticker := time.NewTicker(c.streamPollInterval)
	defer ticker.Stop()

	maxIdleTicks := int(c.streamIdleTimeout / c.streamPollInterval)
	idle := 0
	var frameBuf bytes.Buffer
	for range ticker.C {
		rows, newHWM, err := c.reader.PollTelemetryPinned(r.Context(), sessionID, carIndex, filter.Lap, fields, hwm)
		if err != nil {
			log.Printf("[API] PollTelemetryPinned: %v", err)
			return
		}
		hwm = newHWM

		if len(rows) == 0 {
			idle++
			if idle >= maxIdleTicks { // streamIdleTimeout of no data → session ended
				return
			}
			continue
		}
		idle = 0

		frameBuf.Reset()
		database.EncodeTelemetryFrameInto(&frameBuf, rows, fields)
		if err := conn.WriteMessage(websocket.BinaryMessage, frameBuf.Bytes()); err != nil {
			return
		}
	}
}
