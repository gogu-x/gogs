# gameServer 架构分析文档

> 生成时间：2026-05-17

---

## 一、项目概述

本项目是基于自研 **Actor 框架**（`goAcotr/actor`）构建的分布式游戏服务器，采用 **Gate + Game 双进程**架构，通过 **etcd** 做服务注册发现，**gRPC 双向流**做进程间通信，客户端通过 **WebSocket** 接入。

模块路径：`goAcotr/gameServer`

---

## 二、整体架构

```
Client (WebSocket)
    │
    ▼
┌─────────────────────────────────────────────┐
│                  Gate 进程                   │
│                                             │
│  GateServer (HTTP/WS :8080)                 │
│      │ 每连接                               │
│      ▼                                      │
│  ConnActor ──→ GateRouter (登录/心跳)        │
│      │ 未命中路由                            │
│      ▼                                      │
│  StreamActor (每个 game 服一个)              │
│      │ gRPC 双向流                           │
└──────┼──────────────────────────────────────┘
       │
       ▼  (gRPC Bidirectional Stream)
┌─────────────────────────────────────────────┐
│                  Game 进程                   │
│                                             │
│  GateActor (gRPC Server :900x)              │
│      │                                      │
│      ▼                                      │
│  GameActor ──→ Router ──→ Service           │
│                    │                        │
│               App (Players/Guild/Activity)  │
└─────────────────────────────────────────────┘
       │
       ▼
   etcd (:2379)  服务注册/发现
```

---

## 三、模块详解

### 3.1 Gate 进程 (`gate/`)

| 组件 | 职责 |
|------|------|
| `GateServer` | HTTP 升级 WebSocket，根据子协议选择 codec（protobuf/json），为每个连接 Spawn `ConnActor` |
| `ConnActor` | 每连接一个 Actor，负责 WS 读写、消息解码、路由分发、回包 |
| `GateRouter` | 处理 gate 层消息（登录 `LoginReq`），验证 token，绑定 uid/serverID |
| `StreamActor` | 每个 game 服一个，维护唯一 gRPC 双向流，负责 gate→game 的消息转发 |
| `RegistryActor` | 监听 etcd `/game/server/` 变化，动态 Spawn/Stop `StreamActor` |

**消息流（客户端→服务端）：**
```
WS帧 → ConnActor.handleWsMsg → codec.Unmarshal
    → GateRouter.Route (登录等 gate 层消息)
    → fallback: 封装 gateway.Frame → StreamActor → gRPC stream → GateActor
```

**消息流（服务端→客户端）：**
```
GateActor → gRPC stream → StreamActor.Recv → ConnActor(by connID) → WS写回
```

---

### 3.2 Game 进程 (`game/`)

| 组件 | 职责 |
|------|------|
| `GateActor` | gRPC 服务端，接收 gate 的流消息，解码后投递给 `GameActor`，同时负责向 gate 回包 |
| `GameActor` | 游戏主 Actor，持有 `App` 和 `Router`，所有游戏消息在此单 goroutine 处理 |
| `App` | 聚合所有游戏模块（Players/Guild/Activity），作为依赖容器传递给 service |
| `Router` | 消息路由表，将 proto 消息类型映射到对应 service 函数 |
| `service/` | 具体业务逻辑（如 `ChatService`） |
| `player/Manager` | 玩家内存管理，无锁（单 goroutine 内使用） |

**启动参数：**
```bash
./game --server-id=1   # 监听 :9001
./game --server-id=2   # 监听 :9002
```

---

### 3.3 Codec (`codec/`)

支持两种编解码，共享同一套消息注册表：

| Codec | 格式 | 消息头 |
|-------|------|--------|
| `ProtoCodec` | Protobuf | `2字节 msgID (FNV hash)` + 消息体 |
| `JsonCodec` | JSON | `2字节 msgID` + JSON 消息体 |

消息 ID 由消息类型名的 FNV-32a hash 截断为 uint16，**存在极低概率碰撞风险**。

---

### 3.4 Cluster (`cluster/`)

基于 etcd v3，提供：
- `Register`：注册 game 节点，带 30s 租约自动续租
- `GetAddr`：按 serverID 查询 gRPC 地址
- `GetAll`：全量拉取所有节点
- `Watch`：监听节点上下线事件（返回 channel）

Key 格式：`/game/server/{serverID}` → gRPC 地址

---

### 3.5 NATS (`nats/`)

目前为**实验性代码**，包含 JetStream 初始化和基础 pub/sub 测试，尚未集成到主流程。

---

## 四、数据流总览

