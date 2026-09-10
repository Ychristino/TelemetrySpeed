# Telemetry Engine — Database Architecture

## 1. Goal

The application has **two independent modules**:

1. **Listener** — receives telemetry over UDP and writes it to the database.
2. **Consumer** — exposes an API and reads telemetry from the database.

Both modules use the **same database technology and configuration**, but they should not be able to accidentally mix responsibilities.

The desired architecture is:

```text
                    ┌─────────────────────┐
                    │      InfluxDB       │
                    └──────────┬──────────┘
                               │
                       shared DB client
                               │
                 ┌─────────────┴─────────────┐
                 │                           │
          ┌──────▼──────┐             ┌──────▼──────┐
          │    Writer   │             │    Reader   │
          └──────┬──────┘             └──────┬──────┘
                 │                           │
          ┌──────▼──────┐             ┌──────▼──────┐
          │   Listener  │             │   Consumer  │
          │    (UDP)    │             │    (API)    │
          └─────────────┘             └─────────────┘
```

The important rule is:

> **Listener gets a Writer. Consumer gets a Reader. Neither gets direct access to the database client.**

---

# 2. Why separate Reader and Writer?

Imagine the Listener receives a packet:

```text
UDP packet
    │
    ▼
┌───────────┐
│ Listener  │
└─────┬─────┘
      │
      │ telemetry
      ▼
┌───────────┐
│  Writer   │
└─────┬─────┘
      │
      ▼
  InfluxDB
```

The Listener only needs to write.

It does **not** need:

```text
Query()
Read()
Search()
GetTelemetry()
```

On the other side:

```text
User
 │
 │ HTTP request
 ▼
┌───────────┐
│ Consumer  │
└─────┬─────┘
      │
      ▼
┌───────────┐
│  Reader   │
└─────┬─────┘
      │
      ▼
  InfluxDB
```

The Consumer only needs to read.

It does **not** need:

```text
WritePoint()
WriteBatch()
Insert()
```

This makes the responsibilities clear.

---

# 3. Project structure

A good starting structure is:

```text
NewTelemetryEngine/
│
├── cmd/
│   │
│   ├── listener/
│   │   └── main.go
│   │
│   └── consumer/
│       └── main.go
│
├── internal/
│   │
│   ├── config/
│   │   └── config.go
│   │
│   ├── database/
│   │   ├── influx.go
│   │   ├── reader.go
│   │   └── writer.go
│   │
│   ├── telemetry/
│   │   └── telemetry.go
│   │
│   ├── listener/
│   │   └── listener.go
│   │
│   └── consumer/
│       └── consumer.go
│
├── go.mod
└── .env
```

There are two executable applications:

```text
cmd/listener
       │
       └── package main


cmd/consumer
       │
       └── package main
```

They share code from `internal/`.

---

# 4. The database layer

The database layer owns the InfluxDB client.

```text
                    DATABASE PACKAGE
┌────────────────────────────────────────────────┐
│                                                │
│                ┌──────────────┐                │
│                │   InfluxDB   │                │
│                │    Client    │                │
│                └──────┬───────┘                │
│                       │                        │
│              ┌────────┴────────┐               │
│              │                 │               │
│       ┌──────▼──────┐   ┌──────▼──────┐        │
│       │    Writer   │   │    Reader   │        │
│       └─────────────┘   └─────────────┘        │
│                                                │
└────────────────────────────────────────────────┘
```

The application should not directly access:

```go
influxdb2.Client
```

Instead, the database package controls it.

---

# 5. Database connection ownership

The `InfluxDB` object owns the actual client:

```go
type InfluxDB struct {
    client       influxdb2.Client
    organization string
    bucket       string
}
```

The client is private:

```go
client influxdb2.Client
```

not:

```go
Client influxdb2.Client
```

This is important.

It means code outside the database package cannot simply do:

```go
db.client.QueryAPI(...)
```

or:

```go
db.client.WriteAPI(...)
```

The database package controls how the client is used.

---

# 6. Creating the database

Conceptually:

```go
func NewInfluxDB(cfg *config.Config) *InfluxDB {
    client := influxdb2.NewClient(
        cfg.DatabaseURL,
        cfg.DataBaseToken,
    )

    return &InfluxDB{
        client:       client,
        organization: cfg.DatabaseOrganization,
        bucket:       cfg.DataBaseBucket,
    }
}
```

Now the application has:

