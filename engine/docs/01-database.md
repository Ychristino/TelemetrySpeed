# 01 — Database Schema

## Database: TimescaleDB (PostgreSQL + TimescaleDB extension)

### Why TimescaleDB

Racing telemetry requires two kinds of data that must live together:

- **Relational metadata** — sessions, laps, sectors. These are discrete events with foreign-key relationships. SQL is the right model.
- **High-frequency time-series** — vehicle and engine measurements at 60 Hz. These need time-based partitioning and fast range scans.

TimescaleDB is PostgreSQL with time-series extensions. It handles both in a single database with full SQL, JOINs, foreign keys, and constraints. The alternative (InfluxDB) handles time-series well but has no relational model, requires Flux instead of SQL, and its `session_id`-as-tag pattern causes series cardinality explosion that degrades write performance over time.

---

## Entity Relationships

```
sessions  ──< laps ──< sectors
sessions  ──< telemetry
```

- One session contains many laps.
- One lap contains up to three sectors (1, 2, 3).
- One session contains many telemetry rows (all cars, 60 Hz).
- Lap and sector boundaries are applied to telemetry at query time using time ranges — telemetry rows themselves carry no `lap_id` or `sector`.

---

## Tables

### sessions

One row per continuous track run — from SSTA event to SEND/CHQF.

| Column | Type | Notes |
|---|---|---|
| session_id | UUID PK | auto-generated |
| started_at | TIMESTAMPTZ | server wall clock at SSTA |
| ended_at | TIMESTAMPTZ | NULL while in progress |
| game | TEXT | `"f1_2025"`, `"f1_2026"`, etc. |
| user_id | TEXT | player name from Participants packet |
| track | TEXT | human-readable track name |
| car | TEXT | team name (e.g. `"McLaren"`) |

### laps

One row per lap within a session. `ON CONFLICT DO NOTHING` on `(session_id, lap_number)` prevents duplicates on reconnect.

| Column | Type | Notes |
|---|---|---|
| lap_id | UUID PK | auto-generated |
| session_id | UUID FK | → sessions (CASCADE) |
| lap_number | INT | 1-based |
| started_at | TIMESTAMPTZ | server wall clock when lap number advanced |
| ended_at | TIMESTAMPTZ | NULL while in progress |

### sectors

One row per sector within a lap. Sector is 1-indexed (1, 2, 3) — game sends 0-indexed, the backend adds 1 before writing.

| Column | Type | Notes |
|---|---|---|
| lap_id | UUID FK | → laps (CASCADE) |
| sector | INT | 1, 2, or 3 |
| started_at | TIMESTAMPTZ | server wall clock when sector began |
| ended_at | TIMESTAMPTZ | NULL while in progress; pit-entry closes sector early |

Primary key is `(lap_id, sector)`.

### telemetry

High-frequency vehicle measurements. **TimescaleDB hypertable** partitioned by `ts`. One row per car per frame (~60 Hz). All column groups except the three primary keys are nullable — a game that doesn't emit a packet simply leaves those fields NULL.

---

## Telemetry Column Reference

All four-wheel arrays follow the order **RL, RR, FL, FR** (Rear-Left, Rear-Right, Front-Left, Front-Right).

### Primary keys

| Column | Type | Source | Notes |
|---|---|---|---|
| ts | TIMESTAMPTZ | server wall clock | hypertable time dimension |
| session_id | UUID | SSTA event | FK → sessions (CASCADE) |
| car_index | SMALLINT | packet header `m_playerCarIndex` / loop index | 0–21 for F1 25; 0–23 for F1 26 |

### Driver inputs — CarTelemetry packet (ID 6)

| Column | Type | Unit | Notes |
|---|---|---|---|
| speed | REAL | km/h | |
| throttle | REAL | 0.0–1.0 | |
| brake | REAL | 0.0–1.0 | |
| steering | REAL | -1.0 (full left) to 1.0 (full right) | |
| gear | SMALLINT | — | -1=R 0=N 1–8 |
| clutch | REAL | 0–255 raw → stored as-is | |
| drs | SMALLINT | — | 0=off 1=on |

### Position / orientation — Motion packet (ID 0)

| Column | Type | Unit | Notes |
|---|---|---|---|
| pos_x | REAL | metres | world X |
| pos_y | REAL | metres | world Y (up) |
| pos_z | REAL | metres | world Z |
| vel_x | REAL | m/s | world-space velocity |
| vel_y | REAL | m/s | |
| vel_z | REAL | m/s | |
| yaw | REAL | radians | |
| pitch | REAL | radians | |
| roll | REAL | radians | |

### G-forces — Motion packet (ID 0)

| Column | Type | Unit | Notes |
|---|---|---|---|
| g_lat | REAL | g | lateral (left/right) |
| g_long | REAL | g | longitudinal (front/rear) |
| g_vert | REAL | g | vertical |

