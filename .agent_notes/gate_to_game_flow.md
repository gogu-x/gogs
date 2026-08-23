# Gate → Game 通信流程详解

本文档梳理客户端连接 gate 网关、gate 与 game 之间建立通信、负载均衡（选服）以及消息透传的完整链路。核心代码路径：

- `gate/server/gate.go` — Gateway 进程入口，TCP/WS 监听
- `gate/server/agent.go` — GWAgent，每个客户端连接对应一个 agent，负责与 game 的 gRPC Stream 桥接
- `gate/server/handler.go` — 认证、选服、登录/建号请求处理
- `vendor/git.tap4fun.com/fw/tse/core/cluster/cluster.go` — 基于 etcd 的服务发现/节点管理框架
- `game/gsgate/internal/*.go` — game 进程内的网关适配模块（gsgate），是 gate 连接的对端
- `protocol/pb/gspb/game.proto` — gate ↔ game 之间的 gRPC 协议定义

---

## 1. 整体架构图

```
客户端(TCP/WS)
     │  AES加密 + 自定义分包协议(cspb)
     ▼
┌─────────────┐        etcd (服务发现/注册)         ┌─────────────┐
│    gate     │ ◄──────────────────────────────────► │  game (N个) │
│  (网关进程)  │                                       │ (游戏逻辑进程)│
│             │  gRPC 双向Stream (gspb.GameService)   │             │
│  GWAgent    │ ──────────────────────────────────►  │  GSGate     │
│ (每连接一个) │ ◄──────────────────────────────────  │  GSAgent    │
└─────────────┘         GameFrame{Type,Msg,GenTs}     │ (每连接一个) │
     │                                                 └─────────────┘
     │ NATS (选服/账号导量等异步RPC)                          │
     ▼                                                        ▼
  door/imports模块                                    play/wmap/actv 等业务模块
 (账号验证/服务器列表/负载均衡数据源)
```

gate 与 game 之间**不是**通过 NATS 转发玩家消息，而是**每个客户端连接对应一条 gRPC 双向流（Stream）**，玩家消息在这条流上双向透传。NATS 只用于 gate 内部子模块调用（如 `ImportsMod` 查服务器列表/停服信息）和跨进程的辅助信息同步。

---

## 2. 服务发现与节点注册（etcd）

gate 和 game 都通过 `cluster.Init()` 向同一个 etcd 集群注册/订阅节点：

- game 进程启动时（`game/gsgate/internal/gate.go: OnInit`）以 `NodeType=gconf.GameServerName` 注册自己，key 形如 `<Root>/game/<ServerID>`，value 是 `NodeMeta{Name, Addr, Label, Weight, DBURL}`（`Addr` 就是 game 的 gRPC 监听地址）。
- gate 进程启动时（`gate/server/gate.go: OnInit`）以 `NodeType=gconf.GateServerName` 注册自己，并声明依赖 `Deps: [{NodeType: game, AutoConn: true}]`。`AutoConn: true` 表示 gate 会对每个发现的 game 节点自动建立 gRPC 连接（`grpc.DialContext`），连接池由 cluster 框架维护。
- etcd watcher（`cluster.watcher`）持续 watch `Root/` 前缀下的 PUT/DELETE 事件，动态增删 `NodeGroup.NodeList`，同时维护自身的租约心跳（keepalive），租约过期或进程退出会导致节点从 etcd 中消失，gate 侧随之感知下线。
- gate 通过两个函数使用这份节点表：
  - `cluster.GetByID("game", serverID)`：按服务器 ID 精确定位某个 game 节点的 gRPC 连接，登录/建号时用它确定"连到哪个 game"。
  - `cluster.Range("game", ...)`：遍历所有在线 game 节点，用于组装服务器列表、判断某服是否在线。
  - `cluster.Get("game")`：轮询（round-robin）方式取一个节点，目前 gate 未用它做玩家路由（因为游戏是按 ServerID 精确分服，不是无状态的负载均衡），仅框架层保留该能力。

**结论：gate→game 的"负载均衡"实质是按 ServerID 做精确路由，不是流量层面的轮询负载均衡**。真正的"负载均衡"发生在**选服阶段**（见第 4 节）——即引导玩家去哪个 ServerID 对应的 game 实例，一旦选定服务器，这个连接的所有流量就固定打到那一个 game 节点，不会再分散。

---

## 2.1 客户端多连一层：gateproxy（可选调试代理）

`gateproxy/` 是一个独立的调试/抓包代理进程（非生产链路必需），可插入到 客户端-gate 之间做协议分析、消息注入，与本文主流程（gate↔game）无直接关系，此处不展开。

---

## 3. 客户端连接建立与握手

