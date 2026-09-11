# 03 — Developer Guide

## Running the System

### Prerequisites

- Docker + Docker Compose
- Go 1.24+
- F1 game with UDP telemetry output enabled (see game settings below)

### Environment Variables

Create `.env` in the project root (or export to environment):

```env
# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=telemetry
DB_PASSWORD=telemetry
DB_NAME=telemetry

# Listener
LISTENER_PORT=20777
UDP_QUEUE_SIZE=256
MAX_WORKERS=4
UDP_INCOME_DEADLINE=5
```

| Variable | Purpose | Typical value |
|---|---|---|
| `LISTENER_PORT` | UDP port the listener binds on | 20777 (F1 default) |
| `UDP_QUEUE_SIZE` | rawPacketChan buffer depth | 256 |
| `MAX_WORKERS` | Parser worker goroutines | 4 |
| `UDP_INCOME_DEADLINE` | Socket read deadline (seconds) | 5 |

### Start TimescaleDB

```bash
docker run -d \
  --name telemetry-db \
  -e POSTGRES_USER=telemetry \
  -e POSTGRES_PASSWORD=telemetry \
  -e POSTGRES_DB=telemetry \
  -p 5432:5432 \
  timescale/timescaledb:latest-pg16
```

### Run the listener

```bash
go run ./cmd/listener_udp
```

Migrations run automatically on every startup. Expected output:

```
[main] migrations applied
[main] listener starting
```

### F1 game settings

In the game's telemetry settings:

| Setting | Value |
|---|---|
| UDP Telemetry | On |
| IP Address | Your machine's LAN IP |
| Port | 20777 (matches `LISTENER_PORT`) |
| Format | F1 2025 or F1 2026 |
| Send rate | 60 Hz (recommended) |

### Verifying data is arriving

Watch the log output. Once a session starts you'll see:

```
[Handler 192.168.1.10:20777] session started: <uuid>
[DBWriter] inserted 528 telemetry rows
[DBWriter] inserted 480 telemetry rows
```

In psql or pgAdmin:
```sql
SELECT count(*) FROM telemetry;
SELECT * FROM sessions ORDER BY started_at DESC LIMIT 5;
```

---

## How to Add a New Game

Adding support for a new game requires touching only the `internal/parser/` package. Everything else (Source Router, SourceHandler, TelemetryWriter, DB) is game-agnostic.

### Step 1 — Add format constant

In `internal/f1/header.go`, add a constant for the new format:

```go
const (
    // existing
    Format2025 = 2025
    Format2026 = 2026
    // new
    FormatMyGame = 9999
)
```

If the game uses a different packet header layout than F1, also update `ParseHeader()` in the same file.

### Step 2 — Define payload structs (if needed)

If the new game's packet types differ significantly from the existing F1 structs, add new types to `internal/f1/packets.go` implementing the `Payload` interface:

```go
type MyGameMotionPayload MyGameMotionData
func (MyGameMotionPayload) isPayload() {}
```

If the game's data maps cleanly onto the existing types (speed, throttle, position, etc.) reuse the existing payload types — no new structs needed.

### Step 3 — Create the parser file

Create `internal/parser/mygame.go`:

```go
package parser

import "NewTelemetryEngine/internal/f1"

const (
    mygameNumCars      = 20
    mygameMotionSize   = 48
    mygameTelemetrySize = 56
)

type myGameParser struct{}

func init() {
    Register(myGameParser{}, f1.FormatMyGame)
}

func (p myGameParser) Parse(data []byte, hdr f1.PacketHeader) (f1.Payload, bool) {
    switch hdr.PacketID {
    case f1.PacketIDMotion:
        return parseMygameMotion(data, hdr.PlayerCarIndex)
    case f1.PacketIDCarTelemetry:
        return parseMygameTelemetry(data, hdr.PlayerCarIndex)
    case f1.PacketIDEvent:
        return parseEvent(data) // shared helper if event format matches
    default:
        return nil, false
    }
}

// Per-car packets carry data for every car on track, but only the player's
// slot is decoded — the engine doesn't track the rest of the grid.
func parseMygameMotion(data []byte, playerIdx uint8) (f1.MotionPayload, bool) {
    if int(playerIdx) >= mygameNumCars || len(data) < f1.HeaderSize+mygameNumCars*mygameMotionSize {
        return f1.MotionPayload{}, false
    }
    base := f1.HeaderSize + int(playerIdx)*mygameMotionSize
    c := newCursor(data[base : base+mygameMotionSize])
    return f1.MotionPayload{
        PosX: c.F32(),
        PosY: c.F32(),
        // ... read fields in spec order, c.Skip(n) over ones you don't need
    }, true
}
```

