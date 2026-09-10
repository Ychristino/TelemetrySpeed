package source

import (
	"time"

	"NewTelemetryEngine/internal/database"
	"NewTelemetryEngine/internal/f1"

	"github.com/google/uuid"
)

// CarFrame accumulates telemetry fields for a single car within one frame.
type CarFrame struct {
	// Motion (PacketID=0)
	PosX, PosY, PosZ       float32
	VelX, VelY, VelZ       float32
	GLateral, GLong, GVert float32
	Yaw, Pitch, Roll       float32

	// CarTelemetry (PacketID=6)
	Speed         uint16
	Throttle      float32
	Steer         float32
	Brake         float32
	Clutch        uint8
	Gear          int8
	RPM           uint16
	DRS           uint8
	BrakesTemp    [4]uint16
	TyreSurfTemp  [4]uint8
	TyreInnerTemp [4]uint8
	EngTemp       uint16
	TyrePressure  [4]float32

	// CarStatus (PacketID=7)
	TractionControl uint8
	AntiLockBrakes  uint8
	FuelMix         uint8
	PitLimiter      uint8
	FuelInTank      float32
	FuelRemLaps     float32
	TyreCompound    uint8
	TyreAgeLaps     uint8
	EnginePowerICE  float32
	EnginePowerMGUK float32
	ERSStore        float32
	ERSDeployMode   uint8

	// LapData (PacketID=2)
	CurrentLapTimeMS uint32
	LapDistance      float32
	CarPosition      uint8
	CurrentLapNum    uint8
	Sector           uint8
	PitStatus        uint8
	LapInvalid       uint8
	Penalties        uint8
	DriverStatus     uint8
	ResultStatus     uint8

	// CarDamage (PacketID=10)
	TyresWear    [4]float32
	BrakesDamage [4]uint8
	TyreBlisters [4]uint8

	// MotionEx (PacketID=13, player car only)
	HasMotionEx     bool
	SuspensionPos   [4]float32
	SuspensionVel   [4]float32
	SuspensionAccel [4]float32
	WheelSpeed      [4]float32
	WheelSlipRatio  [4]float32
	WheelSlipAngle  [4]float32
	WheelLatForce   [4]float32
	WheelLongForce  [4]float32
	WheelVertForce  [4]float32
	LocalVelX       float32
	LocalVelY       float32
	LocalVelZ       float32
	AngularVelX     float32
	AngularVelY     float32
	AngularVelZ     float32
	FrontWheelsAngle float32
	FrontAeroHeight float32
	RearAeroHeight  float32
	WheelCamber     [4]float32
}

// Frame holds the assembled per-car data for one frameIdentifier.
type Frame struct {
	SessionID uuid.UUID
	FrameID   uint32
	Timestamp time.Time
	Cars      [f1.MaxCars]CarFrame
}

// optF returns v as an any when hasData is true, or nil (SQL NULL) otherwise.
// Used for player-car-only MotionEx fields.
func optF(hasData bool, v float32) any {
	if !hasData {
		return nil
	}
	return v
}

