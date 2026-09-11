package source

import (
	"context"
	"log"
	"time"

	"NewTelemetryEngine/internal/database"
	"NewTelemetryEngine/internal/f1"

	"github.com/google/uuid"
)

type sessionState struct {
	sessionID    uuid.UUID
	lapID        uuid.UUID
	lapOpen      bool
	lapNum       uint8
	sector       uint8 // 0-indexed (game: 0,1,2 → DB: 1,2,3)
	inPit        bool  // true once player entered pit lane this lap
	playerCarIdx uint8
	game         string

	// Lap-time rollover tracking for the player's car — used to detect a
	// finish-line crossing and record the finishing lap_time_ms into the
	// lap_times table. Zero-valued until first seen.
	carLapNum     uint8
	carLapTimeMS  uint32
	carLapInvalid uint8
	// carSector: last-seen sector (0/1/2), cached the same way as
	// carLapTimeMS — used by recordLapTimes to tell a genuine finish-line
	// crossing from a lap-number bump that never reached the final sector.
	carSector uint8
}

// SourceHandler is a single goroutine that owns all session and frame state
// for one unique UDP source (IP+port). It receives ParsedPackets, assembles
// frames, manages the session/lap/sector lifecycle, and sends completed rows
// to the DB writer.
type SourceHandler struct {
	key      SourceKey
	inChan   chan *ParsedPacket
	writeCh  chan<- []database.TelemetryRow
	sw       *database.SessionWriter
	lastSeen time.Time

	// owned exclusively by the handler goroutine — no locking needed
	session       *sessionState
	curFrameID    uint32
	curFrame      Frame
	droppedFrames uint64 // packets dropped while waiting for a session
	prevLapNum    uint8  // last seen lap number, tracked even without a session
	lastHeader    f1.PacketHeader
}

func newSourceHandler(key SourceKey, writeCh chan<- []database.TelemetryRow, sw *database.SessionWriter) *SourceHandler {
	return &SourceHandler{
		key:     key,
		inChan:  make(chan *ParsedPacket, 512),
		writeCh: writeCh,
		sw:      sw,
	}
}

// logDropped logs a warning every 100 dropped packets while there is no active session.
func (h *SourceHandler) logDropped() {
	h.droppedFrames++
	if h.droppedFrames == 1 || h.droppedFrames%100 == 0 {
		log.Printf("[Handler %s] no session — %d packets dropped (waiting for next lap start)",
			h.key, h.droppedFrames)
	}
}

func (h *SourceHandler) run(ctx context.Context) {
	for {
		select {
		case pkt, ok := <-h.inChan:
			if !ok {
				h.finalizeFrame(ctx)
				return
			}
			h.lastSeen = time.Now()
			h.dispatch(ctx, pkt)
		case <-ctx.Done():
			h.finalizeFrame(ctx)
			return
		}
	}
}

