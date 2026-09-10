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

	// Per-car lap tracking (all cars, not just the player) — used to detect
	// each car's lap rollover and record its finishing lap_time_ms into the
	// lap_times table. Indexed by car_index; zero-valued until first seen.
	carLapNum     [f1.MaxCars]uint8
	carLapTimeMS  [f1.MaxCars]uint32
	carLapInvalid [f1.MaxCars]uint8
	// carSector: last-seen sector (0/1/2) for each car, cached the same way as
	// carLapTimeMS — used by recordLapTimes to tell a genuine finish-line
	// crossing from a lap-number bump that never reached the final sector.
	carSector [f1.MaxCars]uint8
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
		for i, d := range p {
			h.curFrame.Cars[i].PosX = d.PosX
			h.curFrame.Cars[i].PosY = d.PosY
			h.curFrame.Cars[i].PosZ = d.PosZ
			h.curFrame.Cars[i].VelX = d.VelX
			h.curFrame.Cars[i].VelY = d.VelY
			h.curFrame.Cars[i].VelZ = d.VelZ
			h.curFrame.Cars[i].GLateral = d.GLateral
			h.curFrame.Cars[i].GLong = d.GLong
			h.curFrame.Cars[i].GVert = d.GVert
			h.curFrame.Cars[i].Yaw = d.Yaw
			h.curFrame.Cars[i].Pitch = d.Pitch
			h.curFrame.Cars[i].Roll = d.Roll
		}
	case f1.LapPayload:
		h.handleLapData(ctx, p, pkt.Header.FrameID)
	case f1.TelemetryPayload:
		if h.session == nil {
			h.logDropped()
			return
		}
		h.checkNewFrame(ctx, pkt.Header.FrameID)
		for i, d := range p {
			h.curFrame.Cars[i].Speed = d.Speed
			h.curFrame.Cars[i].Throttle = d.Throttle
			h.curFrame.Cars[i].Steer = d.Steer
			h.curFrame.Cars[i].Brake = d.Brake
			h.curFrame.Cars[i].Clutch = d.Clutch
			h.curFrame.Cars[i].Gear = d.Gear
			h.curFrame.Cars[i].RPM = d.RPM
			h.curFrame.Cars[i].DRS = d.DRS
			h.curFrame.Cars[i].BrakesTemp = d.BrakesTemp
			h.curFrame.Cars[i].TyreSurfTemp = d.TyreSurfTemp
			h.curFrame.Cars[i].TyreInnerTemp = d.TyreInnerTemp
			h.curFrame.Cars[i].EngTemp = d.EngTemp
			h.curFrame.Cars[i].TyrePressure = d.TyrePressure
		}
	case f1.StatusPayload:
		if h.session == nil {
			h.logDropped()
			return
		}
		h.checkNewFrame(ctx, pkt.Header.FrameID)
		for i, d := range p {
			h.curFrame.Cars[i].TractionControl = d.TractionControl
			h.curFrame.Cars[i].AntiLockBrakes = d.AntiLockBrakes
			h.curFrame.Cars[i].FuelMix = d.FuelMix
			h.curFrame.Cars[i].PitLimiter = d.PitLimiter
			h.curFrame.Cars[i].FuelInTank = d.FuelInTank
			h.curFrame.Cars[i].FuelRemLaps = d.FuelRemLaps
			h.curFrame.Cars[i].TyreCompound = d.TyreCompound
			h.curFrame.Cars[i].TyreAgeLaps = d.TyreAgeLaps
			h.curFrame.Cars[i].EnginePowerICE = d.EnginePowerICE
			h.curFrame.Cars[i].EnginePowerMGUK = d.EnginePowerMGUK
			h.curFrame.Cars[i].ERSStore = d.ERSStore
			h.curFrame.Cars[i].ERSDeployMode = d.ERSDeployMode
		}
	case f1.DamagePayload:
		if h.session == nil {
			h.logDropped()
			return
		}
		h.checkNewFrame(ctx, pkt.Header.FrameID)
		for i, d := range p {
			h.curFrame.Cars[i].TyresWear = d.TyresWear
			h.curFrame.Cars[i].BrakesDamage = d.BrakesDamage
			h.curFrame.Cars[i].TyreBlisters = d.TyreBlisters
		}
	case f1.MotionExPayload:
		if h.session == nil {
			h.logDropped()
			return
		}
		h.checkNewFrame(ctx, pkt.Header.FrameID)
		idx := pkt.Header.PlayerCarIndex
		h.curFrame.Cars[idx].HasMotionEx = true
		h.curFrame.Cars[idx].SuspensionPos = p.SuspensionPos
		h.curFrame.Cars[idx].SuspensionVel = p.SuspensionVel
		h.curFrame.Cars[idx].SuspensionAccel = p.SuspensionAccel
		h.curFrame.Cars[idx].WheelSpeed = p.WheelSpeed
		h.curFrame.Cars[idx].WheelSlipRatio = p.WheelSlipRatio
		h.curFrame.Cars[idx].WheelSlipAngle = p.WheelSlipAngle
		h.curFrame.Cars[idx].WheelLatForce = p.WheelLatForce
		h.curFrame.Cars[idx].WheelLongForce = p.WheelLongForce
		h.curFrame.Cars[idx].WheelVertForce = p.WheelVertForce
		h.curFrame.Cars[idx].LocalVelX = p.LocalVelX
		h.curFrame.Cars[idx].LocalVelY = p.LocalVelY
		h.curFrame.Cars[idx].LocalVelZ = p.LocalVelZ
		h.curFrame.Cars[idx].AngularVelX = p.AngularVelX
		h.curFrame.Cars[idx].AngularVelY = p.AngularVelY
		h.curFrame.Cars[idx].AngularVelZ = p.AngularVelZ
		h.curFrame.Cars[idx].FrontWheelsAngle = p.FrontWheelsAngle
		h.curFrame.Cars[idx].FrontAeroHeight = p.FrontAeroHeight
		h.curFrame.Cars[idx].RearAeroHeight = p.RearAeroHeight
		h.curFrame.Cars[idx].WheelCamber = p.WheelCamber
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
	if h.session == nil {
		return
	}
	entries := make([]database.ParticipantEntry, 0, len(p))
	for i, info := range p {
		if info.Name == "" {
			continue
		}
		entries = append(entries, database.ParticipantEntry{
			CarIndex:   int16(i),
			Name:       info.Name,
			Team:       teamName(info.TeamID),
			RaceNumber: int16(info.RaceNumber),
		})
	}
	if err := h.sw.UpsertParticipants(ctx, h.session.sessionID, entries); err != nil {
		log.Printf("[Handler %s] upsert participants: %v", h.key, err)
	}
}

