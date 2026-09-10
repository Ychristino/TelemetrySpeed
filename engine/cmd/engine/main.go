// Command engine is the single process the bundled Godot client launches
// once when it opens and stops when it closes: it manages its own embedded
// PostgreSQL instance (no TimescaleDB extension — see
// internal/database/migrations/001_init_local.sql for why) and runs the
// HTTP/WS API continuously for the life of the process. The UDP telemetry
// listener is NOT started at startup — the Godot client's Play/Stop control
// starts and stops it on demand via POST /listener/start|stop, so the user
// can pick the UDP port at the point they press Play rather than when the
// app launches. Ports and a data directory are passed as flags rather than
// read from .env, since the Godot launcher controls this process directly
// and needs deterministic control over what it started.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"NewTelemetryEngine/internal/config"
	consumer "NewTelemetryEngine/internal/consumer_api"
	"NewTelemetryEngine/internal/database"
	"NewTelemetryEngine/internal/listener_udp"
	"NewTelemetryEngine/internal/source"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	apiPort := flag.Int("api-port", 8081, "HTTP/WS API port")
	pgPort := flag.Int("pg-port", 5439, "internal port for the embedded PostgreSQL instance")
	dataDir := flag.String("data-dir", "", "directory to store the embedded PostgreSQL binaries + data (required)")
	flag.Parse()

	if *dataDir == "" {
		log.Fatal("[engine] --data-dir is required")
	}

	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V16).
		Port(uint32(*pgPort)).
		Username("telemetry").
		Password("telemetry").
		Database("telemetry").
		BinariesPath(postgresBinariesPath(*dataDir)).
		DataPath(filepath.Join(*dataDir, "pgdata")).
		RuntimePath(filepath.Join(*dataDir, "pgruntime")).
		StartTimeout(60 * time.Second))

	log.Println("[engine] starting embedded PostgreSQL...")
	if err := pg.Start(); err != nil {
		log.Fatalf("[engine] postgres start: %v", err)
	}
	log.Println("[engine] postgres up")

	baseCfg := &config.Config{
		DBHost:     "127.0.0.1",
		DBPort:     fmt.Sprintf("%d", *pgPort),
		DBUser:     "telemetry",
		DBPassword: "telemetry",
		DBName:     "telemetry",

		APIPort: fmt.Sprintf("%d", *apiPort),

		UDPQueueSize:      4096,
		MaxWorkers:        4,
		UDPIncomeDeadline: 250,

		DBWriteFlushIntervalMS: 100,
		DBWriteBatchSize:       500,
		StreamPollIntervalMS:   500,
		StreamIdleTimeoutS:     30,
		LiveWindowS:            3,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.New(ctx, baseCfg)
	if err != nil {
		stopPG(pg)
		log.Fatalf("[engine] db connect: %v", err)
	}

	if err := db.MigrateLocal(ctx); err != nil {
		stopPG(pg)
		log.Fatalf("[engine] migrate: %v", err)
	}
	if err := db.EnsureCurrentPartitions(ctx); err != nil {
		stopPG(pg)
		log.Fatalf("[engine] ensure partitions: %v", err)
	}
	log.Println("[engine] schema ready")

	tw := db.NewTelemetryWriter()
	tw.Start(ctx)

	lm := &listenerManager{parentCtx: ctx, baseCfg: baseCfg, db: db, writeCh: tw.WriteCh()}

	var shutdownOnce sync.Once
	shutdownCh := make(chan struct{})
	triggerShutdown := func() { shutdownOnce.Do(func() { close(shutdownCh) }) }

	api := consumer.New(baseCfg, db, triggerShutdown, lm)
	go api.Start()

	log.Printf("[engine] ready: api=:%d (UDP listener starts on demand via /listener/start)", *apiPort)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		log.Println("[engine] signal received, shutting down")
	case <-shutdownCh:
		log.Println("[engine] shutdown requested, shutting down")
	}

	lm.Stop()
	cancel()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := api.Shutdown(shutCtx); err != nil {
		log.Printf("[engine] api shutdown: %v", err)
	}
	shutCancel()

	db.Close()
	stopPG(pg)

	log.Println("[engine] stopped")
}

func stopPG(pg *embeddedpostgres.EmbeddedPostgres) {
	if err := pg.Stop(); err != nil {
		log.Printf("[engine] postgres stop: %v", err)
	}
}

// postgresBinariesPath picks where embedded-postgres should find its
// Postgres binaries. embedded-postgres only ever reads from this path
// (see its Start(): it os.Stats bin/pg_ctl there and, if present, runs it
// directly — no copying or extraction happens) — so pointing it at a
// bundled, already-extracted copy shipped next to this executable
// (assets/pgbin-windows-amd64 at build time, see scripts/build-dist)
// makes first launch instant and fully offline: no download from Maven
// Central, no internet required at all.
//
// Falls back to <dataDir>/pgbin — embedded-postgres's normal behavior of
// downloading the binaries there itself — when no bundled copy sits next
// to the executable, which keeps `go run ./cmd/engine` working unchanged
// during development.
func postgresBinariesPath(dataDir string) string {
	exePath, err := os.Executable()
	if err == nil {
		bundled := filepath.Join(filepath.Dir(exePath), "pgbin")
		if _, err := os.Stat(filepath.Join(bundled, "bin", "pg_ctl.exe")); err == nil {
			return bundled
		}
	}
	return filepath.Join(dataDir, "pgbin")
}

// listenerManager implements consumer.ListenerControl, starting/stopping
// the UDP telemetry listener on demand against the engine's long-lived DB
// and TelemetryWriter — only the socket itself (and the SourceRouter feeding
// it) come and go with Start/Stop; everything downstream keeps running.
type listenerManager struct {
	parentCtx context.Context
	baseCfg   *config.Config
	db        *database.DB
	writeCh   chan<- []database.TelemetryRow

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	port   int
}

func (m *listenerManager) Start(port int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return fmt.Errorf("listener already running on port %d", m.port)
	}

	lcfg := *m.baseCfg
	lcfg.ListenerPort = fmt.Sprintf("%d", port)

	router := source.NewSourceRouter(m.writeCh, m.db.NewSessionWriter())
	l, err := listener_udp.New(&lcfg, router)
	if err != nil {
		return err
	}

	lctx, cancel := context.WithCancel(m.parentCtx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		l.Start(lctx)
	}()

	m.cancel = cancel
	m.done = done
	m.port = port
	log.Printf("[engine] UDP listener started on port %d", port)
	return nil
}

func (m *listenerManager) Stop() bool {
	m.mu.Lock()
	cancel := m.cancel
	done := m.done
	port := m.port
	wasRunning := cancel != nil
	m.cancel = nil
	m.done = nil
	m.port = 0
	m.mu.Unlock()

	if wasRunning {
		cancel()
		<-done
		log.Printf("[engine] UDP listener stopped (was port %d)", port)
	}
	return wasRunning
}

func (m *listenerManager) Status() (bool, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cancel != nil, m.port
}
