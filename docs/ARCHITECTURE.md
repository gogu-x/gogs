# gogs 架构文档

## 整体架构

```
┌──────────────────────────────────────────────────────────────┐
│  Client                                                       │
│  WebSocket (protobuf / json)                                  │
└───────────────────────────┬──────────────────────────────────┘
                            │
                            ▼
┌──────────────────────────────────────────────────────────────┐
│  Gate 进程（可水平扩展，多个 gate-id）                          │
│                                                               │
│  WsServer(:808x) ──upgrade──► ConnActor(每连接一个)            │
│                                    │                          │
│                               NATS Publish                    │
│                          game:{serverID}:{nodeID}             │
│                                                               │
│  etcd watch ──► UpdateNodes ──► HashPick(fnv(uid)%nodeCount)  │
│                      └──► NodeFailoverMsg ──► ConnActor       │
└───────────────────────────┬──────────────────────────────────┘
                            │ NATS
                            ▼
┌──────────────────────────────────────────────────────────────┐
│  Game 进程（server-id + node-id，同 server-id 可多实例）        │
│                                                               │
│  NatsActor(subscribe game:{serverID}:{nodeID})                │
│       │                                                       │
│       ├──► PlayerActor(uid) ──► Router ──► Ctl               │
│       │         └── Session.Reply ──► NATS gate.out.{gateId}  │
│       ├──► GuildActor                                         │
│       └──► ActivityActor                                      │
│                                                               │
│  etcd 注册：/game/server/{serverID}/{nodeID} = grpcAddr        │
└───────────────────────────┬──────────────────────────────────┘
                            │
               ┌────────────┼────────────┐
               ▼            ▼            ▼
             etcd        MongoDB        NATS
┌──────────────────────────────────────────────────────────────┐
│  Platform 进程                                                 │
│  gRPC Server(:7000) ──► AuthService / OrderService            │
│       └── MongoDB(platform db)                                │
└──────────────────────────────────────────────────────────────┘
```

---

## 进程说明

### Gate

负责 WebSocket 接入，无业务状态，可任意水平扩展。

| Actor | 职责 |
|-------|------|
| `WsServer` | HTTP 升级 WebSocket，为每个连接 Spawn `ConnActor` |
| `ConnActor` | 维护单个 WS 连接的生命周期，鉴权、编解码、forward 到 Game |
| `NatsActor` | 订阅 `gate.out.{gateID}`，将 Game 回包转发给对应 `ConnActor` |

**启动参数：**
```bash
./gate --gate-id=1   # 监听 :8081（8080 + gate-id）
./gate --gate-id=2   # 监听 :8082
```

---

### Game

有状态业务节点，每个玩家的数据常驻内存。

| Actor | 职责 |
|-------|------|
| `NatsActor` | 订阅 `game:{serverID}:{nodeID}`，收到 Frame 后路由给 `PlayerActor`（不存在则 Spawn） |
| `PlayerActor` | 每个在线玩家独立 Actor，持有完整玩家数据，串行处理所有消息 |
| `GuildActor` | 工会逻辑，全局单例 |
| `ActivityActor` | 活动逻辑，全局单例 |
| `MongoActor` | MongoDB 操作序列化，fire-and-forget 写入 |

**启动参数：**
```bash
./game --server-id=1 --node-id=1 --port=9001
./game --server-id=1 --node-id=2 --port=9002
```

同一 `server-id` 的多个节点共享同一个 MongoDB 库（`game_{serverID}`）。

---

### Platform

账号/鉴权/订单服务，通过 gRPC 对 Gate 提供服务。

| Actor | 职责 |
|-------|------|
| `PlatformGrpcActor` | gRPC Server，处理注册/登录/订单请求 |
| `WebhookActor` | HTTP Webhook，处理支付回调 |

---

## 路由机制

### 登录时节点选定

Gate 在登录成功后通过 `cluster.HashPick` 选定目标 Game 节点，并将 `nodeID` 写入 `ConnActor` 状态，后续所有消息固定发往该节点：

```
nodeID = nodes[ fnv32(uid) % len(nodes) ]
```

- 节点列表不变 → 相同 uid 始终路由到同一节点
- 扩容新节点 → 只接收新登录用户，存量用户不受影响

### 节点故障自动切换

```
Game 节点下线
    │
    ▼
etcd delete 事件
    │
    ├── cluster.UpdateNodes(serverID, 剩余节点)  // 重建路由表
    │
    └── NodeFailoverMsg → WsServer → 广播所有 ConnActor
                                          │
                                          ▼
                              if connActor.nodeID == deadNode:
                                  nodeID = HashPick(serverID, uid)
                                  // 后续消息自动路由到新节点，用户无感
```

---

## 消息流

### Client → Server

```
WS frame
  │
  ▼
ConnActor.handleWsMsg
  ├── codec.Unmarshal
  ├── middleware（鉴权）
  └── router.Route
        ├── LoginReq → onLogin（Platform gRPC 鉴权 → 选节点）
        └── 其他消息 → forward
                          │
                          ▼
                    NATS Publish
                    game:{serverID}:{nodeID}
                          │
                          ▼
                    Game NatsActor
                          │
                          ▼
                    PlayerActor.HandleMessage
                          │
                          ▼
                    Router → Ctl 业务逻辑
```

### Server → Client

```
Ctl 业务逻辑
  │
  ▼
Session.Reply(proto.Message)
  │
  ▼
codec.Marshal + NATS Publish
gate.out.{gateID}
  │
  ▼
Gate NatsActor
  │
  ▼
ConnActor → ws.WriteMessage
  │
  ▼
Client
```

---

## NATS Subject 规范

