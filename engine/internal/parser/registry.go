package parser

import "NewTelemetryEngine/internal/f1"

var registered = map[uint16]GameParser{}

// Register maps one or more packetFormat values to a parser.
// Called from each game-specific file's init().
func Register(p GameParser, formats ...uint16) {
	for _, f := range formats {
		registered[f] = p
	}
}

// Dispatch selects the parser for hdr.PacketFormat and calls Parse.
// Returns (nil, false) if no parser is registered for the format.
func Dispatch(data []byte, hdr f1.PacketHeader) (f1.Payload, bool) {
	p, ok := registered[hdr.PacketFormat]
	if !ok {
		return nil, false
	}
	return p.Parse(data, hdr)
}
