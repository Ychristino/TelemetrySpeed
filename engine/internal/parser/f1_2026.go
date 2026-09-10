package parser

import "NewTelemetryEngine/internal/f1"

const (
	numCars26          = 24
	motionSize26       = 54
	telemetrySize26    = 59
	statusSize26       = 59
	participantSize26  = 60
	damageSize26       = 46  // damageSize25 + tyreBlisters[4] = 42+4
	motionExDataSize26 = 244 // 29+244 = 273-byte total packet; adds wheelCamber[4]+wheelCamberGain[4] vs F1 25
)

type f1_2026Parser struct{}

func init() {
	Register(f1_2026Parser{}, f1.Format2026)
}

func (p f1_2026Parser) Parse(data []byte, hdr f1.PacketHeader) (f1.Payload, bool) {
	switch hdr.PacketID {
	case f1.PacketIDMotion:
		return parseMotion26(data)
	case f1.PacketIDLapData:
		return parseLapData(data, numCars26)
	case f1.PacketIDCarTelemetry:
		return parseCarTelemetry26(data)
	case f1.PacketIDCarStatus:
		return parseCarStatus26(data)
	case f1.PacketIDCarDamage:
		return parseCarDamage26(data)
	case f1.PacketIDMotionEx:
		return parseMotionEx26(data)
	case f1.PacketIDEvent:
		return parseEvent(data)
	case f1.PacketIDSession:
		return parseSession(data)
	case f1.PacketIDParticipants:
		return parseParticipants26(data, hdr.PlayerCarIndex)
	default:
		return nil, false
	}
}

func parseMotion26(data []byte) (f1.MotionPayload, bool) {
	if len(data) < f1.HeaderSize+numCars26*motionSize26 {
		return f1.MotionPayload{}, false
	}
	var out f1.MotionPayload
	for i := 0; i < numCars26; i++ {
		base := f1.HeaderSize + i*motionSize26
		c := newCursor(data[base : base+motionSize26])
		out[i] = f1.CarMotionData{
			PosX: c.F32(), PosY: c.F32(), PosZ: c.F32(),
			VelX: c.F32(), VelY: c.F32(), VelZ: c.F32(),
		}
		c.Skip(12) // [24:36] direction vectors — unused
		// F1 26 packs g-forces as int16/1000 instead of float32.
		out[i].GLateral = float32(c.I16()) / 1000.0
		out[i].GLong = float32(c.I16()) / 1000.0
		out[i].GVert = float32(c.I16()) / 1000.0
		out[i].Yaw = c.F32()
		out[i].Pitch = c.F32()
		out[i].Roll = c.F32()
	}
	return out, true
}

func parseCarTelemetry26(data []byte) (f1.TelemetryPayload, bool) {
	if len(data) < f1.HeaderSize+numCars26*telemetrySize26 {
		return f1.TelemetryPayload{}, false
	}
	var out f1.TelemetryPayload
	for i := 0; i < numCars26; i++ {
		base := f1.HeaderSize + i*telemetrySize26
		c := newCursor(data[base : base+telemetrySize26])
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
		engTemp := uint16(c.U8()) // F1 26 narrows engTemp to a single byte
		tyrePressure := c.F32x4()
		out[i] = f1.CarTelemetryData{
			Speed: speed, Throttle: throttle, Steer: steer, Brake: brake,
			Clutch: clutch, Gear: gear, RPM: rpm, DRS: drs,
			BrakesTemp: brakesTemp, TyreSurfTemp: tyreSurfTemp, TyreInnerTemp: tyreInnerTemp,
			EngTemp: engTemp, TyrePressure: tyrePressure,
		}
	}
	return out, true
}

func parseCarStatus26(data []byte) (f1.StatusPayload, bool) {
	if len(data) < f1.HeaderSize+numCars26*statusSize26 {
		return f1.StatusPayload{}, false
	}
	var out f1.StatusPayload
	for i := 0; i < numCars26; i++ {
		base := f1.HeaderSize + i*statusSize26
		c := newCursor(data[base : base+statusSize26])
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
		out[i] = f1.CarStatusData{
			TractionControl: tractionControl, AntiLockBrakes: antiLockBrakes, FuelMix: fuelMix,
			PitLimiter: pitLimiter, FuelInTank: fuelInTank, FuelRemLaps: fuelRemLaps,
			TyreCompound: tyreCompound, TyreAgeLaps: tyreAgeLaps,
			EnginePowerICE: enginePowerICE, EnginePowerMGUK: enginePowerMGUK,
			ERSStore: ersStore, ERSDeployMode: ersDeployMode,
		}
	}
	return out, true
}

func parseCarDamage26(data []byte) (f1.DamagePayload, bool) {
	if len(data) < f1.HeaderSize+numCars26*damageSize26 {
		return f1.DamagePayload{}, false
	}
	var out f1.DamagePayload
	for i := 0; i < numCars26; i++ {
		base := f1.HeaderSize + i*damageSize26
		c := newCursor(data[base : base+damageSize26])
		tyresWear := c.F32x4()
		c.Skip(4) // [16:20] tyresDamage — unused
		out[i] = f1.CarDamageData{
			TyresWear:    tyresWear,
			BrakesDamage: c.U8x4(),
			TyreBlisters: c.U8x4(),
		}
	}
	return out, true
}

func parseMotionEx26(data []byte) (f1.MotionExPayload, bool) {
	if len(data) < f1.HeaderSize+motionExDataSize26 {
		return f1.MotionExPayload{}, false
	}
	c := newCursor(data[f1.HeaderSize : f1.HeaderSize+motionExDataSize26])
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
	c.Skip(16) // [196:212] roll angles + chassis yaw/pitch — unused
	out.WheelCamber = c.F32x4()
	return out, true
}

func parseParticipants26(data []byte, _ uint8) (f1.ParticipantsPayload, bool) {
	if len(data) < f1.HeaderSize+1+numCars26*participantSize26 {
		return f1.ParticipantsPayload{}, false
	}
	var out f1.ParticipantsPayload
	for i := 0; i < numCars26; i++ {
		base := f1.HeaderSize + 1 + i*participantSize26
		c := newCursor(data[base : base+participantSize26])
		c.Skip(1) // [0] aiControlled — unused
		driverID := uint8(c.U16()) // F1 26 widens driverId to uint16
		c.Skip(2) // [3:5] unused
		teamID := uint8(c.U16()) // F1 26 widens teamId to uint16
		c.Skip(1) // [7] unused
		raceNumber := c.U8()
		c.Skip(1) // [9] unused
		name := c.NullTermStr(32)
		out[i] = f1.ParticipantInfo{
			Name:       name,
			TeamID:     teamID,
			DriverID:   driverID,
			RaceNumber: raceNumber,
		}
	}
	return out, true
}