func (h *SourceHandler) dispatch(ctx context.Context, pkt *ParsedPacket) {
	h.lastHeader = pkt.Header

	switch p := pkt.Payload.(type) {
	case f1.EventPayload:
		log.Printf("[Handler %s] event: %s", h.key, p.Code)
		h.handleEvent(ctx, p, pkt.Header)
	case f1.SessionPayload:
		h.handleSession(ctx, p)
	case f1.ParticipantsPayload:
		h.handleParticipants(ctx, p)
	case f1.MotionPayload:
		if h.session == nil {
			h.logDropped()
			return
		}
		h.checkNewFrame(ctx, pkt.Header.FrameID)
		h.curFrame.Car.PosX = p.PosX
		h.curFrame.Car.PosY = p.PosY
		h.curFrame.Car.PosZ = p.PosZ
		h.curFrame.Car.VelX = p.VelX
		h.curFrame.Car.VelY = p.VelY
		h.curFrame.Car.VelZ = p.VelZ
		h.curFrame.Car.GLateral = p.GLateral
		h.curFrame.Car.GLong = p.GLong
		h.curFrame.Car.GVert = p.GVert
		h.curFrame.Car.Yaw = p.Yaw
		h.curFrame.Car.Pitch = p.Pitch
		h.curFrame.Car.Roll = p.Roll
	case f1.LapPayload:
		h.handleLapData(ctx, p, pkt.Header.FrameID)
	case f1.TelemetryPayload:
		if h.session == nil {
			h.logDropped()
			return
		}
		h.checkNewFrame(ctx, pkt.Header.FrameID)
		h.curFrame.Car.Speed = p.Speed
		h.curFrame.Car.Throttle = p.Throttle
		h.curFrame.Car.Steer = p.Steer
		h.curFrame.Car.Brake = p.Brake
		h.curFrame.Car.Clutch = p.Clutch
		h.curFrame.Car.Gear = p.Gear
		h.curFrame.Car.RPM = p.RPM
		h.curFrame.Car.DRS = p.DRS
		h.curFrame.Car.BrakesTemp = p.BrakesTemp
		h.curFrame.Car.TyreSurfTemp = p.TyreSurfTemp
		h.curFrame.Car.TyreInnerTemp = p.TyreInnerTemp
		h.curFrame.Car.EngTemp = p.EngTemp
		h.curFrame.Car.TyrePressure = p.TyrePressure
	case f1.StatusPayload:
		if h.session == nil {
			h.logDropped()
			return
		}
		h.checkNewFrame(ctx, pkt.Header.FrameID)
		h.curFrame.Car.TractionControl = p.TractionControl
		h.curFrame.Car.AntiLockBrakes = p.AntiLockBrakes
		h.curFrame.Car.FuelMix = p.FuelMix
		h.curFrame.Car.PitLimiter = p.PitLimiter
		h.curFrame.Car.FuelInTank = p.FuelInTank
		h.curFrame.Car.FuelRemLaps = p.FuelRemLaps
		h.curFrame.Car.TyreCompound = p.TyreCompound
		h.curFrame.Car.TyreAgeLaps = p.TyreAgeLaps
		h.curFrame.Car.EnginePowerICE = p.EnginePowerICE
		h.curFrame.Car.EnginePowerMGUK = p.EnginePowerMGUK
		h.curFrame.Car.ERSStore = p.ERSStore
		h.curFrame.Car.ERSDeployMode = p.ERSDeployMode
	case f1.DamagePayload:
		if h.session == nil {
			h.logDropped()
			return
		}
		h.checkNewFrame(ctx, pkt.Header.FrameID)
		h.curFrame.Car.TyresWear = p.TyresWear
		h.curFrame.Car.BrakesDamage = p.BrakesDamage
		h.curFrame.Car.TyreBlisters = p.TyreBlisters
	case f1.MotionExPayload:
		if h.session == nil {
			h.logDropped()
			return
		}
		h.checkNewFrame(ctx, pkt.Header.FrameID)
		h.curFrame.Car.HasMotionEx = true
		h.curFrame.Car.SuspensionPos = p.SuspensionPos
		h.curFrame.Car.SuspensionVel = p.SuspensionVel
		h.curFrame.Car.SuspensionAccel = p.SuspensionAccel
		h.curFrame.Car.WheelSpeed = p.WheelSpeed
		h.curFrame.Car.WheelSlipRatio = p.WheelSlipRatio
		h.curFrame.Car.WheelSlipAngle = p.WheelSlipAngle
		h.curFrame.Car.WheelLatForce = p.WheelLatForce
		h.curFrame.Car.WheelLongForce = p.WheelLongForce
		h.curFrame.Car.WheelVertForce = p.WheelVertForce
		h.curFrame.Car.LocalVelX = p.LocalVelX
		h.curFrame.Car.LocalVelY = p.LocalVelY
		h.curFrame.Car.LocalVelZ = p.LocalVelZ
		h.curFrame.Car.AngularVelX = p.AngularVelX
		h.curFrame.Car.AngularVelY = p.AngularVelY
		h.curFrame.Car.AngularVelZ = p.AngularVelZ
		h.curFrame.Car.FrontWheelsAngle = p.FrontWheelsAngle
		h.curFrame.Car.FrontAeroHeight = p.FrontAeroHeight
		h.curFrame.Car.RearAeroHeight = p.RearAeroHeight
		h.curFrame.Car.WheelCamber = p.WheelCamber
	}
}

