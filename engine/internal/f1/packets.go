package f1

// Payload is a sealed interface for all parsed packet payloads.
// Only types in this package satisfy it.
type Payload interface {
	isPayload()
}

// CarMotionData holds per-car data from PacketID=0 (Motion).
// Binary layout within the packet (54 bytes per car in F1 26, 60 in F1 25):
//
//	[0:4]   posX    float32
//	[4:8]   posY    float32
//	[8:12]  posZ    float32
//	[12:16] velX    float32
//	[16:20] velY    float32
//	[20:24] velZ    float32
//	[24:36] forward/right dir vectors (int16 ×6) — skipped
//	F1 25: [36:40] gLat float32  [40:44] gLong  [44:48] gVert  [48:52] yaw  [52:56] pitch  [56:60] roll
//	F1 26: [36:38] gLat int16/1000  [38:40] gLong  [40:42] gVert  [42:46] yaw  [46:50] pitch  [50:54] roll
type CarMotionData struct {
	PosX, PosY, PosZ       float32
	VelX, VelY, VelZ       float32
	GLateral, GLong, GVert float32
	Yaw, Pitch, Roll       float32
}

// LapDataItem holds per-car data from PacketID=2 (LapData).
// Binary layout within the packet (57 bytes per car):
//
//	[0:4]   lastLapTimeMS    uint32
//	[4:8]   currentLapTimeMS uint32
//	[20:24] lapDistance      float32
//	[32]    carPosition      uint8
//	[33]    currentLapNum    uint8
//	[34]    pitStatus        uint8  (0=none,1=pitting,2=in box)
//	[36]    sector           uint8  (0/1/2)
//	[37]    lapInvalid       uint8
//	[38]    penalties        uint8  (accumulated seconds)
//	[44]    driverStatus     uint8
//	[45]    resultStatus     uint8  (0=invalid,1=inactive,2=active…)
const (
	PitStatusNone    = 0
	PitStatusPitting = 1
	PitStatusInBox   = 2

	DriverStatusGarage    = 0
	DriverStatusFlyingLap = 1
	DriverStatusInLap     = 2
	DriverStatusOutLap    = 3
	DriverStatusOnTrack   = 4
)

type LapDataItem struct {
	CurrentLapTimeMS uint32
	LapDistance      float32
	CarPosition      uint8
	CurrentLapNum    uint8
	PitStatus        uint8
	Sector           uint8
	LapInvalid       uint8
	Penalties        uint8
	DriverStatus     uint8
	ResultStatus     uint8
}

// CarTelemetryData holds per-car data from PacketID=6 (CarTelemetry).
// Binary layout within the packet (60 bytes F1 25 / 59 bytes F1 26 per car):
//
//	[0:2]   speed         uint16
//	[2:6]   throttle      float32
//	[6:10]  steer         float32
//	[10:14] brake         float32
//	[14]    clutch        uint8   (0–100)
//	[15]    gear          int8
//	[16:18] engineRPM     uint16
//	[18]    drs           uint8
//	[22:30] brakesTemp[4] uint16 each
//	[30:34] tyreSurfTemp[4] uint8
//	[34:38] tyreInnerTemp[4] uint8
//	F1 25: [38:40] engTemp uint16;  [40:56] tyrePressure[4] float32
//	F1 26: [38]    engTemp uint8;   [39:55] tyrePressure[4] float32
type CarTelemetryData struct {
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
}

// CarStatusData holds per-car data from PacketID=7 (CarStatus).
// Binary layout within the packet (55 bytes F1 25 / 59 bytes F1 26 per car):
//
//	[0]    tractionControl  uint8
//	[1]    antiLockBrakes   uint8
//	[2]    fuelMix          uint8
//	[4]    pitLimiterStatus uint8
//	[5:9]  fuelInTank       float32
//	[13:17] fuelRemLaps     float32
//	[25]   actualTyreCmpd   uint8
//	[27]   tyreAgeLaps      uint8
//	[29:33] enginePowerICE  float32
//	[33:37] enginePowerMGUK float32
//	[37:41] ersStore        float32
//	[41]   ersDeployMode    uint8
type CarStatusData struct {
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
}