// ToTelemetryRows converts a finalized Frame into database rows.
// Inactive/invalid cars are skipped. Columns must match telemetryCols in writer.go exactly.
func (fr *Frame) ToTelemetryRows() []database.TelemetryRow {
	rows := make([]database.TelemetryRow, 0, f1.MaxCars)
	for i, c := range fr.Cars {
		if c.ResultStatus <= f1.ResultStatusInactive {
			continue
		}
		rows = append(rows, database.TelemetryRow{
			SessionID: fr.SessionID,
			CarIndex:  int16(i),
			Timestamp: fr.Timestamp,

			Speed:    float32(c.Speed),
			Throttle: c.Throttle,
			Brake:    c.Brake,
			Steering: c.Steer,
			Gear:     int16(c.Gear),
			Clutch:   float32(c.Clutch),
			DRS:      int16(c.DRS),

			PosX:  c.PosX,
			PosY:  c.PosY,
			PosZ:  c.PosZ,
			VelX:  c.VelX,
			VelY:  c.VelY,
			VelZ:  c.VelZ,
			Yaw:   c.Yaw,
			Pitch: c.Pitch,
			Roll:  c.Roll,

			GLat:  c.GLateral,
			GLong: c.GLong,
			GVert: c.GVert,

			RPM:            int32(c.RPM),
			EngTemp:        float32(c.EngTemp),
			Fuel:           c.FuelInTank,
			FuelRemLaps:    c.FuelRemLaps,
			FuelMix:        int16(c.FuelMix),
			PitLimiter:     int16(c.PitLimiter),
			EnginePowerICE: c.EnginePowerICE,
			EnginePowerMGUK: c.EnginePowerMGUK,

			ERSStore:      c.ERSStore,
			ERSDeployMode: int16(c.ERSDeployMode),

			TractionControl: int16(c.TractionControl),
			ABS:             int16(c.AntiLockBrakes),

			TyreCompound: int16(c.TyreCompound),
			TyreAgeLaps:  int16(c.TyreAgeLaps),

			TyreSurfTempRL: float32(c.TyreSurfTemp[0]),
			TyreSurfTempRR: float32(c.TyreSurfTemp[1]),
			TyreSurfTempFL: float32(c.TyreSurfTemp[2]),
			TyreSurfTempFR: float32(c.TyreSurfTemp[3]),

			TyreInnerTempRL: float32(c.TyreInnerTemp[0]),
			TyreInnerTempRR: float32(c.TyreInnerTemp[1]),
			TyreInnerTempFL: float32(c.TyreInnerTemp[2]),
			TyreInnerTempFR: float32(c.TyreInnerTemp[3]),

			TyrePressureRL: c.TyrePressure[0],
			TyrePressureRR: c.TyrePressure[1],
			TyrePressureFL: c.TyrePressure[2],
			TyrePressureFR: c.TyrePressure[3],

			TyreWearRL: c.TyresWear[0],
			TyreWearRR: c.TyresWear[1],
			TyreWearFL: c.TyresWear[2],
			TyreWearFR: c.TyresWear[3],

			TyreBlisterRL: int16(c.TyreBlisters[0]),
			TyreBlisterRR: int16(c.TyreBlisters[1]),
			TyreBlisterFL: int16(c.TyreBlisters[2]),
			TyreBlisterFR: int16(c.TyreBlisters[3]),

			BrakeTempRL: float32(c.BrakesTemp[0]),
			BrakeTempRR: float32(c.BrakesTemp[1]),
			BrakeTempFL: float32(c.BrakesTemp[2]),
			BrakeTempFR: float32(c.BrakesTemp[3]),

			BrakeDamageRL: int16(c.BrakesDamage[0]),
			BrakeDamageRR: int16(c.BrakesDamage[1]),
			BrakeDamageFL: int16(c.BrakesDamage[2]),
			BrakeDamageFR: int16(c.BrakesDamage[3]),

			LapTimeMS:    int32(c.CurrentLapTimeMS),
			LapDistance:  c.LapDistance,
			CarPosition:  int16(c.CarPosition),
			LapNum:       int16(c.CurrentLapNum),
			Sector:       int16(c.Sector),
			PitStatus:    int16(c.PitStatus),
			DriverStatus: int16(c.DriverStatus),
			LapInvalid:   int16(c.LapInvalid),
			Penalties:    int16(c.Penalties),

			HasMotionEx: c.HasMotionEx,

			SuspPosRL: c.SuspensionPos[0],
			SuspPosRR: c.SuspensionPos[1],
			SuspPosFL: c.SuspensionPos[2],
			SuspPosFR: c.SuspensionPos[3],

			SuspVelRL: c.SuspensionVel[0],
			SuspVelRR: c.SuspensionVel[1],
			SuspVelFL: c.SuspensionVel[2],
			SuspVelFR: c.SuspensionVel[3],

			SuspAccelRL: c.SuspensionAccel[0],
			SuspAccelRR: c.SuspensionAccel[1],
			SuspAccelFL: c.SuspensionAccel[2],
			SuspAccelFR: c.SuspensionAccel[3],

			WheelSpeedRL: c.WheelSpeed[0],
			WheelSpeedRR: c.WheelSpeed[1],
			WheelSpeedFL: c.WheelSpeed[2],
			WheelSpeedFR: c.WheelSpeed[3],

			WheelSlipRatioRL: c.WheelSlipRatio[0],
			WheelSlipRatioRR: c.WheelSlipRatio[1],
			WheelSlipRatioFL: c.WheelSlipRatio[2],
			WheelSlipRatioFR: c.WheelSlipRatio[3],

			WheelSlipAngleRL: c.WheelSlipAngle[0],
			WheelSlipAngleRR: c.WheelSlipAngle[1],
			WheelSlipAngleFL: c.WheelSlipAngle[2],
			WheelSlipAngleFR: c.WheelSlipAngle[3],

			WheelLatForceRL: c.WheelLatForce[0],
			WheelLatForceRR: c.WheelLatForce[1],
			WheelLatForceFL: c.WheelLatForce[2],
			WheelLatForceFR: c.WheelLatForce[3],

			WheelLongForceRL: c.WheelLongForce[0],
			WheelLongForceRR: c.WheelLongForce[1],
			WheelLongForceFL: c.WheelLongForce[2],
			WheelLongForceFR: c.WheelLongForce[3],

			WheelVertForceRL: c.WheelVertForce[0],
			WheelVertForceRR: c.WheelVertForce[1],
			WheelVertForceFL: c.WheelVertForce[2],
			WheelVertForceFR: c.WheelVertForce[3],

			LocalVelX:        c.LocalVelX,
			LocalVelY:        c.LocalVelY,
			LocalVelZ:        c.LocalVelZ,
			AngVelX:          c.AngularVelX,
			AngVelY:          c.AngularVelY,
			AngVelZ:          c.AngularVelZ,
			FrontWheelsAngle: c.FrontWheelsAngle,
			FrontAeroHeight:  c.FrontAeroHeight,
			RearAeroHeight:   c.RearAeroHeight,

			WheelCamberRL: c.WheelCamber[0],
			WheelCamberRR: c.WheelCamber[1],
			WheelCamberFL: c.WheelCamber[2],
			WheelCamberFR: c.WheelCamber[3],
		})
	}
	return rows
}