func (h *SourceHandler) handleEvent(ctx context.Context, ev f1.EventPayload, hdr f1.PacketHeader) {
	switch ev.Code {
	case "SSTA":
		if h.session != nil {
			// Close lingering session (e.g., crash without SEND)
			_ = h.sw.CloseSession(ctx, h.session.sessionID)
		}
		game := f1.GameName(hdr.PacketFormat)
		id, err := h.sw.CreateSession(ctx, game)
		if err != nil {
			log.Printf("[Handler %s] create session: %v", h.key, err)
			return
		}
		h.session = &sessionState{
			sessionID:    id,
			playerCarIdx: hdr.PlayerCarIndex,
			game:         game,
		}
		h.curFrameID = 0
		h.curFrame = Frame{}
		log.Printf("[Handler %s] session started: %s", h.key, id)

	case "SEND", "CHQF":
		if h.session == nil {
			return
		}
		h.finalizeFrame(ctx)
		now := time.Now()
		if h.session.lapOpen {
			_ = h.sw.CloseSector(ctx, h.session.lapID, h.session.sector+1, now)
			_ = h.sw.CloseLap(ctx, h.session.lapID, now)
		}
		_ = h.sw.CloseSession(ctx, h.session.sessionID)
		log.Printf("[Handler %s] session ended: %s", h.key, h.session.sessionID)
		h.session = nil
	}
}

func (h *SourceHandler) handleSession(ctx context.Context, s f1.SessionPayload) {
	if h.session == nil {
		return
	}
	track := trackName(s.TrackID)
	if err := h.sw.UpdateSessionTrack(ctx, h.session.sessionID, track); err != nil {
		log.Printf("[Handler %s] update track: %v", h.key, err)
	}
}

func (h *SourceHandler) handleParticipants(ctx context.Context, p f1.ParticipantsPayload) {
	if h.session == nil || p.Name == "" {
		return
	}
	entries := []database.ParticipantEntry{{
		CarIndex:   int16(h.session.playerCarIdx),
		Name:       p.Name,
		Team:       teamName(p.TeamID),
		RaceNumber: int16(p.RaceNumber),
	}}
	if err := h.sw.UpsertParticipants(ctx, h.session.sessionID, entries); err != nil {
		log.Printf("[Handler %s] upsert participants: %v", h.key, err)
	}
}