| Subject | 方向 | 说明 |
|---------|------|------|
| `game:{serverID}:{nodeID}` | Gate → Game | 玩家消息，按 server+node 路由 |
| `gate.out.{gateID}` | Game → Gate | 回包，按 gateID 路由 |
| `game.shutdown.{serverID}.{nodeID}` | 内部 | 节点关闭信号 |
| `platform.deliver.{serverID}` | Platform → Game | 平台下发（充值等） |

---

## PlayerActor 生命周期

```
NatsActor 收到 Frame（uid 未登录）
    │
    ▼
Spawn PlayerActor(uid, connID)
    │
    ▼
OnInit:
  1. MongoDB 同步加载玩家数据（最长等 5s）
  2. 初始化 Session、Router、定时存档
    │
    ▼
HandleMessage: 串行处理所有消息（线程安全）
    │
    ▼
OnStop（WS 断开 / 节点关闭）:
  PlayerData.Save() → MongoDB upsert
```

**定时存档：** 每隔 1 分钟触发一次 `PlayerData.Save()`，通过 `ActorSystem` 共享 `TimeWheel` 调度，不阻塞消息处理。

---

## Actor 框架要点（bigTree）

| 特性 | 说明 |
|------|------|
| 每个 Actor 独立 goroutine | 消息串行处理，业务代码无需加锁 |
| Mailbox | 带缓冲 channel，默认大小可通过 `WithMailboxSize` 配置 |
| 共享 TimeWheel | 整个 ActorSystem 共用一个时间轮，避免每个 Actor 独立创建 goroutine |
| Request/Future | 跨 Actor 请求响应，回调在发起方 goroutine 执行 |
| 系统消息优先 | Stop 信号走独立 channel，优先于用户消息处理 |

**ConnActor mailbox 大小：** 连接型 Actor 消息速率低，使用 `WithMailboxSize(64)` 避免默认 6048 导致的内存浪费（2万连接节省 ~1.8GB）。

---

## 目录结构

```
gogs/
├── gate/
│   ├── main.go
│   ├── conn/
│   │   ├── actor.go        # ConnActor 结构、生命周期
│   │   ├── handler.go      # handleWsMsg、forward、NodeFailover
│   │   ├── service.go      # onLogin、onRegister，HashPick 选节点
│   │   ├── middleware.go   # checkAuth 鉴权中间件
│   │   └── router.go       # 消息路由注册
│   ├── wsserver/
│   │   ├── server.go       # HTTP/WS 服务，Spawn ConnActor
│   │   └── router.go       # ConnReg/Unreg/Broadcast/NodeFailover
│   └── nats/
│       └── actor.go        # 订阅 gate.out.{gateID}，派发给 ConnActor
├── game/
│   ├── main.go
│   ├── gate/
│   │   └── external.go     # NewNatsActor，订阅 game:{serverID}:{nodeID}
│   ├── player/
│   │   ├── external.go     # PlayerActor 入口
│   │   ├── internal/base/
│   │   │   ├── player.go   # PlayerData、Load、Save
│   │   │   ├── session.go  # Session、Reply、AfterFunc
│   │   │   └── timer.go    # scheduleSave 定时存档
│   │   └── internal/
│   │       ├── router.go   # 注册所有消息处理器
│   │       └── ctl_*.go    # 各业务模块（auth/chat/guild/activity）
│   ├── guild/
│   └── activity/
├── platform/
│   ├── main.go
│   ├── grpc/               # gRPC Server（auth/order）
│   ├── service/            # 业务逻辑
│   ├── store/              # MongoDB 操作
│   ├── auth/               # JWT
│   └── webhook/            # 支付回调
├── cluster/
│   ├── cluster.go          # UpdateNodes、HashPick、Register
│   ├── hash.go             # fnv hash 取模
│   ├── etcd.go             # etcd client 初始化
│   └── event.go            # GetInstances、WatchInstances
├── natsrpc/
│   ├── nats.go             # NatsActor（订阅/发送统一入口）
│   ├── publish.go          # subject 命名规范、publish
│   ├── messages.go         # SendMsg、SubConfig
│   └── constant.go         # 模块常量
├── codec/
│   ├── proto.go            # ProtoCodec（FNV-32a msgID + protobuf body）
│   └── json.go             # JsonCodec（TypeName + JSON body）
├── rpc/
│   ├── platform/           # Platform gRPC 客户端 Actor
│   └── mongo/              # MongoDB Actor（序列化所有 DB 操作）
├── config/                 # 配置，全部支持环境变量覆盖
├── pb/                     # protobuf 生成代码
├── protocol/               # .proto 源文件
└── bigTree/                # Actor 框架（git submodule）
```

---

## 配置参考

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `ETCD_ENDPOINTS` | `localhost:2379` | etcd 地址（逗号分隔） |
| `GRPC_HOST` | `127.0.0.1` | Game 节点对外 host（容器部署需改） |
| `MONGO_URL` | `mongodb://localhost:27017` | MongoDB |
| `NATS_URL` | `nats://localhost:4222` | NATS |
| `PLATFORM_ADDR` | `:7000` | Platform gRPC 监听 |
| `PLATFORM_GRPC_ADDR` | `127.0.0.1:7000` | Platform gRPC 连接地址 |
| `JWT_SECRET` | `changeme-secret` | JWT 密钥（生产必须修改） |

---

## 基础设施

| 服务 | 镜像 | 端口 | 用途 |
|------|------|------|------|
| etcd | bitnami/etcd:3.5 | 2379 | Game 节点注册与发现 |
| MongoDB | mongo:7 | 27017 | 玩家数据、平台账号 |
| NATS | nats:2.10-alpine | 4222 / 8222 | Gate↔Game 消息总线 |
