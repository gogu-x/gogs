# gogs

基于 [bigTree](https://github.com/gogu-x/bigTree) Actor 模型构建的分布式游戏服务器框架。

## 架构

```
Client (WebSocket)
        │
        ▼
┌─────────────────────────────────┐
│           Gate 进程              │
│  WsServer (:808x)               │
│  ConnActor  ──→  NATS           │
└──────────────┬──────────────────┘
               │ NATS (game:{serverID}:{nodeID})
               ▼
┌─────────────────────────────────┐
│           Game 进程              │
│  NatsActor                      │
│  PlayerActor → Router → Service │
│  GuildActor / ActivityActor     │
└─────────────────────────────────┘
               │
               ▼
        etcd  MongoDB  NATS
```

**Gate** 处理 WebSocket 连接，通过 NATS 将消息路由到对应的 **Game** 节点。

**Game** 节点向 etcd 注册，Gate 启动时加载全部节点并监听变化。同一 `server-id` 可以部署多个 `node-id` 实例，Gate 使用 `fnv(uid) % nodeCount` 将用户固定路由到某个节点。节点下线时，Gate 自动将受影响的在线用户无感切换到存活节点。

## 快速启动

```bash
# 启动基础设施
docker-compose up -d

# 启动 platform
./platform

# 启动 gate（--gate-id 决定监听端口，实际端口 = 8080 + gate-id）
./gate --gate-id=1   # 监听 :8081

# 启动 game（同一 server-id 可以启动多个 node-id 实现水平扩展）
./game --server-id=1 --node-id=1 --port=9001
./game --server-id=1 --node-id=2 --port=9002
```

## 目录结构

```
gogs/
├── gate/                   # Gate 进程（WebSocket 接入层）
│   ├── main.go
│   ├── conn/               # ConnActor：每个连接一个 Actor
│   │   ├── actor.go        # 状态、生命周期
│   │   ├── handler.go      # 消息处理、forward、节点故障切换
│   │   ├── service.go      # 登录/注册，hash 选节点
│   │   ├── middleware.go   # 鉴权中间件
│   │   └── router.go       # 消息路由注册
│   ├── wsserver/           # WsServer Actor，管理所有连接，广播
│   └── nats/               # Gate 侧 NATS 订阅（接收 game 回包）
├── game/                   # Game 进程（业务逻辑层）
│   ├── main.go
│   ├── gate/               # NatsActor：接收 Gate 消息，派发给 PlayerActor
│   ├── player/             # PlayerActor：每个在线玩家独立 Actor
│   │   ├── external.go
│   │   ├── internal/base/  # Session、PlayerData、定时存档
│   │   └── internal/       # 各业务 ctl（auth/chat/guild/activity）
│   ├── guild/              # GuildActor
│   └── activity/           # ActivityActor
├── platform/               # 平台服（账号/鉴权/订单）gRPC
├── cluster/                # etcd 服务注册与发现 + hash 路由
│   ├── cluster.go          # UpdateNodes / HashPick
│   ├── hash.go             # fnv hash 取模路由
│   └── event.go            # WatchInstances
├── natsrpc/                # NATS 收发封装
├── codec/                  # Protobuf / JSON 双 codec
├── config/                 # 配置（支持环境变量覆盖）
├── pb/                     # 生成的 protobuf 代码
└── bigTree/                # Actor 框架（submodule）
```

## 消息流

**Client → Server：**
```
WS frame → ConnActor.forward
         → NATS game:{serverID}:{nodeID}
         → Game NatsActor
         → PlayerActor → Service
```

**Server → Client：**
```
Service → Session.Reply
        → NATS gate.out.{gateID}
        → Gate NatsActor
        → ConnActor → WS
```

## 路由策略

同一 `server-id` 下有多个 `node-id` 时，Gate 在登录时选定节点：

```
nodeID = nodes[ fnv32(uid) % len(nodes) ]
```

- 节点列表不变 → 相同 uid 永远路由到同一节点
- 节点下线 → etcd delete 事件触发，Gate 重建路由表，受影响用户自动切换到存活节点

## 添加新消息处理器

**1. 定义 proto：**
```protobuf
// protocol/gateway/game_req.proto
message MoveReq {
  float x = 1;
  float y = 2;
}
```

**2. 生成代码：**
```bash
make proto
```

**3. 在 `game/player/internal/` 添加 ctl：**
```go
func Move(s *base.Session, msg interface{}) {
    req := msg.(*pb.MoveReq)
    // 业务逻辑
    s.Reply(&pb.MoveAck{})
}
```

**4. 在 `game/player/internal/router.go` 注册：**
```go
r.Register(&pb.MoveReq{}, s.Handle(Move))
```

## Codec

| Codec | 格式 | 消息 ID |
|-------|------|---------|
| ProtoCodec | `[2-byte msgID][protobuf body]` | FNV-32a hash of type name |
| JsonCodec | `{"TypeName": {...}}` | type name lookup |

WebSocket 子协议选择：
- `Sec-WebSocket-Protocol: protobuf` → ProtoCodec（默认）
- `Sec-WebSocket-Protocol: json` → JsonCodec

## 配置

所有配置项均支持环境变量覆盖：

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `ETCD_ENDPOINTS` | `localhost:2379` | etcd 地址（逗号分隔） |
| `GRPC_HOST` | `127.0.0.1` | Game 节点对外暴露的 host |
| `MONGO_URL` | `mongodb://localhost:27017` | MongoDB 连接地址 |
| `NATS_URL` | `nats://localhost:4222` | NATS 连接地址 |
| `PLATFORM_ADDR` | `:7000` | Platform gRPC 监听地址 |
| `PLATFORM_GRPC_ADDR` | `127.0.0.1:7000` | Platform gRPC 连接地址 |
| `JWT_SECRET` | `changeme-secret` | JWT 签名密钥 |

Gate 监听端口 = `8080 + gate-id`，Game gRPC 端口 = `9000 + server-id`（可用 `--port` 覆盖）。

## 基础设施

| 服务 | 镜像 | 端口 |
|------|------|------|
| etcd | bitnami/etcd:3.5 | 2379 |
| MongoDB | mongo:7 | 27017 |
| NATS (JetStream) | nats:2.10-alpine | 4222 / 8222 |

## 依赖

- [github.com/gogu-x/bigTree](https://github.com/gogu-x/bigTree) — Actor 模型、定时器、日志
- [gorilla/websocket](https://github.com/gorilla/websocket)
- [nats-io/nats.go](https://github.com/nats-io/nats.go)
- [etcd/client/v3](https://github.com/etcd-io/etcd)
- [mongo-driver/v2](https://github.com/mongodb/mongo-go-driver)
- [urfave/cli/v3](https://github.com/urfave/cli)

## License

Apache 2.0