func (h *SourceHandler) handleLapData(ctx context.Context, lap f1.LapPayload, frameID uint32) {
	player := f1.LapDataItem(lap)

	// No active session — watch for a lap boundary to auto-create one.
	// We wait for a clean lap start rather than joining mid-lap so that
	// sector started_at timestamps are accurate.
	if h.session == nil {
		if player.ResultStatus <= f1.ResultStatusInactive || player.CurrentLapNum == 0 {
			return
		}
		if h.prevLapNum > 0 && player.CurrentLapNum != h.prevLapNum {
			h.autoCreateSession(ctx, player)
		} else {
			h.logDropped()
		}
		h.prevLapNum = player.CurrentLapNum
		return
	}

	h.checkNewFrame(ctx, frameID)

	// Merge lap data into the current frame.
	h.curFrame.Car.CurrentLapTimeMS = player.CurrentLapTimeMS
	h.curFrame.Car.LapDistance = player.LapDistance
	h.curFrame.Car.CarPosition = player.CarPosition
	h.curFrame.Car.CurrentLapNum = player.CurrentLapNum
	h.curFrame.Car.Sector = player.Sector
	h.curFrame.Car.PitStatus = player.PitStatus
	h.curFrame.Car.LapInvalid = player.LapInvalid
	h.curFrame.Car.Penalties = player.Penalties
	h.curFrame.Car.DriverStatus = player.DriverStatus
	h.curFrame.Car.ResultStatus = player.ResultStatus

	h.recordLapTimes(ctx, player)

	now := time.Now()

	if !h.session.lapOpen && player.CurrentLapNum > 0 {
		// First lap data after SSTA — open lap and first sector.
		id, err := h.sw.OpenLap(ctx, h.session.sessionID, player.CurrentLapNum, now)
		if err != nil {
			log.Printf("[Handler %s] open lap %d: %v", h.key, player.CurrentLapNum, err)
			return
		}
		h.session.lapID = id
		h.session.lapNum = player.CurrentLapNum
		h.session.sector = player.Sector
		h.session.inPit = player.PitStatus > f1.PitStatusNone
		h.session.lapOpen = true
		_ = h.sw.OpenSector(ctx, id, player.Sector+1, now)
		return
	}

	if player.CurrentLapNum > h.session.lapNum {
		// Player crossed the finish line — close previous lap/sector, open new.
		if !h.session.inPit {
			_ = h.sw.CloseSector(ctx, h.session.lapID, h.session.sector+1, now)
		}
		_ = h.sw.CloseLap(ctx, h.session.lapID, now)

		id, err := h.sw.OpenLap(ctx, h.session.sessionID, player.CurrentLapNum, now)
		if err != nil {
			log.Printf("[Handler %s] open lap %d: %v", h.key, player.CurrentLapNum, err)
			return
		}
		h.session.lapID = id
		h.session.lapNum = player.CurrentLapNum
		h.session.sector = player.Sector
		h.session.inPit = false
		_ = h.sw.OpenSector(ctx, id, player.Sector+1, now)
		return
	}

	// Pit entry: close the current sector at the moment of pit lane entry.
	if !h.session.inPit && player.PitStatus > f1.PitStatusNone {
		h.session.inPit = true
		if h.session.lapOpen {
			_ = h.sw.CloseSector(ctx, h.session.lapID, h.session.sector+1, now)
			log.Printf("[Handler %s] pit entry — sector %d closed", h.key, h.session.sector+1)
		}
		return
	}

	// Normal sector advance (on track only, not in pit lane).
	if !h.session.inPit && player.Sector > h.session.sector {
		_ = h.sw.CloseSector(ctx, h.session.lapID, h.session.sector+1, now)
		h.session.sector = player.Sector
		_ = h.sw.OpenSector(ctx, h.session.lapID, player.Sector+1, now)
	}
}

// recordLapTimes detects the player's lap rollover — independent of the
// lap/sector open/close tracking above — and persists the finishing
// lap_time_ms for whichever lap just completed. CurrentLapTimeMS resets to
// ~0 in the same packet CurrentLapNum increments, so the finishing time is
// the value captured on the *previous* call (h.session.carLapTimeMS),
// read here before it gets overwritten with the new lap's in-progress time.
//
// The game's lap-number counter also advances on things that aren't a real
// finish-line crossing — a restarted attempt, or a garage/pit return
// mid-lap — and m_currentLapInvalid never flags those (it's for rule
// violations, not incomplete laps). A genuine crossing must have been in
// the final sector (index 2) on the last frame before the counter moved, so
// anything short of that gets forced invalid here regardless of the game's
// own flag.
func (h *SourceHandler) recordLapTimes(ctx context.Context, d f1.LapDataItem) {
	if d.ResultStatus <= f1.ResultStatusInactive {
		return
	}
	prevLapNum := h.session.carLapNum
	if prevLapNum > 0 && d.CurrentLapNum > prevLapNum {
		finishedTimeMS := h.session.carLapTimeMS
		if finishedTimeMS > 0 {
			lapInvalid := h.session.carLapInvalid
			if h.session.carSector < 2 {
				lapInvalid = 1
			}
			carIndex := int16(h.session.playerCarIdx)
			if err := h.sw.RecordLapTime(ctx, h.session.sessionID, carIndex, int(prevLapNum), int32(finishedTimeMS), lapInvalid); err != nil {
				log.Printf("[Handler %s] record lap time car %d lap %d: %v", h.key, carIndex, prevLapNum, err)
			}
		}
	}
	h.session.carLapNum = d.CurrentLapNum
	h.session.carLapTimeMS = d.CurrentLapTimeMS
	h.session.carLapInvalid = d.LapInvalid
	h.session.carSector = d.Sector
}

