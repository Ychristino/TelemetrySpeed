# 02 — Backend Architecture

## Overview

The backend is a Go application split into two independent binaries:

| Binary | Role |
|---|---|
| `cmd/listener_udp` | Receives UDP packets from racing games, parses and stores telemetry |
| `cmd/consumer_api` | Serves stored telemetry to the frontend via REST and WebSocket *(not yet implemented)* |

This document covers `listener_udp`. The consumer API will be described in `03-consumer-api.md` once implemented.

---

## Data Flow

```
UDP Socket
    │
    ▼  rawPacketChan (bounded, drop-oldest)
Listener (1 goroutine)
    │
    ▼  rawPacketChan
Parser Workers (N goroutines)
    │  parse header → dispatch to registered GameParser
    │
    ▼  chan *ParsedPacket (per-handler channel)
Source Router (1 goroutine)
    │  key = remote IP:port
    │  creates SourceHandler on first packet
    │
    ▼
SourceHandler (1 goroutine per unique remote addr)
    │  assembles frames, manages session/lap/sector lifecycle
    │  writes sessions/laps/sectors directly to DB
    │
    ▼  writeCh chan []TelemetryRow
TelemetryWriter (1 goroutine)
    │  buffers rows, flushes at ≥500 rows or every 100 ms
    │
    ▼
TimescaleDB
```

---

## Startup Sequence

`cmd/listener_udp/main.go`:

1. Load `.env` (optional; falls back to environment)
2. `config.Load()` — read all env vars, validate required fields
3. `database.New()` — open pgxpool, ping DB
4. `db.Migrate()` — run `001_init.sql` then `002_expand_telemetry.sql`
5. `db.NewTelemetryWriter().Start(ctx)` — start the DB writer goroutine
6. `source.NewSourceRouter(writeCh, sessionWriter)` — create the router
7. `listener_udp.New(cfg, router).Start(ctx)` — bind UDP socket, start receive loop and parser workers
8. Block on `ctx.Done()` (SIGINT / SIGTERM)
9. Graceful shutdown: drain queues, flush DB writer buffer, close pool

---

## Package Map

```
cmd/
  listener_udp/        binary entry point
  consumer_api/        (future) REST + WebSocket API

internal/
  config/              env var loading
  database/
    database.go        DB pool, Migrate, factory methods
    writer.go          TelemetryRow, TelemetryWriter (bulk insert)
    session_writer.go  SessionWriter (sessions/laps/sectors CRUD)
    migrations/
      001_init.sql     full schema for fresh installs
      002_expand_telemetry.sql  idempotent column additions
  f1/
    header.go          PacketHeader, packet ID constants
    packets.go         all F1 struct types (Payload interface)
  parser/
    parser.go          GameParser interface
    registry.go        Register(), Dispatch()
    common.go          shared parse helpers + parsers (LapData, Event, Session)
    f1_2025.go         F1 2023/24/25 parser, self-registers in init()
    f1_2026.go         F1 2026 parser, self-registers in init()
  source/
    router.go          SourceRouter, SourceKey
    handler.go         SourceHandler — frame assembler + session lifecycle
    frame.go           CarFrame, Frame, ToTelemetryRow()
  listener_udp/
    listener_udp.go    UDP socket, receive loop, buffer pool
    processor.go       parse() worker, calls parser.Dispatch()
```

---

## Plug-and-Play Parser Architecture

Every game parser implements one interface:

```go
// internal/parser/parser.go
type GameParser interface {
    Parse(data []byte, hdr f1.PacketHeader) (f1.Payload, bool)
}
```

Parsers self-register for one or more packet format codes in their `init()` function:

```go
// internal/parser/f1_2025.go
func init() {
    Register(f1_2025Parser{}, f1.Format2023, f1.Format2024, f1.Format2025)
}
```

The dispatch path in `processor.go` is a single call:

```go
payload, ok := parser.Dispatch(raw.data, hdr)
```

`Dispatch` reads `hdr.PacketFormat` (uint16), looks up the registered parser, and calls `Parse`. If no parser is registered for that format the packet is silently discarded.

The Source Router, SourceHandler, and TelemetryWriter are all game-agnostic. Adding support for a new game requires only a new file in `internal/parser/`.