func (h *SourceHandler) handleLapData(ctx context.Context, lap f1.LapPayload, frameID uint32) {
	playerIdx := h.lastHeader.PlayerCarIndex
	player := lap[playerIdx]

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

	// Merge lap data into current frame for all cars.
	for i, d := range lap {
		h.curFrame.Cars[i].CurrentLapTimeMS = d.CurrentLapTimeMS
		h.curFrame.Cars[i].LapDistance = d.LapDistance
		h.curFrame.Cars[i].CarPosition = d.CarPosition
		h.curFrame.Cars[i].CurrentLapNum = d.CurrentLapNum
		h.curFrame.Cars[i].Sector = d.Sector
		h.curFrame.Cars[i].PitStatus = d.PitStatus
		h.curFrame.Cars[i].LapInvalid = d.LapInvalid
		h.curFrame.Cars[i].Penalties = d.Penalties
		h.curFrame.Cars[i].DriverStatus = d.DriverStatus
		h.curFrame.Cars[i].ResultStatus = d.ResultStatus
	}

	h.recordLapTimes(ctx, lap)

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

// recordLapTimes detects each car's lap rollover — independent of the
// player-only lap/sector tracking above — and persists the finishing
// lap_time_ms for whichever lap just completed. CurrentLapTimeMS resets to
// ~0 in the same packet CurrentLapNum increments, so the finishing time is
// the value captured on the *previous* call (h.session.carLapTimeMS[i]),
// read here before it gets overwritten with the new lap's in-progress time.
//
// The game's lap-number counter also advances on things that aren't a real
// finish-line crossing — a restarted attempt, or a garage/pit return
// mid-lap — and m_currentLapInvalid never flags those (it's for rule
// violations, not incomplete laps). A genuine crossing must have been in
// the final sector (index 2) on the last frame before the counter moved, so
// anything short of that gets forced invalid here regardless of the game's
// own flag.
func (h *SourceHandler) recordLapTimes(ctx context.Context, lap f1.LapPayload) {
	for i, d := range lap {
		if d.ResultStatus <= f1.ResultStatusInactive {
			continue
		}
		prevLapNum := h.session.carLapNum[i]
		if prevLapNum > 0 && d.CurrentLapNum > prevLapNum {
			finishedTimeMS := h.session.carLapTimeMS[i]
			if finishedTimeMS > 0 {
				lapInvalid := h.session.carLapInvalid[i]
				if h.session.carSector[i] < 2 {
					lapInvalid = 1
				}
				if err := h.sw.RecordLapTime(ctx, h.session.sessionID, int16(i), int(prevLapNum), int32(finishedTimeMS), lapInvalid); err != nil {
					log.Printf("[Handler %s] record lap time car %d lap %d: %v", h.key, i, prevLapNum, err)
				}
			}
		}
		h.session.carLapNum[i] = d.CurrentLapNum
		h.session.carLapTimeMS[i] = d.CurrentLapTimeMS
		h.session.carLapInvalid[i] = d.LapInvalid
		h.session.carSector[i] = d.Sector
	}
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
	}
}

func (h *SourceHandler) finalizeFrame(ctx context.Context) {
	if h.session == nil || h.curFrameID == 0 {
		return
	}
	rows := h.curFrame.ToTelemetryRows()
	if len(rows) > 0 {
		select {
		case h.writeCh <- rows:
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