```text
             NewInfluxDB()
                   │
                   ▼
          ┌─────────────────┐
          │    InfluxDB     │
          │                 │
          │ client          │
          │ organization    │
          │ bucket          │
          └─────────────────┘
```

---

# 7. Creating the Writer

The database object can expose a Writer:

```go
writer := db.Writer()
```

Conceptually:

```text
             InfluxDB
                 │
                 │
                 ▼
             Writer()
                 │
                 ▼
          ┌─────────────┐
          │   Writer    │
          └─────────────┘
```

The Writer internally knows how to use InfluxDB.

The Listener does not need to know.

---

# 8. Creating the Reader

Likewise:

```go
reader := db.Reader()
```

Conceptually:

```text
             InfluxDB
                 │
                 │
                 ▼
             Reader()
                 │
                 ▼
          ┌─────────────┐
          │   Reader    │
          └─────────────┘
```

The Consumer receives the Reader.

---

# 9. Listener flow

The Listener application should look like this:

```text
                     LISTENER APPLICATION

 UDP packet
     │
     ▼
┌─────────────┐
│ UDP Receiver│
└──────┬──────┘
       │
       ▼
┌─────────────┐
│   Decoder   │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│  Telemetry  │
│    Model    │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│    Writer   │
└──────┬──────┘
       │
       ▼
   InfluxDB
```

The Listener should only receive a Writer:

```go
writer := db.Writer()

listener := listener.New(writer)
```

The Listener doesn't need:

```go
db.client
```

and doesn't need a Reader.

---

# 10. Consumer flow

The Consumer application is the opposite:

```text
                      CONSUMER APPLICATION

 User
  │
  │ HTTP request
  ▼
┌─────────────┐
│ HTTP Server │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│   Handler   │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│   Service   │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│    Reader   │
└──────┬──────┘
       │
       ▼
   InfluxDB
       │
       ▼
   Telemetry
       │
       ▼
   HTTP JSON
       │
       ▼
      User
```

The Consumer gets only a Reader:

```go
reader := db.Reader()

consumer := consumer.New(reader)
```

---

# 11. The most important boundary

The architecture should enforce this:

```text
                  ┌─────────────────┐
                  │     InfluxDB    │
                  └────────┬────────┘
                           │
              ┌────────────┴────────────┐
              │                         │
              ▼                         ▼
        ┌───────────┐             ┌───────────┐
        │  Writer   │             │  Reader   │
        └─────┬─────┘             └─────┬─────┘
              │                         │
              ▼                         ▼
         ┌──────────┐              ┌──────────┐
         │ Listener │              │ Consumer │
         └──────────┘              └──────────┘
```

Not this:

```text
                         BAD

       ┌──────────────┐
       │   Listener   │
       └──────┬───────┘
              │
              │ direct access
              ▼
       ┌──────────────┐
       │  InfluxDB    │
       │    Client    │
       └──────┬───────┘
              ▲
              │ direct access
              │
       ┌──────┴───────┐
       │   Consumer   │
       └──────────────┘
```

The second design couples your application directly to InfluxDB.

---

# 12. Interfaces

Now we get to an important Go concept.

Instead of making Listener depend on a concrete Writer implementation, define an interface:

```go
type TelemetryWriter interface {
    WriteTelemetry(
        ctx context.Context,
        telemetry Telemetry,
    ) error
}
```

And for reading:

```go
type TelemetryReader interface {
    GetTelemetry(
        ctx context.Context,
        deviceID string,
    ) ([]Telemetry, error)
}
```

Now Listener only knows:

```text
"I have something capable of writing telemetry."
```

It does not know:

```text
"That thing is InfluxDB."
```

Similarly, Consumer only knows:

```text
"I have something capable of reading telemetry."
```

---

# 13. Why this is useful

The Listener can be:

```go
type Listener struct {
    writer TelemetryWriter
}
```

The Consumer can be:

```go
type Consumer struct {
    reader TelemetryReader
}
```

This means:

```text
Listener
   │
   │ depends on
   ▼
TelemetryWriter
```

and:

```text
Consumer
   │
   │ depends on
   ▼
TelemetryReader
```

Neither depends directly on InfluxDB.

---

# 14. InfluxDB becomes an implementation

The InfluxDB code implements those interfaces:

```text
                 TelemetryWriter
                       ▲
                       │ implements
                       │
                 InfluxWriter


                 TelemetryReader
                       ▲
                       │ implements
                       │
                 InfluxReader
```

