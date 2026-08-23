# Gate 通信架构分析

## 一、客户端连接模型

每个客户端连接对应一个 `GWAgent`，由 WebSocket handler 创建：

```
客户端 WebSocket 请求
  → handler(c *gin.Context) 被调用（每次新连接执行一次）
  → upGrader.Upgrade() 升级为 WebSocket 长连接
  → NewGWAgent() 创建 agent
  → go agent.Run()  // 独立 goroutine，阻塞处理该连接
```

每个连接 = 一个独立的 `agent.Run()` goroutine，连接数上限由 `MaxConnNum` 控制。

---

## 二、GWAgent 核心结构

```go
type GWAgent struct {
    conn          network.Conn                  // 客户端 WebSocket 连接
    fromStream    chan *gspb.GameFrame           // 来自 game 节点的消息
    stream        gspb.GameService_StreamClient // 到 game 节点的 gRPC stream
    fromClient    chan []byte                   // 来自客户端的消息
    closeSig      chan bool                     // 关闭信号
    natsMsgChan   chan *nats.Msg                // NATS 消息
    // ...
}
```

---

## 三、gate → game 通信：gRPC Stream

### 连接模型

每个玩家登录后，gate 会调用 `OpenStreamToGame()` 建立一条 gRPC stream：

```go
cli := gspb.NewGameServiceClient(gameServer.Conn)
stream, err := cli.Stream(context.Background())
```

**关键**：`gameServer.Conn`（`*grpc.ClientConn`）是**共享**的，所有连接到同一 game 节点的 agent 复用同一条底层 TCP 连接，但每个 agent 拥有独立的 gRPC stream（HTTP/2 多路复用）：

```
gate 进程
  ├── GWAgent(玩家A) → stream1 ─┐
  ├── GWAgent(玩家B) → stream2 ─┼── 同一 grpc.ClientConn (一条TCP) → game节点
  └── GWAgent(玩家C) → stream3 ─┘
```

### 消息流向

```
客户端消息 → goConnReader goroutine → fromClient channel → Run() → stream.Send() → game
game 消息  → goStreamReader goroutine → fromStream channel → Run() → conn.WriteMsg() → 客户端
```

---

## 四、客户端断线流程

```
conn 读错误（EOF / 网络断开）
  → goConnReader break，close(fromClient)
    → Run() 检测到 fromClient ok=false，return
      → wsConn.Close()          // 关闭底层 WebSocket 连接
      → OnClose()
          ├── closeStreamSend() // 半关闭 gRPC stream（CloseSend）
          ├── conn.Close()      // 关闭连接
          ├── sub.Unsubscribe() // 取消 NATS 订阅
          ├── drain fromClient  // 排空 channel 防阻塞
          └── drain fromStream
      → 从 server.conns 集合移除
```

### CloseSend() 语义

`stream.CloseSend()` 是**半关闭**，只关闭发送方向：

```
gate ──Send──→ game   ← 关闭（game 侧 Recv() 返回 EOF）
gate ←─Recv─── game  ← 不影响
```

game 侧通过 `Recv()` 返回 `EOF` 感知玩家下线，触发存档、广播离线等清理逻辑。  
关闭的是**该玩家的 stream**，不影响共享的 `ClientConn` 和其他玩家的 stream。

---

## 五、closeSig 关闭信号

`closeSig chan bool` 用于**服务器主动关闭** agent（如踢人、服务器停机）：

```go
// 写入方（主动关闭）
func (agent *GWAgent) Close() {
    close(agent.closeSig)  // close() 而非发送值，立即触发所有监听者
}

// 读取方（Run 主循环）
case <-agent.closeSig:
    return
```

触发场景：服务器踢人、`App.Stop()` 停机流程逐个关闭所有 module。

---

## 六、gRPC Stream vs NATS 对比

项目中两种通信并存：
- **主消息**（移动、战斗等高频）：gRPC stream
- **推送/广播类消息**：NATS 订阅

### 性能对比

| | gRPC Stream | NATS |
|--|--|--|
| 延迟 | 极低，点对点直连无中间节点 | 略高，消息经 NATS Server 中转 |
| 吞吐/QPS | 更高，HTTP/2 多路复用零拷贝 | 略低，受 NATS Server 处理能力限制 |
| 消息可靠性 | TCP 层保证有序可靠 | 基础模式 at-most-once，JetStream 支持持久化 |

### 架构特性对比

| | gRPC Stream | NATS |
|--|--|--|
| 拓扑 | 点对点，需知道目标节点 | 发布订阅，发送方无需关心接收方 |
| 路由 | 固定，玩家登录时绑定特定 game | 灵活，topic 路由，动态换接收方 |
| 广播 | 需遍历所有 stream，麻烦 | 天然支持，一条消息多个订阅者 |
| 负载均衡 | 需自己实现 | 内置 queue group |
| 连接管理 | 需维护 stream 生命周期 | 无状态，fire and forget |
| 断线处理 | stream 断了需重建 | 自动重连，可持久化 |

### 适用场景

- **gRPC stream**：gate ↔ game 固定点对点的高频游戏消息，追求最低延迟和最高吞吐
- **NATS**：跨节点通知、GM 命令、事件广播等需要灵活路由或广播的场景