// CarDamageData holds per-car data from PacketID=10 (CarDamage).
// Binary layout per car (42 bytes F1 25 / 46 bytes F1 26):
//
//	[0:16]  tyresWear[4]     float32
//	[16:20] tyresDamage[4]   uint8  — not extracted
//	[20:24] brakesDamage[4]  uint8
//	F1 26 only: [24:28] tyreBlisters[4] uint8
type CarDamageData struct {
	TyresWear    [4]float32
	BrakesDamage [4]uint8
	TyreBlisters [4]uint8 // F1 26 only; zero for F1 25
}

// MotionExData holds extended motion data from PacketID=13 (MotionEx).
// This packet contains player-car-only data.
// Binary layout of the data section (212 bytes F1 25 / 244 bytes F1 26):
//
//	[0:16]   suspensionPosition[4]   float32  RL,RR,FL,FR
//	[16:32]  suspensionVelocity[4]
//	[32:48]  suspensionAcceleration[4]
//	[48:64]  wheelSpeed[4]
//	[64:80]  wheelSlipRatio[4]
//	[80:96]  wheelSlipAngle[4]
//	[96:112] wheelLatForce[4]
//	[112:128] wheelLongForce[4]
//	[128:132] heightOfCOG            float32  — skipped
//	[132:136] localVelocityX
//	[136:140] localVelocityY
//	[140:144] localVelocityZ
//	[144:148] angularVelocityX
//	[148:152] angularVelocityY
//	[152:156] angularVelocityZ
//	[156:168] angularAcceleration[3] — skipped
//	[168:172] frontWheelsAngle
//	[172:188] wheelVertForce[4]
//	[188:192] frontAeroHeight
//	[192:196] rearAeroHeight
//	[196:212] rollAngles+chassisYaw+chassisPitch — skipped
//	F1 26 only: [212:228] wheelCamber[4]
type MotionExData struct {
	SuspensionPos    [4]float32
	SuspensionVel    [4]float32
	SuspensionAccel  [4]float32
	WheelSpeed       [4]float32
	WheelSlipRatio   [4]float32
	WheelSlipAngle   [4]float32
	WheelLatForce    [4]float32
	WheelLongForce   [4]float32
	WheelVertForce   [4]float32
	LocalVelX        float32
	LocalVelY        float32
	LocalVelZ        float32
	AngularVelX      float32
	AngularVelY      float32
	AngularVelZ      float32
	FrontWheelsAngle float32
	FrontAeroHeight  float32
	RearAeroHeight   float32
	WheelCamber      [4]float32 // F1 26 only; zero for F1 25
}

// SessionEvent is the payload for PacketID=3 (Event).
type SessionEvent struct {
	Code string
}

// SessionInfo is the payload for PacketID=1 (Session).
type SessionInfo struct {
	TrackID     int8
	SessionType uint8
}

// ParticipantInfo is per-car data from PacketID=4 (Participants).
type ParticipantInfo struct {
	Name       string
	TeamID     uint8
	DriverID   uint8
	RaceNumber uint8
}

// Payload type aliases — one per packet kind. Per-car packets carry data for
// every car on track, but only the player's slot is decoded (see the parser
// package), so these hold a single car's data rather than the full grid.
type MotionPayload CarMotionData
type LapPayload LapDataItem
type TelemetryPayload CarTelemetryData
type StatusPayload CarStatusData
type DamagePayload CarDamageData
type EventPayload SessionEvent
type SessionPayload SessionInfo
type ParticipantsPayload ParticipantInfo
type MotionExPayload MotionExData

func (MotionPayload) isPayload()       {}
func (LapPayload) isPayload()          {}
func (TelemetryPayload) isPayload()    {}
func (StatusPayload) isPayload()       {}
func (DamagePayload) isPayload()       {}
func (EventPayload) isPayload()        {}
func (SessionPayload) isPayload()      {}
func (ParticipantsPayload) isPayload() {}
func (MotionExPayload) isPayload()     {}