---

## Payload Types

All parsers return one of the types defined in `internal/f1/packets.go`. The SourceHandler dispatches on the concrete type via `switch p := pkt.Payload.(type)`.

| Type | Source Packet | Handled by |
|---|---|---|
| `MotionPayload` | ID 0 — Motion | handler: merge into frame |
| `LapPayload` | ID 2 — LapData | handler: lap/sector lifecycle + merge |
| `EventPayload` | ID 3 — Event | handler: SSTA / SEND / CHQF |
| `ParticipantsPayload` | ID 4 — Participants | handler: UPDATE sessions |
| `TelemetryPayload` | ID 6 — CarTelemetry | handler: merge into frame |
| `StatusPayload` | ID 7 — CarStatus | handler: merge into frame |
| `DamagePayload` | ID 10 — CarDamage | handler: merge into frame |
| `MotionExPayload` | ID 13 — MotionEx | handler: merge into player car frame |
| `SessionPayload` | ID 1 — Session | handler: UPDATE sessions.track |

All other packet IDs return `nil, false` from `Parse` and are discarded before reaching the SourceHandler.

---

## Frame Assembly

F1 games guarantee that all 60 Hz packets in a single game frame share the same `m_frameIdentifier`. The SourceHandler exploits this to assemble one complete `Frame` per game frame:

```
receive packet with frameID = N
    if curFrameID == 0:
        start new frame (N)
    elif frameID != curFrameID:
        finalize curFrame → writeCh
        start new frame (N)
    merge packet fields into curFrame.Car
```

Finalization is triggered by the arrival of the *next* frame, not a timer. At 60 Hz there is at most ~16 ms of latency before finalization.

Per-car packets (Motion, LapData, CarTelemetry, CarStatus, CarDamage, Participants) carry data for every car on track, but the parser only decodes the slot at `hdr.PlayerCarIndex` — the rest of the grid is never parsed or stored. `Frame.Car` holds that one car's data, tagged with `Frame.CarIndex`. `ToTelemetryRow()` returns `ok = false` (nothing written) when `ResultStatus ≤ ResultStatusInactive` — e.g. still in the garage before the session goes live.

---

## CarFrame — fields per car per frame

```
// Motion (ID 0)
PosX, PosY, PosZ         float32   world position (metres)
VelX, VelY, VelZ         float32   m/s
GLateral, GLong, GVert   float32   g-forces
Yaw, Pitch, Roll         float32   radians

// CarTelemetry (ID 6)
Speed                    uint16    km/h
Throttle, Brake          float32   0.0–1.0
Steer                    float32   -1.0–1.0
Clutch                   uint8     0–255
Gear                     int8      -1=R 0=N 1–8
RPM                      uint16
DRS                      uint8     0=off 1=on
BrakesTemp[4]            uint16    RL RR FL FR  °C
TyreSurfTemp[4]          uint8     RL RR FL FR  °C
TyreInnerTemp[4]         uint8     RL RR FL FR  °C
EngTemp                  uint16    °C
TyrePressure[4]          float32   RL RR FL FR  PSI

// CarStatus (ID 7)
TractionControl          uint8     0=off 1=med 2=full
AntiLockBrakes           uint8     0/1
FuelMix                  uint8     0–3
PitLimiter               uint8     0/1
FuelInTank               float32   kg
FuelRemLaps              float32
TyreCompound             uint8     compound ID
TyreAgeLaps              uint8
EnginePowerICE           float32   watts
EnginePowerMGUK          float32   watts
ERSStore                 float32   joules
ERSDeployMode            uint8     0–3

// LapData (ID 2)
CurrentLapTimeMS         uint32    ms
LapDistance              float32   metres
CarPosition              uint8     race position (1-based)
CurrentLapNum            uint8
Sector                   uint8     0/1/2
PitStatus                uint8     0=none 1=pitting 2=pit area
LapInvalid               uint8     0=valid 1=invalid
Penalties                uint8     penalty seconds
DriverStatus             uint8
ResultStatus             uint8     0=invalid 1=inactive → skipped

// CarDamage (ID 10)
TyresWear[4]             float32   RL RR FL FR  0–100 %
BrakesDamage[4]          uint8     RL RR FL FR  0–100 %
TyreBlisters[4]          uint8     RL RR FL FR  (F1 26 only; 0 for F1 25)

// MotionEx (ID 13) — player car only; HasMotionEx=false → NULL in DB
HasMotionEx              bool
SuspensionPos[4]         float32   RL RR FL FR
SuspensionVel[4]         float32
SuspensionAccel[4]       float32
WheelSpeed[4]            float32   m/s
WheelSlipRatio[4]        float32
WheelSlipAngle[4]        float32   radians
WheelLatForce[4]         float32
WheelLongForce[4]        float32
WheelVertForce[4]        float32
LocalVelX/Y/Z            float32   m/s, car-local frame
AngularVelX/Y/Z          float32   rad/s
FrontWheelsAngle         float32   radians
FrontAeroHeight          float32   metres
RearAeroHeight           float32   metres
WheelCamber[4]           float32   radians (F1 26 only; 0 for F1 25)
```

