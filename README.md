# gogs

A distributed game server framework built on the [bigTree](https://github.com/gogu-x/bigTree) Actor model.

## Architecture

```
Client (WebSocket)
        │
        ▼
┌──────────────────────────────┐
│          Gate Process        │
│  GateServer (:8080)          │
│  ConnActor  ──→  GateRouter  │
│  StreamActor (per game node) │
└──────────┬───────────────────┘
           │ gRPC Bidirectional Stream
           ▼
┌──────────────────────────────┐
│          Game Process        │
│  GateActor (gRPC :900x)      │
│  GameActor → Router → Service│
│  App (Players/Guild/Activity)│
└──────────────────────────────┘
           │
           ▼
    etcd  MongoDB  NATS
```

**Gate** handles WebSocket connections and routes messages to the correct **Game** process via gRPC bidirectional stream. Game processes register themselves in etcd; Gate discovers them dynamically.

## Quick Start

```bash
# Start all services
docker-compose up --build

# Or run locally
./gate
./game --server-id=1
./game --server-id=2
```

## Project Structure

```
gogs/
├── gate/               # Gate process (WebSocket → gRPC)
│   ├── main.go
│   └── server/
│       ├── conn_actor.go      # Per-connection actor
│       ├── stream_actor.go    # Per-game-node gRPC stream
│       ├── registry_actor.go  # etcd watcher, manages StreamActors
│       └── gate_router.go     # Gate-layer message handlers (login etc.)
├── game/               # Game process (business logic)
│   ├── main.go
│   ├── model/
│   │   ├── game.go    # GameActor — main message dispatcher
│   │   └── gate.go    # GateActor — gRPC server
│   ├── app/           # App context + dependency container
│   ├── router/        # Message → handler registration
│   ├── ctl/           # Business logic handlers
│   └── player/        # In-memory player manager
├── codec/             # Protobuf + JSON dual codec
├── cluster/           # etcd service registration & discovery
├── config/            # Config with env var support
├── pb/                # Generated protobuf code
├── Dockerfile.gate
├── Dockerfile.game
└── docker-compose.yml
```

## Message Flow

**Client → Server:**
```
WS frame → ConnActor → codec.Unmarshal
         → GateRouter (login/heartbeat handled in gate)
         → fallback: gateway.Frame → StreamActor → gRPC → GateActor → GameActor → Service
```

**Server → Client:**
```
Service → ctx.Reply → GateActor → gRPC stream → StreamActor → ConnActor(by connID) → WS
```

## Adding a New Message Handler

**1. Define the message in `protobuf/game.proto`:**
```protobuf
message MoveReq {
  uint64 uid = 1;
  float  x   = 2;
  float  y   = 3;
}
```

**2. Regenerate code:**
```bash
make proto
```

**3. Add handler in `game/ctl/`:**
```go
func MoveService(ctx *app.Context, msg interface{}) {
    req := msg.(*pb.MoveReq)
    // business logic
    ctx.Reply(&pb.MoveResp{})
}
```

**4. Register in `game/router/handler.go`:**
```go
r.Register(&pb.MoveReq{}, a.Handle(ctl.MoveService))
```

## Codec

Two codecs share the same message registry:

| Codec | Wire format | Message ID |
|-------|-------------|------------|
| ProtoCodec | `[2-byte msgID][protobuf body]` | FNV-32a hash of type name |
| JsonCodec | `{"TypeName": {...}}` | type name lookup |

Select codec via WebSocket subprotocol:
- `Sec-WebSocket-Protocol: protobuf` → ProtoCodec (default)
- `Sec-WebSocket-Protocol: json` → JsonCodec

## Configuration

All config values support environment variable overrides:

| Env Var | Default | Description |
|---------|---------|-------------|
| `ETCD_ENDPOINTS` | `localhost:2379` | etcd addresses (comma-separated) |
| `GATE_LISTEN_ADDR` | `:8080` | WebSocket listen address |
| `GRPC_HOST` | `127.0.0.1` | This game node's advertised host |
| `MONGO_URL` | `mongodb://localhost:27017` | MongoDB connection URL |
| `NATS_URL` | `nats://localhost:4222` | NATS connection URL |

## Infrastructure

| Service | Image | Port |
|---------|-------|------|
| etcd | bitnami/etcd:3.5 | 2379 |
| MongoDB | mongo:7 | 27017 |
| NATS (JetStream) | nats:2.10-alpine | 4222 / 8222 |

## Dependencies

- [github.com/gogu-x/bigTree](https://github.com/gogu-x/bigTree) — Actor model, timer, log
- [gorilla/websocket](https://github.com/gorilla/websocket)
- [grpc/grpc-go](https://github.com/grpc/grpc-go)
- [etcd/client/v3](https://github.com/etcd-io/etcd)
- [urfave/cli/v3](https://github.com/urfave/cli)

## License

Apache 2.0