// autoCreateSession creates a DB session from a lap boundary detected mid-stream.
// Called when the listener joins after SSTA was already sent.
func (h *SourceHandler) autoCreateSession(ctx context.Context, player f1.LapDataItem) {
	game := f1.GameName(h.lastHeader.PacketFormat)
	id, err := h.sw.CreateSession(ctx, game)
	if err != nil {
		log.Printf("[Handler %s] auto-create session: %v", h.key, err)
		return
	}

	now := time.Now()
	h.session = &sessionState{
		sessionID:    id,
		playerCarIdx: h.lastHeader.PlayerCarIndex,
		game:         game,
	}
	h.curFrameID = 0
	h.curFrame = Frame{}
	h.droppedFrames = 0
	log.Printf("[Handler %s] auto-session created (mid-join at lap %d): %s", h.key, player.CurrentLapNum, id)

	lapID, err := h.sw.OpenLap(ctx, id, player.CurrentLapNum, now)
	if err != nil {
		log.Printf("[Handler %s] auto open lap %d: %v", h.key, player.CurrentLapNum, err)
		return
	}
	h.session.lapID = lapID
	h.session.lapNum = player.CurrentLapNum
	h.session.sector = player.Sector
	h.session.inPit = player.PitStatus > f1.PitStatusNone
	h.session.lapOpen = true
	_ = h.sw.OpenSector(ctx, lapID, player.Sector+1, now)
}

// checkNewFrame finalizes the current frame when frameID advances.
func (h *SourceHandler) checkNewFrame(ctx context.Context, frameID uint32) {
	if h.curFrameID == 0 {
		h.curFrameID = frameID
		h.curFrame = Frame{
			SessionID: h.session.sessionID,
			FrameID:   frameID,
			Timestamp: time.Now(),
			CarIndex:  int16(h.session.playerCarIdx),
		}
		return
	}
	if frameID == h.curFrameID {
		return
	}
	h.finalizeFrame(ctx)
	h.curFrameID = frameID
	h.curFrame = Frame{
		SessionID: h.session.sessionID,
		FrameID:   frameID,
		Timestamp: time.Now(),
		CarIndex:  int16(h.session.playerCarIdx),
	}
}

func (h *SourceHandler) finalizeFrame(ctx context.Context) {
	if h.session == nil || h.curFrameID == 0 {
		return
	}
	if row, ok := h.curFrame.ToTelemetryRow(); ok {
		select {
		case h.writeCh <- []database.TelemetryRow{row}:
		default:
			log.Printf("[Handler %s] writeCh full, dropping frame %d", h.key, h.curFrameID)
		}
	}
	h.curFrameID = 0
}

// trackName maps an F1 25 trackId to a human-readable string.
func trackName(id int8) string {
	tracks := map[int8]string{
		0: "Melbourne", 1: "Paul Ricard", 2: "Shanghai", 3: "Sakhir",
		4: "Catalunya", 5: "Monaco", 6: "Montreal", 7: "Silverstone",
		8: "Hockenheim", 9: "Hungaroring", 10: "Spa", 11: "Monza",
		12: "Singapore", 13: "Suzuka", 14: "Abu Dhabi", 15: "Texas",
		16: "Brazil", 17: "Austria", 18: "Sochi", 19: "Mexico",
		20: "Baku", 21: "Sakhir Short", 22: "Silverstone Short",
		23: "Texas Short", 24: "Suzuka Short", 25: "Hanoi",
		26: "Zandvoort", 27: "Imola", 28: "Portimao", 29: "Jeddah",
		30: "Miami", 31: "Las Vegas", 32: "Losail",
	}
	if name, ok := tracks[id]; ok {
		return name
	}
	return "Unknown"
}

// teamName maps a team id to a short name.
func teamName(id uint8) string {
	teams := map[uint8]string{
		0: "Mercedes", 1: "Ferrari", 2: "Red Bull", 3: "Williams",
		4: "Aston Martin", 5: "Alpine", 6: "RB", 7: "Haas",
		8: "McLaren", 9: "Sauber",
	}
	if name, ok := teams[id]; ok {
		return name
	}
	return "Unknown"
}
