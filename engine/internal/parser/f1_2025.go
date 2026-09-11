package parser

import "NewTelemetryEngine/internal/f1"

const (
	numCars25          = 22
	motionSize25       = 60
	telemetrySize25    = 60
	statusSize25       = 55
	participantSize25  = 57
	damageSize25       = 42  // 16(wear)+4(damage)+4(brakes)+18(single-byte fields)
	motionExDataSize25 = 212 // 29+212 = 241-byte total packet
)

type f1_2025Parser struct{}

func init() {
	Register(f1_2025Parser{}, f1.Format2023, f1.Format2024, f1.Format2025)
}

func (p f1_2025Parser) Parse(data []byte, hdr f1.PacketHeader) (f1.Payload, bool) {
	switch hdr.PacketID {
	case f1.PacketIDMotion:
		return parseMotion25(data, hdr.PlayerCarIndex)
	case f1.PacketIDLapData:
		return parseLapData(data, numCars25, hdr.PlayerCarIndex)
	case f1.PacketIDCarTelemetry:
		return parseCarTelemetry25(data, hdr.PlayerCarIndex)
	case f1.PacketIDCarStatus:
		return parseCarStatus25(data, hdr.PlayerCarIndex)
	case f1.PacketIDCarDamage:
		return parseCarDamage25(data, hdr.PlayerCarIndex)
	case f1.PacketIDMotionEx:
		return parseMotionEx25(data)
	case f1.PacketIDEvent:
		return parseEvent(data)
	case f1.PacketIDSession:
		return parseSession(data)
	case f1.PacketIDParticipants:
		return parseParticipants25(data, hdr.PlayerCarIndex)
	default:
		return nil, false
	}
}

func parseMotion25(data []byte, playerIdx uint8) (f1.MotionPayload, bool) {
	if int(playerIdx) >= numCars25 || len(data) < f1.HeaderSize+numCars25*motionSize25 {
		return f1.MotionPayload{}, false
	}
	base := f1.HeaderSize + int(playerIdx)*motionSize25
	c := newCursor(data[base : base+motionSize25])
	out := f1.MotionPayload{
		PosX: c.F32(), PosY: c.F32(), PosZ: c.F32(),
		VelX: c.F32(), VelY: c.F32(), VelZ: c.F32(),
	}
	c.Skip(12) // [24:36] direction vectors — unused
	out.GLateral = c.F32()
	out.GLong = c.F32()
	out.GVert = c.F32()
	out.Yaw = c.F32()
	out.Pitch = c.F32()
	out.Roll = c.F32()
	return out, true
}

func parseCarTelemetry25(data []byte, playerIdx uint8) (f1.TelemetryPayload, bool) {
	if int(playerIdx) >= numCars25 || len(data) < f1.HeaderSize+numCars25*telemetrySize25 {
		return f1.TelemetryPayload{}, false
	}
	base := f1.HeaderSize + int(playerIdx)*telemetrySize25
	c := newCursor(data[base : base+telemetrySize25])
	speed := c.U16()
	throttle := c.F32()
	steer := c.F32()
	brake := c.F32()
	clutch := c.U8()
	gear := c.I8()
	rpm := c.U16()
	drs := c.U8()
	c.Skip(3) // [19:22] unused
	brakesTemp := c.U16x4()
	tyreSurfTemp := c.U8x4()
	tyreInnerTemp := c.U8x4()
	engTemp := c.U16()
	tyrePressure := c.F32x4()
	return f1.TelemetryPayload{
		Speed: speed, Throttle: throttle, Steer: steer, Brake: brake,
		Clutch: clutch, Gear: gear, RPM: rpm, DRS: drs,
		BrakesTemp: brakesTemp, TyreSurfTemp: tyreSurfTemp, TyreInnerTemp: tyreInnerTemp,
		EngTemp: engTemp, TyrePressure: tyrePressure,
	}, true
}

