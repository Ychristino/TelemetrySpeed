// Package consumer implements the HTTP + WebSocket API that serves stored
// telemetry to clients (the Godot app). Handlers are grouped by resource:
// sessions.go, laps.go, telemetry.go, stream.go.
package consumer

import (
	"context"
	"log"
	"net/http"
	"time"

	"NewTelemetryEngine/internal/config"
	"NewTelemetryEngine/internal/database"

	"github.com/gorilla/websocket"
)

type Consumer struct {
	cfg      *config.Config
	reader   *database.Reader
	db       *database.DB
	server   *http.Server
	upgrader websocket.Upgrader

	// streamPollInterval controls how often handleWebSocket checks the DB
	// for new rows and sends a frame. Each frame already batches every row
	// written since the last poll (not one message per row), so raising
	// this trades a bit of live latency for meaningfully fewer, larger
	// WebSocket messages and DB polls — worthwhile since the client only
	// renders a scrolling chart, where a few hundred ms of extra buffering
	// isn't perceptible. Independent of the DB writer's flush cadence (see
	// writer.go), which stays fast because it feeds the live-detection
	// window (see reader.go's liveWindow).
	streamPollInterval time.Duration
	// streamIdleTimeout: how long to wait with zero new rows before
	// assuming the driver's session has ended and closing the stream.
	streamIdleTimeout time.Duration
}

// New builds the API server. shutdown, if non-nil, is invoked when a client
// POSTs /shutdown — cmd/engine wires this to its own graceful-teardown path
// (stopping the embedded Postgres included), since the bundled Godot client
// has no cross-platform way to send a real process signal to a child it
// launched. lc, if non-nil, exposes /listener/{start,stop,status} for the
// on-demand UDP listener control cmd/engine wires up. The standalone
// cmd/consumer_api binary passes nil for both and relies on OS signals only,
// as before.
func New(cfg *config.Config, db *database.DB, shutdown func(), lc ListenerControl) *Consumer {
	c := &Consumer{
		cfg:    cfg,
		reader: db.NewReader(),
		db:     db,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		streamPollInterval: time.Duration(cfg.StreamPollIntervalMS) * time.Millisecond,
		streamIdleTimeout:  time.Duration(cfg.StreamIdleTimeoutS) * time.Second,
	}

	port := cfg.APIPort
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	if shutdown != nil {
		mux.HandleFunc("POST /shutdown", handleShutdown(shutdown))
	}
	if lc != nil {
		mux.HandleFunc("POST /listener/start", handleListenerStart(lc))
		mux.HandleFunc("POST /listener/stop", handleListenerStop(lc))
		mux.HandleFunc("GET /listener/status", handleListenerStatus(lc))
	}
	mux.HandleFunc("GET /sessions", c.handleListSessions)
	mux.HandleFunc("GET /sessions/{id}", c.handleGetSession)
	mux.HandleFunc("GET /sessions/{id}/participants", c.handleListParticipants)
	mux.HandleFunc("GET /laptimes", c.handleListLapTimes)
	mux.HandleFunc("GET /idealap", c.handleGetIdealLap)
	mux.HandleFunc("GET /live", c.handleListLive)
	mux.HandleFunc("GET /telemetry", c.handleGetTelemetry)
	mux.HandleFunc("GET /telemetry/ideal", c.handleGetIdealLapTelemetry)
	mux.HandleFunc("GET /ws/stream", c.handleWebSocket)
	mux.HandleFunc("GET /export", c.handleExport)
	mux.HandleFunc("POST /import", c.handleImport)

	c.server = &http.Server{
		Addr:        ":" + port,
		Handler:     mux,
		ReadTimeout: 30 * time.Second,
	}
	return c
}

func (c *Consumer) Start() {
	log.Printf("[API] listening on %s", c.server.Addr)
	if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("[API] server error: %v", err)
	}
}

func (c *Consumer) Shutdown(ctx context.Context) error {
	return c.server.Shutdown(ctx)
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleShutdown responds before calling trigger (in a goroutine) since
// trigger's teardown shuts down this very HTTP server — running it inline
// would deadlock waiting on the response we haven't sent yet.
func handleShutdown(trigger func()) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		go trigger()
	}
}