---

## Session Lifecycle

```
SSTA event
    → INSERT INTO sessions (game) RETURNING session_id
    → sessionState.sessionID = new UUID

Participants packet
    → UPDATE sessions SET user_id, car

Session packet (ID 1)
    → UPDATE sessions SET track

LapData: first packet with CurrentLapNum > 0
    → INSERT INTO laps (session_id, lap_number, started_at)
    → INSERT INTO sectors (lap_id, sector=1, started_at)
    → sessionState.lapOpen = true

LapData: CurrentLapNum advances
    → UPDATE sectors SET ended_at (current sector)
    → UPDATE laps SET ended_at (previous lap)
    → INSERT INTO laps (new lap)
    → INSERT INTO sectors (sector=1 of new lap)

LapData: Sector advances (on track, not in pit)
    → UPDATE sectors SET ended_at
    → INSERT INTO sectors (new sector)

LapData: PitStatus > 0 (pit entry)
    → UPDATE sectors SET ended_at  (sector closed at pit entry; no new sector opened)
    → sessionState.inPit = true

SEND or CHQF event
    → finalize current frame → writeCh
    → UPDATE sectors SET ended_at
    → UPDATE laps SET ended_at
    → UPDATE sessions SET ended_at
    → sessionState = nil
```

### Mid-stream auto-session

If the listener starts after SSTA was already sent, it detects a lap boundary in LapData (lap number changed since last seen) and auto-creates a session + lap + sector. This avoids joining mid-lap with inaccurate sector timestamps.

---

## TelemetryWriter — Bulk Insert

The TelemetryWriter runs in a single goroutine. It accumulates `[]TelemetryRow` slices sent over `writeCh` and flushes using `pgx.CopyFrom` (PostgreSQL COPY protocol — fastest bulk insert available):

- Flush immediately when buffer has ≥ 500 rows
- Flush on 100 ms ticker otherwise

At 24 cars × 60 Hz the buffer reaches 500 rows in ~350 ms, so the ticker rarely fires during active racing. On shutdown the goroutine drains the channel and does one final flush before returning.

---

## Goroutine Summary

| Goroutine | Count | Reads | Writes | Owns |
|---|---|---|---|---|
| UDP Receive Loop | 1 | network | `rawPacketChan` | UDP socket, buffer pool |
| Parser Worker | N (cfg.MaxWorkers) | `rawPacketChan` | per-handler `inChan` | — |
| Source Router | 1 | parser workers' output | per-handler `inChan` | `map[SourceKey]*SourceHandler` |
| SourceHandler | 1 per remote addr | `inChan` | `writeCh`, DB direct | `sessionState`, `curFrame` |
| TelemetryWriter | 1 | `writeCh` | DB telemetry | row buffer |

Every SourceHandler is a single goroutine with a `select` loop, so its `sessionState` and `curFrame` need no synchronisation — only one goroutine ever touches them.

---

## Write Volume

```
Player car only: 60 Hz = 60 rows/second (active racing)

Bulk flush at 500 rows → every ~8 s
90-min race             → ~325 K telemetry rows
TimescaleDB compression → typically 10–20× on repetitive float data
```

Only the player's active frames are flushed. `ResultStatus ≤ 1` (Invalid or Inactive) is skipped by `ToTelemetryRow()`.
