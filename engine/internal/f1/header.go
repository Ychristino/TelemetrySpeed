package f1

import "encoding/binary"

const (
	HeaderSize = 29

	PacketIDMotion       = 0
	PacketIDSession      = 1
	PacketIDLapData      = 2
	PacketIDEvent        = 3
	PacketIDParticipants = 4
	PacketIDCarTelemetry = 6
	PacketIDCarStatus    = 7
	PacketIDCarDamage    = 10
	PacketIDMotionEx     = 13

	Format2023 = 2023
	Format2024 = 2024
	Format2025 = 2025
	Format2026 = 2026

	ResultStatusInvalid  = 0
	ResultStatusInactive = 1
)

// PacketHeader is the common 29-byte header present in every F1 UDP packet.
// Layout (little-endian, packed, no padding):
//
//	[0:2]  packetFormat   uint16
//	[2]    gameYear       uint8
//	[3]    gameMajor      uint8
//	[4]    gameMinor      uint8
//	[5]    packetVersion  uint8
//	[6]    packetId       uint8
//	[7:15] sessionUID     uint64
//	[15:19] sessionTime   float32
//	[19:23] frameId       uint32
//	[23:27] overallFrameId uint32
//	[27]   playerCarIndex uint8
//	[28]   secondary...   uint8
type PacketHeader struct {
	PacketFormat   uint16
	PacketID       uint8
	FrameID        uint32
	PlayerCarIndex uint8
}

// ParseHeader reads the 29-byte header from the start of data.
// Returns (header, true) on success, (zero, false) if data is too short.
func ParseHeader(data []byte) (PacketHeader, bool) {
	if len(data) < HeaderSize {
		return PacketHeader{}, false
	}
	return PacketHeader{
		PacketFormat:   binary.LittleEndian.Uint16(data[0:2]),
		PacketID:       data[6],
		FrameID:        binary.LittleEndian.Uint32(data[19:23]),
		PlayerCarIndex: data[27],
	}, true
}

// GameName converts packetFormat to a short game identifier string.
func GameName(format uint16) string {
	switch format {
	case Format2026:
		return "f1_2026"
	case Format2025:
		return "f1_2025"
	case Format2024:
		return "f1_2024"
	case Format2023:
		return "f1_2023"
	default:
		return "unknown"
	}
}
