# gogs 优化建议

> 基于 2026-05-24 的代码快照整理。问题按 **严重 → 一般 → 锦上添花** 三级分类，每条都标注了文件位置、根因和建议改法。

## 目录

- [🔴 严重问题（会引发 bug 或线上事故）](#-严重问题会引发-bug-或线上事故)
- [🟡 一般问题（影响可维护性 / 性能 / 正确性边界）](#-一般问题影响可维护性--性能--正确性边界)
- [🟢 锦上添花（架构改进 / 工程实践）](#-锦上添花架构改进--工程实践)
- [优先级路线图](#优先级路线图)

---

## 🔴 严重问题（会引发 bug 或线上事故）

### 1. Codec 没有任何消息被注册，所有上行消息会失败

**位置**：`codec/codec.go`、`codec/proto.go`、`codec/json.go`

**现象**：`codec.RegisterMsg(...)` 是注册入口，但仓库里**没有任何调用方**。`protoChat.ChatReq`、`protoGateway.LoginReq` 等都没注册过。

**后果**：
- `ProtoCodec.Unmarshal` → `unknown msgID`
- `JsonCodec.Unmarshal` → `unknown message`
- 客户端发任何消息上来都会失败

**建议**：
- 在每个 proto 包加 `init()` 调用 `codec.RegisterMsg(...)`
- 或在 `main.go` 启动时统一调用一次
- `tools/gen-register.lua` 看起来就是干这个的，需要接入构建流程

---

### 2. `ConnActor.handleWsMsg` 每条消息都重设 fallback 并覆盖 serverID

**位置**：`gate/server/conn_actor.go`

```go
func (c *ConnActor) handleWsMsg(ctx actor.ActorContext, data []byte) {
    inner, err := c.codec.Unmarshal(data)
    ...
    c.router.Route(ctx, inner)             // 先 Route
    c.serverID = "1"                       // ← 硬编码！每次都覆盖登录设置
    c.router.SetFallback(func(...) {       // ← 每条消息分配一个新闭包
        ...
    })
}
```

**三个问题叠在一起**：
- `c.serverID = "1"` 把登录时设置的 `serverID` 直接擦掉，多区服永远路由到 server 1
- 每条消息重新 `SetFallback`，分配新闭包，GC 压力 + 行为不稳定
- 顺序错了：先 `Route` 再设 fallback，第一条消息走不到 fallback

**建议**：fallback 在 `OnInit` 里只设一次；`serverID` 只能由登录流程修改。

---

### 3. `ConnActor.router` 是零值，未初始化

**位置**：`gate/server/conn_actor.go`

```go
type ConnActor struct {
    ...
    router actor.Router  // 零值
}
```

`initGateRouter(c)` 直接调用 `c.router.Register(...)`。如果 `actor.Router` 是 interface 类型，会 nil panic；是 struct 则依赖其零值可用。

**建议**：在 `OnInit` 显式 `c.router = actor.NewRouter()`（按 bigTree 实际 API 调整）。

---

### 4. `GateActor.stream` 单字段保存，多客户端会被覆盖

**位置**：`game/model/gate.go`

```go
func (s *gatewayService) Stream(stream protoGateway.Gateway_StreamServer) error {
    s.actor.stream = stream   // ← 没有锁，且只能记一条
    ...
}
```

**问题**：
- HA 部署时多个 Gate 进程同时连接 game，后连的会**覆盖**前一条 stream
- `g.stream.Send(m)` 没加锁，gRPC stream 的 `Send` 不是并发安全的
- 反向消息只能发到最后一条 stream，其他 Gate 上的客户端收不到回包

**建议**：
- 用 `map[streamID]stream` 管理多条流
- 或改成"每条 stream 一个 sub-actor"，跟 Gate 侧的 `StreamActor` 对称
- Send 路径要么加锁，要么通过单一 actor 串行化

---

### 5. `StreamActor` 没有重连，节点抖动后永久失联

**位置**：`gate/server/stream_actor.go`

**现象**：
- gRPC stream `Recv` 出错后直接发 `stopMsg`，对应的 `StreamActor` 死掉
- 但 etcd 的 key 还在（lease 没过期），`RegistryActor` 不会重新拉起
- 结果：game 网络抖一下，gate 就再也连不上它，直到 etcd lease 过期

**建议**：
- `StreamActor` 自己带重试（指数退避 + 上限）
- 或 stop 之后通知 `RegistryActor` 重新 spawn
- 配合健康检查，连接失败累计后再触发 etcd 状态更新

---

### 6. `cluster.Register` 的 keepalive goroutine 永不退出

**位置**：`cluster/cluster.go`

```go
ch, err := Client.KeepAlive(context.Background(), lease.ID)
...
go func() {
    for range ch {
    }
}()
```

**问题**：
- `Close()` 时这个 goroutine 不会退出
- `KeepAlive` 失败时不会重新建立租约
- ctx 用了 `Background()`，无法取消

**建议**：
- 改用可取消的 ctx，`Close()` 时 cancel
- KeepAlive channel 关闭时打日志 + 自动重新 grant 租约
- 提供退出回调，让上层知道注册已失效

---

### 7. `ChatService` 的 `Printf` 格式串不匹配

**位置**：`game/ctl/chat.go`

```go
fmt.Printf("game server [%d] player %s says: %s\n", config.ServerID, req.Content)
```

3 个占位符，2 个参数。运行时输出 `%!s(MISSING)`，`go vet` 会告警。

**建议**：补全参数或精简占位符。同时引入 `go vet` 到 CI。

---

### 8. 登录路由实际上不生效

**位置**：`gate/server/gate_router.go` + `conn_actor.go`

登录 handler 设置了 `c.uid` 和 `c.serverID`，但因为 [问题 #2](#2-connactorhandlewsmsg-每条消息都重设-fallback-并覆盖-serverid) 中 `serverID = "1"` 的覆盖，登录的设置立刻被擦除。**整套登录路由是失效的**，只是当前测试没暴露。

**建议**：连同 #2 一起修。修完后加一个端到端的登录测试用例。

---

## 🟡 一般问题（影响可维护性 / 性能 / 正确性边界）

### 9. `protocol/` 和 `pb/` 同时存在，源与产物不清晰

**位置**：`protocol/`、`pb/`

- `protocol/gateway/gateway.proto` 是源文件
- `pb/protoGateway/*.pb.go` 是生成产物
- 但 `protocol/player/`、`protocol/chat/` 没有对应的 `pb/...`
- 反过来 `pb/protoChat` 又有 `chatReq.pb.go` 和 `chatAck.pb.go`

**建议**：
- proto 输出路径直接配成 `pb/`
- 命名风格统一（要么全 `protoXxx` 要么全 `xxx`）
- `makefile` 里加 `make proto` 一键生成

---

### 10. 配置硬编码了一个公网 IP

**位置**：`config/config.go`

```go
EtcdEndpoints = envSlice("ETCD_ENDPOINTS", "43.160.212.55:2379")
```

README 写的是 `localhost:2379`，但代码里是公网 IP 默认值。本地开发会失败、提交泄露也不好。

**建议**：改成 `127.0.0.1:2379`，README 与代码保持一致。

---

### 11. `App` 的依赖装配只装了 Players

**位置**：`game/app/app.go`

```go
func New() *App {
    return &App{
        Players: player.NewManager(),
        // Guild、Activity 都是 nil
    }
}
```

当前没 handler 用到，但一旦有 handler 取 `ctx.Guild.XXX` 就会 nil panic。

**建议**：
- 要么补全所有子模块
- 要么改成"按需懒加载"（带 sync.Once）
- 给 `App` 加单测确保所有字段非 nil

---

### 12. `appContext.go` 的双层 context 设计绕

**位置**：`game/app/appContext.go`

链路：`actor.ActorContext → gameContext (uid/connID) → app.Context (再包一遍)`

`app.Handle` 还要类型断言 `ctx.(*gameContext)`，一旦谁直接用 `ctx`（不走 `Handle`）就崩。

**建议**：
- 让 `GameActor` 路由时一次包成 `app.Context`
- 或 `gameContext` 实现一个明确的 interface，避免 type assertion

---

### 13. Reply 路径 codec 不对称

**位置**：`game/app/appContext.go` + `game/model/gate.go`

```go
// app/appContext.go
body, err := codec.ProtoCodec.Marshal(msg)  // game 回包用 proto

// game/model/gate.go
&gatewayService{actor: g, codec: codec.JsonCodec}  // gate 解码用 json
```

方向不对称且不可配置。回包的 codec 应该跟着客户端 subprotocol 走（即跟着 `ConnActor.codec`），不应该在 game 侧固定。

**建议**：
- `Frame` 加 `Codec` 字段，或者通过 connID 在 Gate 侧反查
- game 侧透明转发 payload，编解码只发生在 Gate

---

### 14. `Frame.MsgType` 字段被填充但未被消费

**位置**：`gate/server/conn_actor.go` + `game/model/gate.go`

`ConnActor.forward` 把 `MsgType: reflect.TypeOf(inner).Elem().Name()` 塞进 Frame，但 game 侧 `gatewayService.Stream` 解码完全靠 codec，没读 `MsgType`。

**建议**：
- 要么删掉这个字段
- 要么真的用它来加速解码（绕过 codec 的注册表查找）

---

### 15. WebSocket 缺基础健壮性

**位置**：`gate/server/gate_server.go`

- 没有 `ReadLimit`、读写超时、Ping/Pong 心跳
- `upgrader.CheckOrigin` 直接返回 `true`，生产不能这么写
- 没有 graceful shutdown（`http.ListenAndServe` 阻塞，无法干净停）

**建议**：
- 设置合理的 `ReadLimit`（比如 64KB）和 `SetReadDeadline`
- 配置 `CheckOrigin` 走白名单 / 同源
- 用 `http.Server` 替换全局 `http.ListenAndServe`，支持 `Shutdown(ctx)`
- 加心跳：服务端定期发 Ping，连续失败则关闭连接

---

### 16. 日志混用 3 套

**位置**：散落在多处

- 标准库 `log`（gate 侧到处是）
- `bigTree/log`（game/main.go 用）
- `fmt.Printf`（ctl/chat.go）

**建议**：
- 统一到 `bigTree/log` 或换成标准库 `slog`
- 加 lint 规则禁用其他 logger

---

### 17. `ConnActor` 注册了两个不同的名字

**位置**：`gate/server/gate_server.go` + `conn_actor.go`

```go
// gate_server.go
name := fmt.Sprintf("conn-%p", conn)
pid := s.sys.Spawn(name, connActor)

// conn_actor.go OnInit
ctx.Register(connActorName(c.connID))  // 又注册了一次，名字格式不一样
```

`ConnActor` 同时存在两个名字（`conn-%p` 和 `conn-{id}`）。StreamActor 回程用 `connActorName(frame.ConnId)` 查找——能不能查到完全取决于 bigTree 是否允许多名字注册。

**建议**：
- 统一用 `connActorName(connID)` 一种格式
- `Spawn` 时直接传这个名字

---

### 18. `inboundMsg` 字段全小写跨包不可见，状态结构有冗余

**位置**：`game/model/gate.go` + `game/app/appContext.go`

```go
type inboundMsg struct {
    msg    proto.Message
    uid    uint64
    connID uint64
}
```

`GameActor.HandleMessage` 在 `model` 包内消费没问题，但 `gameContext` 在 `app` 包又定义了一遍 uid/connID。两份内部状态结构耦合冗余，长期看会漂移。

**建议**：抽到一个公共包（比如 `game/internal/msg`），导出字段，两处共享。

---

## 🟢 锦上添花（架构改进 / 工程实践）

### 19. 缺测试

仓库里 `bigTree` 自己有测试，但 gogs 业务侧 **0 测试**。建议至少有：

- `codec` 的 round-trip 测试（注册 → marshal → unmarshal）
- `cluster` 的 etcd register/watch 集成测试（用 embedded etcd）
- 一个端到端的 gate↔game 烟雾测试

---

### 20. 缺 metric / tracing

游戏服务器线上排查仅靠日志远远不够。**至少**要有：

- actor 处理耗时直方图
- 消息队列长度（mailbox 深度）
- gRPC stream 健康度（Send/Recv 错误率、重连次数）
- WS 连接数 / QPS

Prometheus + 一行 middleware 就能上。后续可加 OpenTelemetry tracing 串起 Gate↔Game。

---

### 21. 没有优雅停机

`Ctrl+C` 时：
- WS 不会通知客户端
- ActorSystem 不会等 actor 跑完
- gRPC stream 不会 `CloseSend`
- etcd lease 不会主动 revoke（要等 30s 才下线）

**建议**：在 `main.go` 用 `signal.Notify` 串起停机流程：

1. 停止接受新连接（关闭 listener）
2. 通知所有 actor stop
3. 等 ActorSystem drain
4. revoke etcd lease
5. 关闭 etcd / NATS / Mongo 连接

---

### 22. `tools/gen-register.lua` 应该改成 Go

团队成员不一定都装 Lua。protoreg 这类小工具用 Go + `text/template` 写一个，跟 `make proto` 串起来更顺。

---

### 23. `bigTree` 作为目录嵌套在仓库里

看到 `D:\gosrc\gogs\bigTree\.git` 的存在，说明它是直接 clone 进来而不是 submodule。这样 `bigTree` 自己的 `.git` 会被 `gogs` 的 git 当成一个不跟踪的子目录，依赖更新很容易出乱子。

**建议**：
- 改成正式 git submodule
- 或走 `go.mod` 引用 + 本地 `replace` 指令

---

### 24. `gateway.proto` 双重序列化

把整条业务消息（已经 proto 序列化过）再塞进 `Frame.Payload`（又是 bytes），等于在 wire 上**双重序列化**。

**建议**：
- `Frame` 用 `oneof` 直接承载具体消息类型
- 省一次 marshal/unmarshal
- 同时让 game 侧能直接拿到强类型消息

---

## 优先级路线图

按"修复成本 / 收益"排序的建议执行顺序：

### 第一批：让系统真正能跑通

| 编号 | 问题 | 估时 |
|------|------|------|
| #1 | Codec 注册流程接通 | 0.5 day |
| #2 | ConnActor.handleWsMsg 状态 bug | 0.5 day |
| #3 | router 初始化 | 0.5h |
| #7 | Printf 格式串 | 5 min |
| #8 | 登录路由生效（依赖 #2） | 0.5 day |

完成后应该跑通 `客户端 → 登录 → 业务消息 → 回包` 全链路。

### 第二批：分布式部署的稳定性底线

| 编号 | 问题 | 估时 |
|------|------|------|
| #4 | GateActor.stream 并发安全 | 1 day |
| #5 | StreamActor 重连 | 1 day |
| #6 | etcd keepalive 重试 | 0.5 day |
| #21 | 优雅停机 | 0.5 day |

### 第三批：工程基础

| 编号 | 问题 | 估时 |
|------|------|------|
| #19 | 单测 + 端到端烟雾测试 | 2 day |
| #20 | metric / tracing | 1 day |
| #15 | WS 健壮性 | 0.5 day |
| #16 | 日志统一 | 0.5 day |

### 第四批：架构清理

剩余编号（#9-#14、#17、#18、#22-#24），可以在功能开发的间隙穿插重构。

---

## 附：当前代码读取来源

整理本文档时阅读了以下文件：

- `gate/main.go`、`gate/server/*.go`
- `game/main.go`、`game/model/*.go`、`game/app/*.go`、`game/router/handler.go`、`game/ctl/chat.go`、`game/player/playerMgr.go`
- `codec/*.go`
- `cluster/cluster.go`
- `config/config.go`

如有变更，请同步更新本文档。
