package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"NewTelemetryEngine/internal/config"
	consumer "NewTelemetryEngine/internal/consumer_api"
	"NewTelemetryEngine/internal/database"

	"github.com/joho/godotenv"
)

func main() {
	log.Println("[API] starting")

	if err := godotenv.Load(); err != nil {
		log.Println("[API] no .env file")
	}

	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.New(ctx, cfg)
	if err != nil {
		log.Fatalf("[API] database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("[API] migrate: %v", err)
	}

	api := consumer.New(cfg, db, nil, nil)

	go api.Start()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[API] shutting down")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := api.Shutdown(shutCtx); err != nil {
		log.Printf("[API] shutdown error: %v", err)
	}
	log.Println("[API] stopped")
}