Note: F1 2025 sends these as float32. F1 2026 sends them as int16/1000 — the parser normalises to float32 before storing.

### Engine / fuel — CarTelemetry (ID 6) + CarStatus (ID 7)

| Column | Type | Unit | Source |
|---|---|---|---|
| rpm | INT | RPM | CarTelemetry |
| eng_temp | REAL | °C | CarTelemetry |
| fuel | REAL | kg in tank | CarStatus |
| fuel_rem_laps | REAL | laps | CarStatus |
| fuel_mix | SMALLINT | 0–3 | CarStatus (0=lean 1=std 2=rich 3=max) |
| pit_limiter | SMALLINT | 0/1 | CarStatus |
| engine_power_ice | REAL | watts | CarStatus |
| engine_power_mguk | REAL | watts | CarStatus |

### ERS — CarStatus packet (ID 7)

| Column | Type | Unit | Notes |
|---|---|---|---|
| ers_store | REAL | joules | max ~4 000 000 J |
| ers_deploy_mode | SMALLINT | — | 0=none 1=medium 2=hotlap 3=overtake |

### Driver aids — CarStatus packet (ID 7)

| Column | Type | Notes |
|---|---|---|
| traction_control | SMALLINT | 0=off 1=medium 2=full |
| abs | SMALLINT | 0=off 1=on |

### Tyres (scalars) — CarStatus packet (ID 7)

| Column | Type | Notes |
|---|---|---|
| tyre_compound | SMALLINT | actual compound ID (game-specific enum) |
| tyre_age_laps | SMALLINT | laps since tyre change |

### Tyre surface temperature — CarTelemetry packet (ID 6)

| Column | Type | Unit |
|---|---|---|
| tyre_surf_temp_rl | REAL | °C |
| tyre_surf_temp_rr | REAL | °C |
| tyre_surf_temp_fl | REAL | °C |
| tyre_surf_temp_fr | REAL | °C |

### Tyre inner temperature — CarTelemetry packet (ID 6)

| Column | Type | Unit |
|---|---|---|
| tyre_inner_temp_rl | REAL | °C |
| tyre_inner_temp_rr | REAL | °C |
| tyre_inner_temp_fl | REAL | °C |
| tyre_inner_temp_fr | REAL | °C |

### Tyre pressure — CarTelemetry packet (ID 6)

| Column | Type | Unit |
|---|---|---|
| tyre_pressure_rl | REAL | PSI |
| tyre_pressure_rr | REAL | PSI |
| tyre_pressure_fl | REAL | PSI |
| tyre_pressure_fr | REAL | PSI |

### Tyre wear — CarDamage packet (ID 10)

| Column | Type | Unit |
|---|---|---|
| tyre_wear_rl | REAL | 0–100 % |
| tyre_wear_rr | REAL | 0–100 % |
| tyre_wear_fl | REAL | 0–100 % |
| tyre_wear_fr | REAL | 0–100 % |

### Tyre blisters — CarDamage packet (ID 10)

Available in F1 2026 only; always 0 for F1 2025.

| Column | Type | Notes |
|---|---|---|
| tyre_blister_rl | SMALLINT | blister severity |
| tyre_blister_rr | SMALLINT | |
| tyre_blister_fl | SMALLINT | |
| tyre_blister_fr | SMALLINT | |

### Brake temperature — CarTelemetry packet (ID 6)

| Column | Type | Unit |
|---|---|---|
| brake_temp_rl | REAL | °C |
| brake_temp_rr | REAL | °C |
| brake_temp_fl | REAL | °C |
| brake_temp_fr | REAL | °C |

### Brake damage — CarDamage packet (ID 10)

| Column | Type | Unit |
|---|---|---|
| brake_damage_rl | SMALLINT | 0–100 % |
| brake_damage_rr | SMALLINT | |
| brake_damage_fl | SMALLINT | |
| brake_damage_fr | SMALLINT | |

### Lap state — LapData packet (ID 2)

| Column | Type | Notes |
|---|---|---|
| lap_time_ms | INTEGER | current lap time in milliseconds |
| lap_distance | REAL | metres from start line (negative in pits) |
| car_position | SMALLINT | race position (1-based) |
| lap_num | SMALLINT | current lap number |
| sector | SMALLINT | 0, 1, or 2 (0-indexed, game convention) |
| pit_status | SMALLINT | 0=none 1=pitting 2=in pit area |
| driver_status | SMALLINT | 0=garage 1=flying lap 2=in lap 3=out lap 4=grid |
| lap_invalid | SMALLINT | 0=valid 1=invalid |
| penalties | SMALLINT | accumulated penalty seconds |

### MotionEx: suspension — MotionEx packet (ID 13), player car only

NULL for all non-player cars. RL, RR, FL, FR.

| Column | Type | Notes |
|---|---|---|
| susp_pos_rl … susp_pos_fr | REAL | suspension position |
| susp_vel_rl … susp_vel_fr | REAL | suspension velocity |
| susp_accel_rl … susp_accel_fr | REAL | suspension acceleration |

