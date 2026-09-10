package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	ListenerPort      string
	APIPort           string
	UDPQueueSize      int
	MaxWorkers        int
	UDPIncomeDeadline int

	// DBWriteFlushIntervalMS / DBWriteBatchSize: the DB writer flushes
	// buffered rows on whichever comes first. Lower interval = fresher data
	// (matters for LiveWindowS below) at the cost of more, smaller inserts.
	DBWriteFlushIntervalMS int
	DBWriteBatchSize       int

	// StreamPollIntervalMS: how often a live WebSocket stream checks the DB
	// for new rows and sends a frame. Every row written since the last poll
	// goes out as one message, so raising this trades live latency for
	// fewer/larger network messages.
	StreamPollIntervalMS int
	// StreamIdleTimeoutS: how long a stream waits with no new rows before
	// assuming the driver's lap/session ended and closing the connection.
	StreamIdleTimeoutS int

	// LiveWindowS: how recent a driver's last telemetry row must be to show
	// up as "currently live" in the driver list.
	LiveWindowS int
}

func (c *Config) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName)
}

func Load() *Config {
	udpQueueSize, err := strconv.Atoi(getEnv("UDP_QUEUE_SIZE"))
	if err != nil {
		log.Fatalf("failed to read UDP_QUEUE_SIZE: %v", err)
	}
	maxWorkers, err := strconv.Atoi(getEnv("MAX_WORKERS"))
	if err != nil {
		log.Fatalf("failed to read MAX_WORKERS: %v", err)
	}
	udpIncomeDeadline, err := strconv.Atoi(getEnv("UDP_INCOME_DEADLINE"))
	if err != nil {
		log.Fatalf("failed to read UDP_INCOME_DEADLINE: %v", err)
	}

	cfg := &Config{
		DBHost:            getEnv("DB_HOST"),
		DBPort:            getEnv("DB_PORT"),
		DBUser:            getEnv("DB_USER"),
		DBPassword:        getEnv("DB_PASSWORD"),
		DBName:            getEnv("DB_NAME"),
		ListenerPort:      getEnv("LISTENER_PORT"),
		APIPort:           getEnv("API_PORT"),
		UDPQueueSize:      udpQueueSize,
		MaxWorkers:        maxWorkers,
		UDPIncomeDeadline: udpIncomeDeadline,

		DBWriteFlushIntervalMS: getEnvIntOr("DB_WRITE_FLUSH_INTERVAL_MS", 100),
		DBWriteBatchSize:       getEnvIntOr("DB_WRITE_BATCH_SIZE", 500),
		StreamPollIntervalMS:   getEnvIntOr("STREAM_POLL_INTERVAL_MS", 500),
		StreamIdleTimeoutS:     getEnvIntOr("STREAM_IDLE_TIMEOUT_S", 30),
		LiveWindowS:            getEnvIntOr("LIVE_WINDOW_S", 3),
	}

	if cfg.DBHost == "" {
		log.Fatal("DB_HOST is required")
	}
	return cfg
}

func getEnv(key string) string {
	v, _ := os.LookupEnv(key)
	return v
}

// getEnvIntOr reads an optional tuning knob, falling back to def if unset or
// unparseable — these have sane defaults so an outdated .env shouldn't
// prevent startup the way a missing required var does.
func getEnvIntOr(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("invalid %s=%q, using default %d", key, v, def)
		return def
	}
	return n
}
