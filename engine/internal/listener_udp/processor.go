package listener_udp

import (
	"NewTelemetryEngine/internal/f1"
	"NewTelemetryEngine/internal/parser"
	"NewTelemetryEngine/internal/source"
)

// parserWorker reads raw UDP packets, parses the header and payload, and
// forwards parsed packets to routerChan. It stops when rawChan is closed.
func parserWorker(rawChan <-chan rawPacket, routerChan chan<- *source.ParsedPacket) {
	for raw := range rawChan {
		pkt := parse(raw)
		if pkt == nil {
			continue
		}
		sendToRouter(pkt, routerChan)
	}
}

func parse(raw rawPacket) *source.ParsedPacket {
	hdr, ok := f1.ParseHeader(raw.data)
	if !ok {
		return nil
	}
	payload, ok := parser.Dispatch(raw.data, hdr)
	if !ok {
		return nil
	}
	return &source.ParsedPacket{
		Addr:    raw.addr,
		Header:  hdr,
		Payload: payload,
	}
}

// sendToRouter forwards a parsed packet to the router channel.
// Session events block (they are rare and must not be dropped).
// Frame packets use a non-blocking send — the game resends equivalent data
// next frame, so a drop here is harmless.
func sendToRouter(pkt *source.ParsedPacket, routerChan chan<- *source.ParsedPacket) {
	switch pkt.Payload.(type) {
	case f1.EventPayload:
		routerChan <- pkt
	default:
		select {
		case routerChan <- pkt:
		default:
		}
	}
}