Parsers read fields with a `cursor` (`internal/parser/cursor.go`) instead of hand-computed byte ranges like `d[4:8]`. `cursor` methods (`U8`, `U16`, `U32`, `F32`, `F32x4`, `U8x4`, `U16x4`, `NullTermStr`) advance an internal position, so a parse function reads top-to-bottom in the same order the game's spec table lists fields, and `c.Skip(n)` steps over bytes you don't need. This keeps parse functions readable and means only the bytes around a changed field need touching when a new game year tweaks the layout — not every offset after it.

`init()` runs automatically when the `parser` package is imported — no registration call needed anywhere else.

### Step 4 — Update `GameName()` (optional)

In `internal/f1/header.go`, add a case to `GameName()` so sessions get a meaningful `game` value in the DB:

```go
func GameName(format uint16) string {
    switch format {
    case Format2025: return "f1_2025"
    case Format2026: return "f1_2026"
    case FormatMyGame: return "mygame_2026"
    default: return fmt.Sprintf("unknown_%d", format)
    }
}
```

### Step 5 — Map to CarFrame in SourceHandler (if new packet IDs)

If the new game introduces packet IDs that don't exist in F1 (and you added new payload types for them), handle them in `internal/source/handler.go`'s `dispatch()` switch:

```go
case MyGameSpecialPayload:
    h.checkNewFrame(ctx, pkt.Header.FrameID)
    h.curFrame.Car.SomeField = p.Value
```

If the new game uses the same F1 payload types, `dispatch()` already handles them — nothing to change.

---

## How to Add a New Telemetry Field

Adding a new field end-to-end touches six files. Work through them in order.

### Checklist

- [ ] `internal/f1/packets.go` — add field to the right struct
- [ ] `internal/parser/f1_2025.go` and/or `f1_2026.go` — parse the byte offset
- [ ] `internal/source/frame.go` — add field to `CarFrame`
- [ ] `internal/source/handler.go` — merge field from payload into `curFrame`
- [ ] `internal/source/frame.go` (`ToTelemetryRow`) — copy field to `TelemetryRow`
- [ ] `internal/database/writer.go` — add field to `TelemetryRow` struct, `telemetryCols` slice, and `bulkInsert` row slice
- [ ] `internal/database/migrations/002_expand_telemetry.sql` — `ADD COLUMN IF NOT EXISTS`

### Worked example: adding `wing_damage_front SMALLINT`

**1. `internal/f1/packets.go` — `CarDamageData`**

```go
type CarDamageData struct {
    TyresWear    [4]float32
    BrakesDamage [4]uint8
    TyreBlisters [4]uint8
    WingDamageFront uint8  // ← new
}
```

**2. `internal/parser/f1_2025.go` — `parseCarDamage25`**

```go
tyresWear := c.F32x4()
c.Skip(4) // [16:20] tyresDamage — unused
brakesDamage := c.U8x4()
c.Skip(6) // [24:30] tyreBlisters etc. — unused
return f1.DamagePayload{
    TyresWear:       tyresWear,
    BrakesDamage:    brakesDamage,
    WingDamageFront: c.U8(), // ← new, read where the spec places it
}, true
```

Do the same in `f1_2026.go` — the field sits at whatever point in the read sequence its own spec places it, which may not be the same `Skip` count.