func parseCarStatus25(data []byte, playerIdx uint8) (f1.StatusPayload, bool) {
	if int(playerIdx) >= numCars25 || len(data) < f1.HeaderSize+numCars25*statusSize25 {
		return f1.StatusPayload{}, false
	}
	base := f1.HeaderSize + int(playerIdx)*statusSize25
	c := newCursor(data[base : base+statusSize25])
	tractionControl := c.U8()
	antiLockBrakes := c.U8()
	fuelMix := c.U8()
	c.Skip(1) // [3] unused
	pitLimiter := c.U8()
	fuelInTank := c.F32()
	c.Skip(4) // [9:13] unused
	fuelRemLaps := c.F32()
	c.Skip(8) // [17:25] unused
	tyreCompound := c.U8()
	c.Skip(1) // [26] unused
	tyreAgeLaps := c.U8()
	c.Skip(1) // [28] unused
	enginePowerICE := c.F32()
	enginePowerMGUK := c.F32()
	ersStore := c.F32()
	ersDeployMode := c.U8()
	return f1.StatusPayload{
		TractionControl: tractionControl, AntiLockBrakes: antiLockBrakes, FuelMix: fuelMix,
		PitLimiter: pitLimiter, FuelInTank: fuelInTank, FuelRemLaps: fuelRemLaps,
		TyreCompound: tyreCompound, TyreAgeLaps: tyreAgeLaps,
		EnginePowerICE: enginePowerICE, EnginePowerMGUK: enginePowerMGUK,
		ERSStore: ersStore, ERSDeployMode: ersDeployMode,
	}, true
}

func parseCarDamage25(data []byte, playerIdx uint8) (f1.DamagePayload, bool) {
	if int(playerIdx) >= numCars25 || len(data) < f1.HeaderSize+numCars25*damageSize25 {
		return f1.DamagePayload{}, false
	}
	base := f1.HeaderSize + int(playerIdx)*damageSize25
	c := newCursor(data[base : base+damageSize25])
	tyresWear := c.F32x4()
	c.Skip(4) // [16:20] tyresDamage — unused
	return f1.DamagePayload{
		TyresWear:    tyresWear,
		BrakesDamage: c.U8x4(),
	}, true
}

func parseMotionEx25(data []byte) (f1.MotionExPayload, bool) {
	if len(data) < f1.HeaderSize+motionExDataSize25 {
		return f1.MotionExPayload{}, false
	}
	c := newCursor(data[f1.HeaderSize : f1.HeaderSize+motionExDataSize25])
	out := f1.MotionExPayload{
		SuspensionPos:   c.F32x4(),
		SuspensionVel:   c.F32x4(),
		SuspensionAccel: c.F32x4(),
		WheelSpeed:      c.F32x4(),
		WheelSlipRatio:  c.F32x4(),
		WheelSlipAngle:  c.F32x4(),
		WheelLatForce:   c.F32x4(),
		WheelLongForce:  c.F32x4(),
	}
	c.Skip(4) // [128:132] heightOfCOG — unused
	out.LocalVelX = c.F32()
	out.LocalVelY = c.F32()
	out.LocalVelZ = c.F32()
	out.AngularVelX = c.F32()
	out.AngularVelY = c.F32()
	out.AngularVelZ = c.F32()
	c.Skip(12) // [156:168] angular acceleration — unused
	out.FrontWheelsAngle = c.F32()
	out.WheelVertForce = c.F32x4()
	out.FrontAeroHeight = c.F32()
	out.RearAeroHeight = c.F32()
	return out, true
}

func parseParticipants25(data []byte, playerIdx uint8) (f1.ParticipantsPayload, bool) {
	if int(playerIdx) >= numCars25 || len(data) < f1.HeaderSize+1+numCars25*participantSize25 {
		return f1.ParticipantsPayload{}, false
	}
	base := f1.HeaderSize + 1 + int(playerIdx)*participantSize25
	c := newCursor(data[base : base+participantSize25])
	c.Skip(1) // [0] aiControlled — unused
	driverID := c.U8()
	c.Skip(1) // [2] networkId — unused
	teamID := c.U8()
	c.Skip(1) // [4] myTeam — unused
	raceNumber := c.U8()
	c.Skip(1) // [6] nationality — unused
	name := c.NullTermStr(32)
	return f1.ParticipantsPayload{
		Name:       name,
		TeamID:     teamID,
		DriverID:   driverID,
		RaceNumber: raceNumber,
	}, true
}
