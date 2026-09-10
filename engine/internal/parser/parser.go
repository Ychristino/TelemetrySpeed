package parser

import "NewTelemetryEngine/internal/f1"

// GameParser translates raw UDP bytes into typed f1.Payload values
// for a specific game format.
type GameParser interface {
	Parse(data []byte, hdr f1.PacketHeader) (f1.Payload, bool)
}
