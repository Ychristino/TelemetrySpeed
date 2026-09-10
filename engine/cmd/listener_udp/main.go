package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"NewTelemetryEngine/internal/config"
	"NewTelemetryEngine/internal/database"
	"NewTelemetryEngine/internal/listener_udp"
	"NewTelemetryEngine/internal/source"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, reading from environment")
	}

	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.New(ctx, cfg)
	if err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("migration failed: %v", err)
	}
	log.Println("[main] migrations applied")

	tw := db.NewTelemetryWriter()
	tw.Start(ctx)

	router := source.NewSourceRouter(tw.WriteCh(), db.NewSessionWriter())
	listener, err := listener_udp.New(cfg, router)
	if err != nil {
		log.Fatalf("listener init failed: %v", err)
	}

	log.Println("[main] listener starting")
	listener.Start(ctx)
	log.Println("[main] shutdown complete")
}
