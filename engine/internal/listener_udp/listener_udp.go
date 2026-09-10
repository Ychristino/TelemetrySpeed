package listener_udp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"NewTelemetryEngine/internal/config"
	"NewTelemetryEngine/internal/source"
)

// rawPacket bundles a received UDP payload with its sender address.
type rawPacket struct {
	addr *net.UDPAddr
	data []byte
}

// Listener binds the UDP socket, receives raw packets, and dispatches
// them to parser workers via rawChan.
type Listener struct {
	cfg        *config.Config
	conn       *net.UDPConn
	router     *source.SourceRouter
	rawChan    chan rawPacket
	routerChan chan *source.ParsedPacket
	wg         sync.WaitGroup
}

// bufPool eliminates the per-receive allocation for the read buffer.
var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 2048)
		return &b
	},
}

// New binds the UDP socket for cfg.ListenerPort and returns a Listener ready
// to Start, or an error if the port couldn't be bound (e.g. already in use)
// — callers that can recover from a bad port (see cmd/engine's on-demand
// listener control) must check this instead of the process dying outright.
func New(cfg *config.Config, router *source.SourceRouter) (*Listener, error) {
	addr, err := net.ResolveUDPAddr("udp", ":"+cfg.ListenerPort)
	if err != nil {
		return nil, fmt.Errorf("resolve UDP addr: %w", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("bind UDP port %s: %w", cfg.ListenerPort, err)
	}
	fmt.Printf("[Listener] listening on UDP port %s\n", cfg.ListenerPort)

	return &Listener{
		cfg:        cfg,
		conn:       conn,
		router:     router,
		rawChan:    make(chan rawPacket, cfg.UDPQueueSize),
		routerChan: make(chan *source.ParsedPacket, 512),
	}, nil
}

// Start launches the parser workers and UDP receive loop, then blocks until
// ctx is cancelled.
func (l *Listener) Start(ctx context.Context) {
	for range l.cfg.MaxWorkers {
		l.wg.Add(1)
		go func() {
			defer l.wg.Done()
			parserWorker(l.rawChan, l.routerChan)
		}()
	}

	routerDone := make(chan struct{})
	go func() {
		defer close(routerDone)
		l.router.Run(ctx, l.routerChan)
	}()

	l.receive(ctx)

	// rawChan close signals parser workers to stop.
	close(l.rawChan)
	l.wg.Wait()

	// routerChan close signals the Source Router to stop.
	close(l.routerChan)
	<-routerDone

	_ = l.conn.Close()
	log.Println("[Listener] shutdown complete")
}

func (l *Listener) receive(ctx context.Context) {
	deadline := time.Duration(l.cfg.UDPIncomeDeadline) * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		bufPtr := bufPool.Get().(*[]byte)
		buf := *bufPtr

		_ = l.conn.SetReadDeadline(time.Now().Add(deadline))
		n, addr, err := l.conn.ReadFromUDP(buf)
		if err != nil {
			bufPool.Put(bufPtr)
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			log.Printf("[Listener] read error: %v", err)
			continue
		}

		data := make([]byte, n)
		copy(data, buf[:n])
		bufPool.Put(bufPtr)

		l.enqueue(rawPacket{addr: addr, data: data})
	}
}

// enqueue puts a packet on rawChan. If the channel is full it drops the
// oldest entry to avoid blocking the receive loop.
func (l *Listener) enqueue(pkt rawPacket) {
	select {
	case l.rawChan <- pkt:
	default:
		select {
		case <-l.rawChan:
		default:
		}
		select {
		case l.rawChan <- pkt:
		default:
		}
	}
}
