package source

import (
	"net"

	"NewTelemetryEngine/internal/f1"
)

// ParsedPacket is produced by parser workers and consumed by the Source Router.
type ParsedPacket struct {
	Addr    *net.UDPAddr
	Header  f1.PacketHeader
	Payload f1.Payload
}
