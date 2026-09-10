package source

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"NewTelemetryEngine/internal/database"
)

const (
	staleTimeout   = 10 * time.Second
	cleanupInterval = 5 * time.Second
)

// SourceKey uniquely identifies a UDP sender by IP and port.
type SourceKey struct {
	IP   string
	Port int
}

func (k SourceKey) String() string {
	return fmt.Sprintf("%s:%d", k.IP, k.Port)
}

func keyFromAddr(addr *net.UDPAddr) SourceKey {
	return SourceKey{IP: addr.IP.String(), Port: addr.Port}
}

// SourceRouter reads ParsedPackets from inputCh, creates or reuses a
// SourceHandler per unique (IP, port), and dispatches packets to each handler.
// It also cleans up stale handlers after staleTimeout of inactivity.
//
// Run blocks until ctx is cancelled, then signals all handlers to stop and
// waits for them before closing writeCh.
type SourceRouter struct {
	writeCh chan<- []database.TelemetryRow
	sw      *database.SessionWriter

	mu      sync.Mutex
	sources map[SourceKey]*SourceHandler
	wg      sync.WaitGroup
}

func NewSourceRouter(writeCh chan<- []database.TelemetryRow, sw *database.SessionWriter) *SourceRouter {
	return &SourceRouter{
		writeCh: writeCh,
		sw:      sw,
		sources: make(map[SourceKey]*SourceHandler),
	}
}

// Run is the Source Router goroutine. It blocks until inputCh is closed or
// ctx is cancelled, then gracefully shuts down all handlers.
func (r *SourceRouter) Run(ctx context.Context, inputCh <-chan *ParsedPacket) {
	cleanup := time.NewTicker(cleanupInterval)
	defer cleanup.Stop()

	for {
		select {
		case pkt, ok := <-inputCh:
			if !ok {
				r.shutdown()
				return
			}
			r.dispatch(ctx, pkt)
		case <-cleanup.C:
			r.cleanupStale(ctx)
		case <-ctx.Done():
			r.shutdown()
			return
		}
	}
}

func (r *SourceRouter) dispatch(ctx context.Context, pkt *ParsedPacket) {
	key := keyFromAddr(pkt.Addr)

	r.mu.Lock()
	h, exists := r.sources[key]
	if !exists {
		h = newSourceHandler(key, r.writeCh, r.sw)
		r.sources[key] = h
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			h.run(ctx)
		}()
		log.Printf("[Router] new source: %s", key)
	}
	r.mu.Unlock()

	select {
	case h.inChan <- pkt:
	default:
		log.Printf("[Router] %s inChan full, dropping packet", key)
	}
}

func (r *SourceRouter) cleanupStale(ctx context.Context) {
	cutoff := time.Now().Add(-staleTimeout)

	r.mu.Lock()
	var stale []SourceKey
	for key, h := range r.sources {
		if h.lastSeen.Before(cutoff) {
			stale = append(stale, key)
		}
	}
	for _, key := range stale {
		h := r.sources[key]
		delete(r.sources, key)
		close(h.inChan)
		log.Printf("[Router] cleaned up stale source: %s", key)
	}
	r.mu.Unlock()
}

func (r *SourceRouter) shutdown() {
	r.mu.Lock()
	for _, h := range r.sources {
		close(h.inChan)
	}
	r.mu.Unlock()

	r.wg.Wait()
}
