# NatsRPC 模块分析文档

## 一、模块定位

NatsRPC 是项目的**进程间通信基础设施**，基于 [NATS](https://nats.io/) 消息队列，为所有节点（game/cross/actv/rank/mail 等）提供统一的跨进程调用能力，支持：

- **同步调用（Call）**：发送后阻塞等待响应，有超时机制
- **异步调用（ACall）**：发送后注册回调，不阻塞
- **单向投递（Cast）**：发送后不等回复，fire-and-forget
- **直接寻址（SendToSubj）**：向指定 NATS subject 发送消息（如推送给客户端）

---

## 二、目录结构

```
natsrpc/
├── external.go             # 对外暴露的调用接口（唯一出口）
├── natsdef/def.go          # 节点类型常量、默认 ServerID
└── internal/
    ├── module.go           # 模块生命周期、消息接收主循环
    ├── handler.go          # 消息路由：ClusterCall / RPCPackReq / RPCPackAck
    └── ctl_cmd.go          # NatsCmd 特殊指令处理

utility/natsutil/nats.go    # NATS 连接封装、收发底层实现
common/natsmsg/natsmsg.go   # 非 proto 消息的类型注册与反射查找
```

---

## 三、节点类型常量（natsdef/def.go）

```go
const (
    Game    = "game"
    Cross   = "cross"
    Actv    = "actv"
    Door    = "door"
    Global  = "global"
    Rank    = "rank"
    Mail    = "mail"
    Jabber  = "jabber"
    Chat    = "chat"
    WebServ = "webserv"
    PLAYMOD = "play"   // chanRPC 模块名
    C2SMOD  = "c2s"   // chanRPC 模块名
)

// 全局唯一节点的默认 ID
var DoorId   int32 = 1
var GlobalId int32 = 1
var RankId   int32 = 1
```

---

## 四、NATS Subject 寻址规则

每个节点订阅一个固定 subject，格式：

```
{gameID}.{nodeType}.{serverID}
```

示例：
- `k3x.game.1001`  —— gameID=k3x、game 节点、serverID=1001
- `k3x.rank.1`     —— rank 节点

`gameID` 来自启动参数 `--gameid`（或 gconf 中配置），用于隔离不同游戏实例。

---

## 五、连接参数（natsutil/nats.go）

| 参数 | 值 | 说明 |
|------|----|------|
| MaxReconnects | -1 | 无限重连 |
| ReconnectWait | 200ms | 重连间隔 |
| ConnectionTimeout | 3s | 连接超时 |
| PingInterval | 15s | 心跳间隔 |
| MaxPingsOutstanding | 10 | 最大未响应 ping 数 |
| MaxPayload | 8MB | 单条消息最大字节 |
| PendingLimits | 20万条 / 128MB | 订阅缓冲上限 |
| SubChan buffer | 409600 | Go channel 缓冲大小 |
| RPC 超时（默认） | 5s | `timerNatsCall` 触发 |

---

## 六、核心数据结构

### 6.1 RPCPack（公共信封）

```go
type RPCPack struct {
    Type      RPCPackType  // REQ=0 / ACK=1 / CAST=2
    Reply     string       // 发送方的 subject（用于回复）
    From      string       // 回复方的 subject（ACK 中填写）
    SessionID int64        // 会话ID，用于匹配请求与响应
    RpcHandle string       // 接收方处理该消息的 chanRPC 模块名
    Data      []byte       // 序列化后的业务消息体
    ProtoName string       // 非 proto 消息时，存 JSON 类型名（iproto）
    NodeType  string       // 发送方节点类型
    ServerID  int32        // 发送方 serverID
}
```

### 6.2 消息包装层

| 结构体 | 用途 |
|--------|------|
| `RPCPackReq { Pack *RPCPack }` | 请求包（REQ / CAST）通过 NATS 发送 |
| `RPCPackAck { Pack *RPCPack }` | 响应包（ACK）通过 NATS 回复 |
| `RPCFrameReq { Data []byte }` | `SendToSubj` 使用的简单帧（无 RPC 语义） |
| `RPCErrorNtf { Err string }` | 错误通知，目标 handle 不存在时返回 |

### 6.3 RPCPackType

```
REQ  = 0  // 需要响应的请求
ACK  = 1  // 响应
CAST = 2  // 不需要响应的单向投递
```

### 6.4 ClusterCall（内部调度结构）

```go
type ClusterCall struct {
    GameID     string      // 目标 gameID（空则用自身 gameID）
    NodeType   string      // 目标节点类型（如 "rank"）
    ServerID   int32       // 目标 serverID（0 表示单例节点）
    HandleServ string      // 目标节点的 chanRPC 模块名
    Msg        interface{} // 业务消息体
    Cast       bool        // true=CAST，false=REQ
    Subj       string      // 非空时绕过路由，直接发到该 subject
    Timeout    int64       // 自定义超时（ms），0 则用默认 5s
}
```

### 6.5 ClusterCallAck（超时/错误返回）

```go
type ClusterCallAck struct {
    Err              error
    TargetServerType string
    TargetServerID   int32
    Msg              interface{} // 原始请求消息，用于重试
}
```

---

## 七、对外接口（external.go）

### 7.1 Call — 同步调用（阻塞）

```go
func Call(serverType string, serverID int32, handleName string, msg interface{}) *chanrpc.RetInfo
```

发送消息并**阻塞等待**响应，超时默认 5s。适合需要立即使用结果的场景。

### 7.2 ACall — 异步调用（回调）

```go
func ACall(s *module.Skeleton, serverType string, serverID int32, handleName string, msg interface{}, cb chanrpc.Callback, ctx cbctx.M) error
```

发送后**注册回调**，不阻塞当前 goroutine。`ctx` 用于向回调传递上下文参数。

### 7.3 ACallWithTimeout — 带超时的异步调用

```go
func ACallWithTimeout(s *module.Skeleton, serverType string, serverID int32, handleName string, msg interface{}, cb chanrpc.Callback, timeout int64, ctx cbctx.M) error
```

同 ACall，额外指定超时时间（ms）。

### 7.4 Cast — 单向投递（fire-and-forget）

```go
func Cast(serverType string, serverID int32, handleName string, msg interface{}) error
```

发送后不等回复。适合日志、通知、状态同步等不需要响应的场景。

### 7.5 CastWithGameID — 跨游戏投递

```go
func CastWithGameID(gameID string, serverType string, serverID int32, handleName string, msg interface{}) error
```

同 Cast，但可以指定目标 `gameID`，用于跨游戏实例通信。

### 7.6 SendToSubj — 直接 subject 发送

```go
func SendToSubj(subj string, msg proto.Message) error
```

绕过节点路由，直接向指定 subject 发送消息（无 RPC 语义，只发不收）。

### 7.7 SendTo — 向玩家推送消息

```go
func SendTo(pid int64, msg proto.Message) error
```

将消息推送给指定玩家（通过 `gconf.GetPlayerGateSubj(pid)` 获取玩家所在 gate 的 subject）。

---

## 八、模块内部工作流程

### 8.1 初始化（Module.OnInit）

```
1. 创建 skeleton（chanRPC 调度器）
2. 建立 NATS 连接（natsutil.NewNatsRPC）
3. 订阅自身 subject：{gameID}.{tp}.{serverID}
4. 注册消息处理器（initHandler）
5. 初始化 rpcqueue（map[sessionID]*CallInfo）
6. 注册超时定时器 TimerNatsCall
```

### 8.2 消息接收主循环（runRPC goroutine）

```
NATS 消息到达
  → 写入 SubChan（buffered channel，容量 409600）
  → runRPC goroutine 消费
  → processor.Unmarshal 解包（得到 RPCPackReq / RPCPackAck）
  → skeleton.Cast 投入 chanRPC 队列
  → handler 处理
```

### 8.3 发送一条 Call（REQ）的完整链路

```
调用方 natsrpc.Call(serverType, serverID, handleName, msg)
  │
  ├─ external.go: 向 NatsMod chanRPC 投递 ClusterCall
  │
  ├─ handler.go onClusterCall():
  │     sessionID++
  │     ns.Send() → 序列化为 RPCPackReq → NATS Publish
  │     rpcqueue[sessionID] = ci（保存等待回调的 CallInfo）
  │     注册 5s 超时定时器
  │
  ├─ [目标节点收到 RPCPackReq]
  │     handleReq():
  │       解析消息体 → 路由到目标 chanRPC（req.RpcHandle）
  │       skeleton.ClusterAsynCall → 业务处理
  │       onRpcCall 回调 → ns.Reply() → 发回 RPCPackAck
  │
  └─ [发送方收到 RPCPackAck]
        handleAck():
          从 rpcqueue 取出 ci
          取消超时定时器
          解析响应 → ci.Ret(msg) → 唤醒阻塞的 Call
```

### 8.4 超时处理

若 5s 内未收到 ACK，`timerNatsCall` 触发：

```go
ci.Ret(&ClusterCallAck{
    Err:              fmt.Errorf("timeout"),
    TargetServerType: call.NodeType,
    TargetServerID:   call.ServerID,
    Msg:              call.Msg,  // 保留原始消息，调用方可选择重试
})
delete(m.rpcqueue, sid)
```

---

## 九、消息序列化：两种模式

| 模式 | 条件 | 序列化方式 | ProtoName 字段 |
|------|------|-----------|----------------|
| **Proto** | 消息实现 `proto.Message` 接口 | `cspb.Processor.Marshal` | 空字符串 |
| **iProto（JSON）** | 普通 Go struct（非 proto） | `jsoniter.Marshal` | 类型全名（如 `types.RallyInfo`） |

iProto 消息需提前用 `natsmsg.Register(msg)` 注册类型，接收方通过 `natsmsg.GetType(name)` 反射还原。

---

## 十、NatsCmd 特殊指令（ctl_cmd.go）

专为 GM/运维指令设计的特殊消息类型 `NatsCmdReq`，通过命令名路由：

```go
func (m *Module) OnNatsCmdReq(req0 *cspb.RPCPackReq, cmd *cspb.NatsCmdReq)
```

流程：
1. 用 `cmd.Args[0]` 查配置 `gdconf.GetCmdMetasCfgByName()`
2. 通过 `skeleton.ClusterAsynCallByName` 按名字路由到对应 handler
3. 若 handle 不存在，返回 `NatsCmdAck{Msg: "not find handle..."}`

---

## 十一、典型调用示例

### game 节点调用 rank 节点（Cast）

```go
// rank/rankmgr 或 game 内部
natsrpc.Cast(natsdef.Rank, natsdef.RankId, gameconf.RANK, &cspb.UpdateRankNodeReq{
    RankId: "xxx",
    NodeId: "player_123",
    Score:  9999,
})
```

### game 节点调用 rank 节点（Call，等待结果）

```go
ret := natsrpc.Call(natsdef.Rank, natsdef.RankId, gameconf.RANK, &cspb.GetRankListReq{
    RankId: "xxx",
    Count:  100,
})
ack := ret.Ack.(*cspb.GetRankListAck)
```

### 向玩家推送消息

```go
natsrpc.SendTo(playerID, &cspb.SomeNtf{...})
// 等价于：
natsrpc.SendToSubj(gconf.GetPlayerGateSubj(playerID), &cspb.SomeNtf{...})
```

---

## 十二、注意事项

| 问题 | 说明 |
|------|------|
| **SubChan 溢出** | 缓冲 409600 条，若消费跟不上生产会触发 NATS 的 PendingLimits（20万/128MB），超出后 NATS 会强制断开订阅 |
| **rpcqueue 内存泄漏** | 若超时定时器未触发（极端情况），sessionID 对应的 `CallInfo` 会一直留在 map 中 |
| **单线程处理** | `handleReq/handleAck` 均在 skeleton 的单一 goroutine 执行，高并发下可能成为瓶颈 |
| **NATS 连接中断** | 设置了无限重连，中断期间的发送会返回错误，调用方需自行处理 |
| **iProto 消息需提前注册** | 非 proto 的 Go struct 必须调用 `natsmsg.Register()` 注册，否则接收方无法反序列化 |