### MotionEx: wheel dynamics — MotionEx packet (ID 13), player car only

| Column | Type | Notes |
|---|---|---|
| wheel_speed_rl … wheel_speed_fr | REAL | m/s |
| wheel_slip_ratio_rl … wheel_slip_ratio_fr | REAL | slip ratio |
| wheel_slip_angle_rl … wheel_slip_angle_fr | REAL | slip angle (radians) |
| wheel_lat_force_rl … wheel_lat_force_fr | REAL | lateral force |
| wheel_long_force_rl … wheel_long_force_fr | REAL | longitudinal force |
| wheel_vert_force_rl … wheel_vert_force_fr | REAL | vertical force |

### MotionEx: body dynamics — MotionEx packet (ID 13), player car only

| Column | Type | Notes |
|---|---|---|
| local_vel_x / y / z | REAL | m/s, car-local frame |
| ang_vel_x / y / z | REAL | rad/s |
| front_wheels_angle | REAL | radians, steering angle |
| front_aero_height | REAL | ride height front (metres) |
| rear_aero_height | REAL | ride height rear (metres) |

### MotionEx: camber — MotionEx packet (ID 13), player car only

Available in F1 2026 (active suspension); always 0.0 for F1 2025.

| Column | Type | Notes |
|---|---|---|
| wheel_camber_rl … wheel_camber_fr | REAL | camber angle (radians) |

---

## Indexes

```sql
-- One car's telemetry over time within a session (primary query pattern)
CREATE INDEX ON telemetry (session_id, car_index, ts DESC);

-- All cars for a session (multi-car overlays, race charts)
CREATE INDEX ON telemetry (session_id, ts DESC);

-- Lap lookup within a session
CREATE INDEX ON laps (session_id, lap_number);
```

---

## Query Patterns

### Telemetry for a specific lap

```sql
SELECT t.*
FROM   telemetry t
JOIN   laps l ON l.session_id = $1 AND l.lap_number = $2
WHERE  t.session_id = $1
  AND  t.car_index  = $3
  AND  t.ts BETWEEN l.started_at AND l.ended_at
ORDER  BY t.ts;
```

### Telemetry for a specific sector

```sql
SELECT t.*
FROM   telemetry t
JOIN   laps    l ON l.session_id = $1 AND l.lap_number = $2
JOIN   sectors s ON s.lap_id = l.lap_id AND s.sector = $3
WHERE  t.session_id = $1
  AND  t.car_index  = $4
  AND  t.ts BETWEEN s.started_at AND s.ended_at
ORDER  BY t.ts;
```

### Lap times for a session

```sql
SELECT
    lap_number,
    EXTRACT(EPOCH FROM (ended_at - started_at)) * 1000 AS duration_ms
FROM   laps
WHERE  session_id = $1
ORDER  BY lap_number;
```

### Sector times for a lap

```sql
SELECT
    sector,
    EXTRACT(EPOCH FROM (ended_at - started_at)) * 1000 AS duration_ms
FROM   sectors
WHERE  lap_id = $1
ORDER  BY sector;
```

### All sessions

```sql
SELECT session_id, game, track, car, user_id, started_at, ended_at
FROM   sessions
ORDER  BY started_at DESC;
```

---

## Design Decisions

**No `lap_id` or `sector` column on `telemetry`**
Lap and sector boundaries are applied as time-range predicates at query time using `laps.started_at/ended_at` and `sectors.started_at/ended_at`. This keeps every telemetry row minimal and avoids the need to know which lap/sector the backend is in before writing a frame. TimescaleDB's chunk exclusion makes these time-range queries efficient.

**No `duration_ms` stored anywhere**
Always derived: `EXTRACT(EPOCH FROM (ended_at - started_at)) * 1000`. Storing it would create redundancy and an update surface.

**MotionEx columns are nullable**
MotionEx (ID 13) only covers the player car. All other cars get NULL for those 56 columns. The backend sets `HasMotionEx = true` only on the player's `CarFrame`; the DB writer uses `optF()` to emit nil for rows where it is false.

**`ON DELETE CASCADE` everywhere**
Deleting a session removes all laps, sectors, and telemetry rows automatically.

**`UNIQUE (session_id, lap_number)` on `laps`**
`INSERT … ON CONFLICT DO NOTHING` prevents duplicate lap rows on reconnect or mid-lap auto-session creation.

**Migrations are idempotent and run on every startup**
`001_init.sql` uses `CREATE TABLE IF NOT EXISTS` and `CREATE EXTENSION IF NOT EXISTS`.
`002_expand_telemetry.sql` uses `ALTER TABLE … ADD COLUMN IF NOT EXISTS` for every new column.
Both files are embedded with `//go:embed` and executed sequentially in `(*DB).Migrate()` during startup. No migration tracking table is needed.
