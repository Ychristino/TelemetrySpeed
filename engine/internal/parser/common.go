package parser

import "NewTelemetryEngine/internal/f1"

const lapDataSize = 57

// parseLapData is shared across all game versions — the LapData packet
// layout has not changed between F1 23 and F1 26. Only the player's slot is
// decoded; the rest of the grid isn't tracked.
func parseLapData(data []byte, numCars int, playerIdx uint8) (f1.LapPayload, bool) {
	if int(playerIdx) >= numCars {
		return f1.LapPayload{}, false
	}
	required := f1.HeaderSize + numCars*lapDataSize
	if len(data) < required {
		return f1.LapPayload{}, false
	}
	base := f1.HeaderSize + int(playerIdx)*lapDataSize
	c := newCursor(data[base : base+lapDataSize])
	c.Skip(4) // [0:4] lastLapTimeMS — unused
	currentLapTimeMS := c.U32()
	c.Skip(12) // [8:20] sector times — unused
	lapDistance := c.F32()
	c.Skip(8) // [24:32] totalDistance, safetyCarDelta — unused
	carPosition := c.U8()
	currentLapNum := c.U8()
	pitStatus := c.U8()
	c.Skip(1) // [35] unused
	sector := c.U8()
	lapInvalid := c.U8()
	penalties := c.U8()
	c.Skip(5) // [39:44] unused
	driverStatus := c.U8()
	resultStatus := c.U8()
	return f1.LapPayload{
		CurrentLapTimeMS: currentLapTimeMS,
		LapDistance:      lapDistance,
		CarPosition:      carPosition,
		CurrentLapNum:    currentLapNum,
		PitStatus:        pitStatus,
		Sector:           sector,
		LapInvalid:       lapInvalid,
		Penalties:        penalties,
		DriverStatus:     driverStatus,
		ResultStatus:     resultStatus,
	}, true
}

// parseEvent is shared — the Event packet format has not changed.
func parseEvent(data []byte) (f1.EventPayload, bool) {
	if len(data) < f1.HeaderSize+4 {
		return f1.EventPayload{}, false
	}
	c := newCursor(data[f1.HeaderSize:])
	return f1.EventPayload{Code: string(c.Bytes(4))}, true
}

// parseSession is shared — the Session packet layout has not changed.
func parseSession(data []byte) (f1.SessionPayload, bool) {
	if len(data) < f1.HeaderSize+8 {
		return f1.SessionPayload{}, false
	}
	c := newCursor(data[f1.HeaderSize:])
	c.Skip(6) // [0:6] weather, temps, etc. — unused
	return f1.SessionPayload{
		SessionType: c.U8(),
		TrackID:     c.I8(),
	}, true
}

func nullTermStr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