1. 客户端建立 TCP 或 WebSocket 连接到 gate（`gate/server/gate.go: initTcpLogic` / `OnInit` 中的 `wss`）。
2. 每条连接对应一个 `GWAgent`（`gate/server/agent.go: NewGWAgent`）。
3. **仅 TCP 连接**需要密钥交换（`keyExchange`）：
   - 客户端发送 RSA 加密的临时密钥 `tempKey`；
   - gate 用私钥解密得到 `tempKey`，随机生成 `sessionKey`（16字节 AES key）；
   - 用 `tempKey` 加密 `sessionKey` 后回传给客户端；
   - 之后该连接上所有消息均用 `sessionKey` 做 AES 加解密（`aesCrypter`）。
   - WS 连接不做这层加密（协议里假设 WSS 已经有 TLS）。
4. `GWAgent.Run()` 启动后进入主循环，用 `select` 同时监听 4 类事件源：
   - `fromClient`：客户端发来的原始字节（`goConnReader` goroutine 读取、解密、限流后写入这个 channel）；
   - `fromStream`：game 通过 gRPC Stream 回传的 `GameFrame`；
   - `natsMsgChan`：NATS 订阅到的消息（用于服务端主动推送，如踢人、GM广播等）；
   - 心跳定时器：超过 `HeartbeatKickDur`(300s) 无心跳则断开连接。

### 限流保护
`goConnReader` 内置简单滑动窗口流控：每 `FlowCtrlStage`(10s) 统计一次，超过 `FlowCtrlStageMaxPackets`(10000包) 直接断连；此外单包间隔小于 `FlowCtrlMinSpan`(50ms) 会被动 sleep 补齐，防止单连接刷包。

---

## 4. 认证与选服（负载均衡核心）

客户端连接建立后的典型请求序列：`AuthReq → (可选)ServersReq → CharacterLoginReq/CharacterCreateReq/FastLoginReq`。

### 4.1 AuthReq（账号认证 + 推荐服）— `handler.go: OnAuthReq`

1. 校验客户端版本号（低于 `version.GateSupport` 直接拒绝）。
2. 组装三方登录信息（`pfproto.ThirdData`），通过 chanrpc **同步 Call** 到 gate 内部 `PFMOD`（平台模块）做账号鉴权，得到 `AccountID / JWTToken / Characters(角色列表)`。
3. 调用 `getServersInfo(agent)`：通过 chanrpc 同步 Call 到 `conf.ImportsMod`（gate 内部导量/服务器信息模块），该模块负责持有全服 `GSMetas`（服务器配置：状态/权重/最大在线数/维护时间等）以及各服当前导量统计（`GSImportStatisticalMgr`），这些数据由 `imports/internal/gsmeta_reader.go` 定时（10秒）从 DB（door 服务的库）异步刷新。
4. 调用 `getServerMeta(agent)` 获取维护时间等 meta 信息，用于过滤/合服判断。
5. 组装角色列表 `covertPfCharacters`：过滤已合服的角色，标记黑名单角色。
6. **`agent.selectRecommand(serverInfo, clientInfo)`** —— 这是负载均衡/选服算法的入口，返回推荐服 `RecommandServer` 和导量渠道 `channelId`：
   - 先用 `cluster.Range("game", ...)` 拿到当前**真实在线**的 game 节点集合；
   - 遍历所有服务器配置，过滤出"可登录"的服（`isTrust` 白名单用户跳过检查，否则要求非维护期 + 已上线）；
   - 调用 `getRecommand(ableServers, importInfo, cInfo)` 做多条件过滤 + 权重择优：
     - **状态过滤**：`Status != ServerStatus_Fill` 的服（非"招新中"状态）跳过；
     - **满员过滤**：若配置了 `MaxLoginCount>0` 且当前导量统计的 `Total` 超过上限，跳过（防止单服超载，这是容量层面的负载均衡）；
     - **平台过滤**：`Platform` 限定且与客户端平台不匹配则跳过；
     - **语言过滤**：`Locales` 限定且客户端语言不在列表则跳过；
     - **分渠道限流**：若服务器开启 `ImportType==2`（按渠道导量），根据客户端包名 `BundleID` 找到渠道规则，若该渠道当前导量数 `ChannelCnt >= Max` 则跳过（精细化到渠道级别的容量控制）；
     - 在剩余候选中选 **`Weight`（权重）最大的一组**，若权重并列则**随机**选一个（`getRecommand` 末尾逻辑，代码中 `maxServers` 收集所有同权重服，再随机取一个）——这就是"权重负载均衡"的落地实现；
     - 若没有任何候选通过过滤（如 disable 状态但仍需返回），兜底逻辑选 `ServerID` 最大的服。
   - 找到目标服后，按客户端 `BundleID` 查 `ImportPkgRule` 得到 `importChannelId`（用于后续导量统计打点）。