So the complete architecture becomes:

```text
                    APPLICATION
─────────────────────────────────────────────

     Listener                       Consumer
        │                              │
        ▼                              ▼
TelemetryWriter                 TelemetryReader
        │                              │
        └──────────────┬───────────────┘
                       │
───────────────────────┼───────────────────────
                       │
                  DATABASE LAYER
                       │
                       ▼
                 InfluxDB Client
                       │
                       ▼
                    InfluxDB
```

---

# 15. Your two `main.go` files

## Listener

```go
func main() {
    cfg := config.Load()

    db := database.NewInfluxDB(cfg)
    defer db.Close()

    writer := db.Writer()

    app := listener.New(writer)

    if err := app.Start(); err != nil {
        log.Fatal(err)
    }
}
```

Flow:

```text
config
  │
  ▼
database
  │
  ▼
writer
  │
  ▼
listener
  │
  ▼
UDP
```

---

## Consumer

```go
func main() {
    cfg := config.Load()

    db := database.NewInfluxDB(cfg)
    defer db.Close()

    reader := db.Reader()

    app := consumer.New(reader)

    if err := app.Start(); err != nil {
        log.Fatal(err)
    }
}
```

Flow:

```text
config
  │
  ▼
database
  │
  ▼
reader
  │
  ▼
consumer
  │
  ▼
HTTP API
```

---

# 16. The shared client

Both applications can use the same database package, but remember:

> "Same connection" does not necessarily mean the Listener process and Consumer process literally share one TCP connection.

If Listener and Consumer are separate executables/processes, each process creates its own InfluxDB client.

That is completely normal.

You get:

```text
                 InfluxDB Server
                       │
             ┌─────────┴─────────┐
             │                   │
             ▼                   ▼
       Listener process     Consumer process
             │                   │
       Influx client        Influx client
             │                   │
          Writer               Reader
```

Within each process, however, you should avoid creating a new InfluxDB client for every operation.

---

# 17. Do not over-abstract too early

A good first version does not need 20 interfaces.

Start with:

```text
database/
├── influx.go
├── writer.go
└── reader.go
```

And:

```text
Listener → Writer → InfluxDB
Consumer → Reader → InfluxDB
```

Once you understand the application/domain models, you can make the interfaces more specific.

For example:

```go
WriteTelemetry(...)
GetTelemetry(...)
GetLatestTelemetry(...)
GetDeviceHistory(...)
```

is usually better than exposing generic database operations such as:

```go
ExecuteQuery(...)
WritePoint(...)
```

because the first approach keeps InfluxDB details inside the database layer.

---

# 18. Final architecture

This is the architecture I would aim for:

```text
                         ┌───────────────────┐
                         │     InfluxDB      │
                         └─────────┬─────────┘
                                   │
                         ┌─────────▼─────────┐
                         │ InfluxDB Client   │
                         └─────────┬─────────┘
                                   │
                   ┌───────────────┴───────────────┐
                   │                               │
            ┌──────▼──────┐                 ┌──────▼──────┐
            │   Writer    │                 │   Reader    │
            └──────┬──────┘                 └──────┬──────┘
                   │                               │
            TelemetryWriter                 TelemetryReader
                   │                               │
                   ▼                               ▼
            ┌─────────────┐                 ┌─────────────┐
            │  Listener   │                 │  Consumer   │
            │             │                 │             │
            │ UDP Server  │                 │ HTTP API    │
            └──────┬──────┘                 └──────┬──────┘
                   │                               │
                   ▼                               ▼
              UDP Devices                         Users
```

The key rule to remember is:

> **The database package owns InfluxDB. The Listener owns ingestion. The Consumer owns the API. Interfaces connect them without exposing InfluxDB details.**

This gives you a clean separation while still allowing both applications to use the same database implementation.


Bucket: Session

Measurement: session
Tags:
    game
    user_id
    track
    car
    session_id

Fields:
    ...

Measurement: lap
Tags:
    game
    session_id
    lap_number

Fields:
    ...

Measurement: sector
Tags:
    game
    session_id
    lap_number
    sector

Fields:
    ...

Bucket: Telemetry

Measurement: vehicle
Tags: 
    game
    session_id

Fields:
    speed
    throttle
    brake
    steering
    gear
    fuel
    position_x
    position_y
    position_z

Measurement: engine
Tags: 
    game
    session_id

Fields:
    rpm
    temperature
    oil_pressure
    fuel