**3. `internal/source/frame.go` — `CarFrame`**

```go
// CarDamage (PacketID=10)
TyresWear        [4]float32
BrakesDamage     [4]uint8
TyreBlisters     [4]uint8
WingDamageFront  uint8  // ← new
```

**4. `internal/source/handler.go` — `dispatch()`, `DamagePayload` case**

```go
case f1.DamagePayload:
    h.curFrame.Car.TyresWear       = p.TyresWear
    h.curFrame.Car.BrakesDamage    = p.BrakesDamage
    h.curFrame.Car.TyreBlisters    = p.TyreBlisters
    h.curFrame.Car.WingDamageFront = p.WingDamageFront  // ← new
```

**5. `internal/source/frame.go` — `ToTelemetryRow()`**

```go
row := database.TelemetryRow{
    // ... existing fields ...
    WingDamageFront: int16(c.WingDamageFront),  // ← new
}
```

**6. `internal/database/writer.go`**

Add to `TelemetryRow` struct:
```go
WingDamageFront int16
```

Add to `telemetryCols` in the right position (must match the `bulkInsert` row slice exactly):
```go
var telemetryCols = []string{
    // ...
    "brake_damage_rl", "brake_damage_rr", "brake_damage_fl", "brake_damage_fr",
    "wing_damage_front",  // ← new, after brake_damage
    // ...
}
```

Add to the row slice in `bulkInsert`:
```go
copyRows[i] = []any{
    // ... in the same position as telemetryCols ...
    r.BrakeDamageRL, r.BrakeDamageRR, r.BrakeDamageFL, r.BrakeDamageFR,
    r.WingDamageFront,  // ← new
    // ...
}
```

**7. `internal/database/migrations/002_expand_telemetry.sql`**

```sql
-- Wing damage
ALTER TABLE telemetry ADD COLUMN IF NOT EXISTS wing_damage_front SMALLINT;
```

The migration runs on next startup. No restart of a running DB required — `ADD COLUMN IF NOT EXISTS` is safe on a live table.

### Type mapping reference

| Go type | DB type | Notes |
|---|---|---|
| `float32` | `REAL` | standard |
| `uint8`, `int8`, `uint16`, `int16` | `SMALLINT` | cast to `int16` in TelemetryRow |
| `uint32`, `int32` | `INTEGER` | cast to `int32` |
| `float64` | `DOUBLE PRECISION` | rarely needed |
| nullable float | `REAL` (nullable) | use `optF(hasData, value)` in bulkInsert; field type stays `float32` in TelemetryRow |

---

## Critical Invariant: `telemetryCols` and `bulkInsert` must stay in sync

`pgx.CopyFrom` maps columns positionally. The column names in `telemetryCols` and the values in the `copyRows[i] = []any{...}` slice must appear in identical order. If they diverge, data silently lands in the wrong columns.

When adding a field, always add it to **both** in the same relative position. The safest way is to add it at the end of its logical group in both lists simultaneously.

---

## API Layer (Upcoming)

The `cmd/consumer_api` binary will read from the same DB. Key queries to implement:

- `GET /sessions` — list sessions with metadata
- `GET /sessions/{id}/laps` — lap times for a session
- `GET /sessions/{id}/laps/{n}/telemetry?car={idx}&channels=speed,throttle,brake` — channel data for one lap
- `GET /sessions/{id}/laps/{n}/sectors` — sector times
- `WS  /sessions/{id}/stream` — live telemetry during active session

See `01-database.md` for the SQL patterns behind each of these.

The telemetry query pattern:
```sql
SELECT ts, <channels>
FROM   telemetry t
JOIN   laps l ON l.session_id = $1 AND l.lap_number = $2
WHERE  t.session_id = $1
  AND  t.car_index  = $3
  AND  t.ts BETWEEN l.started_at AND l.ended_at
ORDER  BY t.ts;
```

`channels` should be a whitelist on the API side — never interpolate user input into the column list directly.