7. 返回 `AuthAck`，包含：角色列表、`Servers`（`packServers` 组装的全服状态列表，含每服 `IsOnline` 标记——在线判断同样依赖 `cluster.Range("game",...)` 里是否能找到对应节点）、`RecommandServer` 推荐服 ID。

### 4.2 ServersReq（主动查服务器列表）— `handler.go: OnServersReq`

单纯查询，逻辑与 AuthAck 里的 `Servers` 字段一致，走同一个 `getServersInfo` + `packServers`，不涉及选服决策。

### 4.3 canLogin 二次校验

`OnCharacterLoginReq / OnCharacterCreateReq / OnFastLoginReq` 在真正建立到 game 的连接前，都会调用 `agent.canLogin(serverID)` 再次校验：
- dev/beta 环境直接放行；
- 白名单用户（`isTrust`：UDID/账号/IP 白名单）直接放行；
- 否则要求该服存在、不在维护期、且**必须在 `cluster.GetByID("game", serverID)` 中找到对应在线节点**——即该服的 game 进程必须真实在线，避免把连接指向一个已经下线的服。

---

## 5. gate → game 的连接建立：`OpenStreamToGame`

选服完成、客户端发出 `CharacterLoginReq`（或建号/快速登录）后，`agent.go: OpenStreamToGame(basic *gspb.FrameBasic)` 建立到目标 game 的 gRPC 流：

```go
func (agent *GWAgent) OpenStreamToGame(basic *gspb.FrameBasic) cspb.ErrCode {
    preStream := agent.Getstream()
    if preStream != nil && agent.gsID != basic.Server {
        // 已有到另一个服的连接，先关闭旧的
        agent.closeStreamSend()
    }
    if preStream == nil {
        gameServer := cluster.GetByID("game", int(basic.Server))  // 按ServerID精确路由
        if gameServer == nil {
            return cspb.ErrCodeServerNotFound   // 目标game不在线
        }
        cli := gspb.NewGameServiceClient(gameServer.Conn)  // 复用cluster管理的gRPC连接
        stream, err := cli.Stream(context.Background())    // 建立双向流
        ...
        agent.Setstream(stream)
        agent.gsID = basic.Server
        agent.goStreamReader(agent.Getstream(), agent.fromStream) // 起goroutine持续读game侧回包
    }
    return cspb.ErrCodeSuccess
}
```

要点：
- **一个 GWAgent（一条客户端连接）在同一时刻只维护一条到某个 game 的 Stream**；若客户端切服/转服，会先 `closeStreamSend()` 关闭旧流再开新流。
- Stream 底层复用的是 cluster 框架里已经建立好的 gRPC `ClientConn`（`gameServer.Conn`），不是每次都新建 TCP 连接，只是在这个连接上开一个新的 gRPC stream。
- 连不通（`gameServer == nil` 或 `cli.Stream()` err）时返回 `ErrCodeServerNotFound` / `ErrCodeConnectServerFailed`，gate 会关闭客户端连接（`agent.Close()`）。

---

## 6. gate ↔ game 消息透传协议

协议定义在 `protocol/pb/gspb/game.proto`：

```protobuf
service GameService {
    rpc Stream(stream GameFrame) returns (stream GameFrame);  // 双向流
}

enum FrameType {
    FrameTypeMsg     = 0;  // 普通业务消息
    FrameTypeKickout = 1;  // game侧踢下线通知
}

message GameFrame {
    FrameType Type = 1;
    bytes Msg      = 2;   // 内层是 cspb 编码后的业务消息（MsgID+Body）
    int64 GenTs    = 3;   // 生成时间戳，用于耗时统计
}
```

### 6.1 客户端 → game（上行）

`recvPacket(data)`（`agent.go`）：

1. `codec.ReadMsgID(data)` 解出消息 ID；
2. 若消息 ID 已过期（`pb.IsMsgIdOutDate`）直接丢弃；
3. 统计请求耗时打点（`CountRespReq`）；
4. **若该消息在 gate 本地注册了 hook**（`agent.router`，见 `handler.go: initHook`，包括 `AuthReq/ServersReq/CharacterLoginReq/CharacterCreateReq/FastLoginReq/HeartBeatAck/...`），**在 gate 本地直接处理，不转发**；
5. 否则要求 `agent.GetState() == StateLogined`（已登录状态），取出 `stream := agent.Getstream()`，透传：`stream.Send(&gspb.GameFrame{Type: FrameTypeMsg, Msg: data, GenTs: now})`。

即：**登录/认证/选服相关消息在 gate 拦截自处理；登录成功后的所有业务消息（背包/建筑/行军等）原样透传给 game，gate 不解析业务内容**。

### 6.2 game → 客户端（下行）

`recvFrame(frame *gspb.GameFrame)`（`agent.go`，由 `goStreamReader` 从 stream 读到后塞进 `agent.fromStream`，主循环 `Run()` 里消费）：

