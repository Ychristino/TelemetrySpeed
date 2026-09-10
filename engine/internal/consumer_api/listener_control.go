package consumer

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// ListenerControl lets the API start/stop the UDP telemetry listener on
// demand, independent of the DB/API which run for the life of the process —
// wired by cmd/engine so the Godot client's Play/Stop control drives just
// the UDP socket. nil in the standalone cmd/consumer_api binary, which has
// no on-demand listener to control (its listener is a separate process).
type ListenerControl interface {
	// Start binds the UDP listener on port. Returns an error (e.g. "already
	// running", "port in use") without affecting the DB/API.
	Start(port int) error
	// Stop tears down the running listener, if any. wasRunning reports
	// whether one was actually running.
	Stop() (wasRunning bool)
	// Status reports whether a listener is currently running and on which
	// port (port is meaningless when running is false).
	Status() (running bool, port int)
}

type listenerStatusJSON struct {
	Running bool `json:"running"`
	Port    int  `json:"port,omitempty"`
}

// POST /listener/start?port=20777
func handleListenerStart(lc ListenerControl) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		portStr := r.URL.Query().Get("port")
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			http.Error(w, "port must be an integer between 1 and 65535", http.StatusBadRequest)
			return
		}
		if err := lc.Start(port); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listenerStatusJSON{Running: true, Port: port})
	}
}

// POST /listener/stop
func handleListenerStop(lc ListenerControl) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		lc.Stop()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listenerStatusJSON{Running: false})
	}
}

// GET /listener/status
func handleListenerStatus(lc ListenerControl) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		running, port := lc.Status()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listenerStatusJSON{Running: running, Port: port})
	}
}