```
1. 客户端连接 WebSocket
2. GateServer 升级协议，Spawn ConnActor
3. 客户端发送 LoginReq → ConnActor → GateRouter 处理，绑定 uid + serverID
4. 客户端发送游戏消息 → ConnActor → 封装 gateway.Frame → StreamActor
5. StreamActor 通过 gRPC 流发送到对应 game 进程的 GateActor
6. GateActor 解码 → 投递给 GameActor
7. GameActor → Router → Service 处理业务
8. Service 通过 Context.Reply → GateActor → gRPC 流 → StreamActor → ConnActor → WS 回包
```

---

## 五、技术栈

| 层次 | 技术 |
|------|------|
| 并发模型 | 自研 Actor 框架（`goAcotr/actor`） |
| 客户端协议 | WebSocket（gorilla/websocket） |
| 进程间通信 | gRPC 双向流（protobuf） |
| 服务发现 | etcd v3 |
| 消息编码 | Protobuf / JSON（双 codec 共存） |
| CLI | urfave/cli v3 |
| 消息队列（实验） | NATS JetStream |
| 构建 | Makefile + protoc 代码生成 |

---

## 六、观点与建议

### ✅ 设计亮点

1. **Actor 模型彻底消除锁**：`GameActor` 单 goroutine 处理所有游戏逻辑，`player.Manager` 无需加锁，设计简洁正确。

2. **Gate/Game 进程分离**：gate 横向扩展不影响 game 逻辑，game 可按区服独立部署，职责清晰。

3. **动态服务发现**：`RegistryActor` 监听 etcd 事件自动管理 `StreamActor` 生命周期，gate 无需重启即可感知 game 节点变化。

4. **双 Codec 共存**：同一套消息注册表同时支持 protobuf 和 JSON，便于调试和客户端兼容。

5. **App 依赖容器 + Context 包装**：`app.Context` 将 `ActorContext` 和业务依赖合并，service 函数签名统一，扩展方便。

---

### ⚠️ 问题与风险

#### 1. gRPC 流单点问题（高风险）
`GateActor` 只保存一个 `stream` 字段，意味着每个 game 进程只能接受**一个 gate 进程**的连接。多 gate 实例时后连接会覆盖前者，导致消息丢失。

**建议**：用 `map[streamID]stream` 管理多个流，或改为每次请求独立 gRPC 调用。

#### 2. msgID 哈希碰撞
FNV-32a 截断为 uint16（65536 个槽），消息数量增多后碰撞概率上升，且碰撞时**静默失败**（解码到错误类型）。

**建议**：改为显式注册 ID（如 proto option 或配置文件），启动时检测碰撞并 panic。

#### 3. serverID 硬编码在 ConnActor
`conn_actor.go` 中 `c.serverID = "1"` 是硬编码，登录后的 serverID 赋值逻辑在 `gate_router.go` 中，但 fallback 里又强制覆盖为 `"1"`，导致路由逻辑矛盾。

**建议**：删除 fallback 中的硬编码，仅使用登录时绑定的 `serverID`。

#### 4. 回包路径依赖 connID 全局查找
game 回包时通过 `connID` 在 ActorSystem 全局 map 中查找 `ConnActor`，若连接已断开则静默丢弃。这是合理的，但**没有任何通知机制**告知 game 侧连接已断开，可能导致 game 持续处理已离线玩家的消息。

**建议**：连接断开时向 game 发送 `PlayerOffline` 事件。

#### 5. NATS 模块未集成
`nats/` 目录代码为测试代码（`main` 包），与主项目完全隔离，且包含大量注释代码。

**建议**：明确 NATS 的定位（替代 gRPC 流？跨服广播？），或清理掉避免混淆。

#### 6. Guild/Activity 模块为空
`guild/guild.go` 和 `activity/activity.go` 仅有包声明，`App.New()` 中也未初始化这两个 Manager。

**建议**：在 `App.New()` 中补全初始化，或移除未实现的字段避免误用。

#### 7. 配置硬编码
etcd 地址 `43.160.212.55:2379` 直接写在 `config.go` 中，生产环境应通过环境变量或配置文件注入。

---

### 🔧 短期改进优先级

| 优先级 | 问题 | 影响 |
|--------|------|------|
| P0 | `serverID` 硬编码覆盖 | 功能 Bug，所有消息都路由到 server 1 |
| P0 | gRPC 流单点 | 多 gate 部署时数据丢失 |
| P1 | 连接断开未通知 game | 内存泄漏 + 无效消息处理 |
| P1 | msgID 碰撞无检测 | 难以排查的消息解码错误 |
| P2 | 配置硬编码 | 部署灵活性差 |
| P3 | NATS/Guild/Activity 清理 | 代码可读性 |