1. `FrameTypeKickout`：直接返回 `false`，主循环随之退出，`OnClose` 清理连接（踢下线）；
2. `FrameTypeMsg`：解出 `msgID`，统计回包耗时（`CountRespAck`，若耗时超10ms打 `[SlowTrans]` 日志）；
3. 若该 `msgID` 在 gate 本地也注册了 hook（比如 `LoginAck` 需要 gate 记录状态切换为 `StateLogined`、缓存角色信息等），先本地处理；
4. 否则 `agent.WriteRaw(frame.Msg)` 直接把 game 传来的编码后的数据（TCP连接会先AES加密）写回客户端 socket。

即下行同样是**透传为主，仅登录态相关的少量消息 gate 会拦截处理**（如登录成功后设置 `StateLogined`、保存 `playerID`）。

### 6.3 game 侧接收端（gsgate 模块）

`game/gsgate/internal/service.go: Stream()` 是 gRPC 服务端实现：

```go
func (gsGate *GSGate) Stream(stream gspb.GameService_StreamServer) (err error) {
    agent := NewGSAgent(stream, gsGate.Processor, gsGate.msgStat)
    gsGate.agents.Store(agent.sessionID, agent)
    agent.Run()          // 阻塞运行，直到连接结束
    agent.OnClose()
    gsGate.agents.Delete(agent.sessionID)
    return
}
```

- 每条 gate 发起的 Stream 对应 game 侧一个 `GSAgent`，同样维护一个 `sessionID -> GSAgent` 的 map，登录成功后还会建立 `playerID -> GSAgent`（`pid2Agent`）映射，供 game 内部模块按玩家 ID 主动推送消息。
- `GSAgent` 收到消息后按 `msgID` 查本地 `hooks`（`agentrouter.go: initHook`：`HeartBeatReq / CharacterCreateReq / CharacterLoginReq / FastLoginReq / LogoutReq` 等），登录/建号类消息在 gsgate 层做处理后通过 chanrpc 转给 `PLAYMOD`（play 模块）等业务模块；其余消息透传给 play 模块处理后再把结果通过 stream 回传给 gate。
- game 进程内部通过 `core.DefaultApp().GetChanRPC(mod)` 这种模块内 chanrpc 机制把 gsgate 收到的请求路由给 `play/wmap/actv` 等具体业务模块（详见 `CONTEXT/core.md` 里 game 内部 4 模块的划分）。

---

## 7. 掉线/踢人/重连

- **客户端主动断开或超时**：`GWAgent.OnClose()` 会先 `closeStreamSend()`（正常关闭到 game 的流）再关闭客户端 socket，同时 `Unsubscribe` NATS 订阅。
- **game 侧踢人**：game 主动发送 `FrameTypeKickout` 帧，gate 收到后关闭客户端连接。
- **服务端下线公告**：`GSGate.OnDestroy()` 中会给所有在线 `pid2Agent` 发送 `ServerShutdownNtf`，停机前 2 秒缓冲期。
- **重连**：`game/gsgate/internal/gate.go` 里维护了 `reconnKeep`（playerID → 断线保留时间�戳），用于短时间断线重连时恢复会话状态（具体重连协议未在本次梳理范围内展开，如需要可进一步深入 `agent.go` / `agenthook.go`）。

---

## 8. 小结：负载均衡机制定位

| 层级 | 机制 | 代码位置 |
|------|------|---------|
| 服务发现 | etcd 注册 + watch，gate 自动建立到所有 game 节点的 gRPC 连接池 | `cluster/cluster.go` |
| 连接路由 | 按 ServerID 精确匹配（不是无状态轮询），一个客户端连接只打向一个 game | `agent.go: OpenStreamToGame` + `cluster.GetByID` |
| **选服负载均衡** | 认证阶段按权重(Weight)、容量(MaxLoginCount)、渠道导量(ImportRule)、平台/语言多条件过滤后择优推荐 | `handler.go: selectRecommand / getRecommand` |
| 消息面负载 | 无额外负载均衡，登录后所有消息固定走一条 gRPC Stream 直连该服 game | `recvPacket / SendToGame / recvFrame` |
| 全局态数据源 | 各服在线人数/导量统计来自 `conf.ImportsMod`，定时(10s)从 DB 异步刷新 | `imports/internal/gsmeta_reader.go` |

一句话总结：**gate 到 game 之间没有传统意义上"多实例分摊同一份流量"的负载均衡（因为游戏本身是分服架构，每个 ServerID 对应唯一一个 game 实例）；真正的负载均衡发生在玩家登录前的"选服推荐"阶段，通过权重+容量+渠道多重规则把新玩家分散引导到不同服务器，一旦选定服务器，该连接的通信链路就是 gate↔该game 的一条固定 gRPC Stream，全程透传，不做二次转发。**
