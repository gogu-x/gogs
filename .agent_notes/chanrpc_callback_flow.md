# ChanRPC 回调机制完整调用关系文档

## 概述

这是一个基于 Go channel 的 RPC 框架，支持同步调用、异步调用和回调机制。本文档详细说明异步回调的完整执行流程。

---

## 核心数据结构

### CallInfo - 调用信息
```go
type CallInfo struct {
    id          uint32        // 消息类型id
    Req         interface{}   // 入参
    chanRet     chan *RetInfo // 结果信息返回通道
    cb          Callback      // 回调函数
    ctx         cbctx.M       // 回调上下文
    hasRet      bool          // 是否已经返回（防重复返回）
    SrcNodeType string        // 源节点类型
    SrcServerID int32         // 源服务器ID
    TimerID     int64         // 定时器ID
    sessionID   int64         // 会话ID（用于追踪）
}
```

### RetInfo - 返回信息
```go
type RetInfo struct {
    Ack             interface{} // 结果值（作为回调函数的入参）
    Err             error       // 错误信息
    cb              Callback    // 回调函数
    Ctx             cbctx.M     // 回调上下文
    clientSessionID int64       // 客户端会话ID
}
```

### Server - 服务端
```go
type Server struct {
    functions map[interface{}]Handler  // 消息ID → 处理函数映射
    ChanCall  chan *CallInfo            // 调用请求通道
    name      string
    // ...
}
```

### Client - 客户端
```go
type Client struct {
    chanCall        chan *CallInfo           // 调用信息通道（指向 Server.ChanCall）
    chanSyncRet     chan *RetInfo            // 同步调用结果通道
    ChanAsynRet     chan *RetInfo            // 异步调用结果通道
    pendingAsynCall int                      // 待处理异步调用计数
    callList        map[int64]interface{}    // 会话ID → 调用信息映射
    sessionID       int64                    // 当前会话ID
}
```

---

## 完整调用流程

### 阶段 1：客户端发起异步调用

#### 1.1 调用 `AsynCall` 方法
```go
// 文件位置: chanrpc.go:378-399
func (c *Client) AsynCall(req interface{}, cb Callback, ctx cbctx.M) error {
    id := MsgID(req)           // 计算消息ID
    c.sessionID++              // 递增会话ID
    
    err := call(c.chanCall, &CallInfo{
        id:        id,
        Req:       req,
        chanRet:   c.ChanAsynRet,  // ⚠️ 指定异步返回通道
        cb:        cb,              // ⚠️ 保存回调函数
        ctx:       ctx,             // ⚠️ 保存回调上下文
        sessionID: c.sessionID,
    }, false)  // false = 非阻塞模式
    
    if err != nil {
        return err
    }
    
    // 记录调用信息（用于调试）
    c.callList[c.sessionID] = reflect.TypeOf(req).String()
    c.pendingAsynCall++  // 递增待处理计数
    return nil
}
```

#### 1.2 `call` 函数将 CallInfo 放入通道
```go
// 文件位置: chanrpc.go:482-499
func call(chanCall chan *CallInfo, ci *CallInfo, block bool) error {
    // 非阻塞模式
    select {
    case chanCall <- ci:  // ⚠️ 将 CallInfo 发送到 Server.ChanCall
        return nil
    default:
        return fmt.Errorf("server chanrpc channel full")
    }
}
```

**此时状态：**
- `CallInfo` 已进入 `Server.ChanCall` 通道
- 回调函数 `cb` 保存在 `CallInfo.cb` 字段中
- 返回通道 `CallInfo.chanRet` 指向 `Client.ChanAsynRet`

---

### 阶段 2：服务端接收并处理请求

#### 2.1 服务端从通道取出 CallInfo
```go
// 典型的服务端消息循环（用户代码）
for {
    select {
    case ci := <-server.ChanCall:  // ⚠️ 从通道取出调用信息
        server.Exec(ci)             // 执行处理
    }
}
```

#### 2.2 `Exec` 执行业务处理
```go
// 文件位置: chanrpc.go:240-246
func (s *Server) Exec(ci *CallInfo) {
    ci.hasRet = false  // 重置返回标志
    err := s.exec(ci)  // 实际执行
    if err != nil {
        tlog.Errorf("%v", err)
    }
}

// 文件位置: chanrpc.go:213-237
func (s *Server) exec(ci *CallInfo) (err error) {
    defer func() {
        if r := recover(); r != nil {
            // panic 捕获，自动调用 ci.ret 返回错误
            if !ci.hasRet {
                _ = ci.ret(&RetInfo{Err: fmt.Errorf("%v", r)})
            }
        }
    }()
    
    // 根据消息ID查找处理函数
    handler, ok := s.functions[ci.id]
    if !ok {
        panic(fmt.Sprintf("msg %+v not register handler", ci.id))
    }
    
    handler(ci)  // ⚠️ 调用业务处理函数
    return
}
```

#### 2.3 业务处理函数执行并返回结果
```go
// 用户注册的处理函数示例
func handleUserLogin(ci *CallInfo) {
    req := ci.Req.(*LoginRequest)
    
    // 执行业务逻辑
    user := authenticateUser(req.Username, req.Password)
    
    // 返回结果
    ci.Ret(user)  // ⚠️ 调用 Ret 方法返回结果
}
```

---

### 阶段 3：服务端返回结果

#### 3.1 `Ret` 方法封装返回信息
```go
// 文件位置: chanrpc.go:66-79
func (ci *CallInfo) Ret(ret interface{}) {
    // 防止重复返回
    if ci.hasRet {
        tlog.Errorf("chanrpc can not ret twice")
        return
    }
    ci.hasRet = true
    
    // 封装返回信息并调用 ret
    err := ci.ret(&RetInfo{
        Ack: ret,      // 返回值
        Ctx: ci.ctx,   // 回调上下文
    })
    
    if err != nil {
        tlog.Errorf("chanrpc ret error: %v, msgid: %v", err, ci.id)
    }
}
```

#### 3.2 `ret` 方法将结果发送到客户端
```go
// 文件位置: chanrpc.go:97-113
func (ci *CallInfo) ret(ri *RetInfo) (err error) {
    // 检查返回通道
    if ci.chanRet == nil {
        return  // 如果是 Cast 调用，没有返回通道
    }
    
    defer func() {
        if r := recover(); r != nil {
            err = r.(error)
        }
    }()
    
    // ⚠️ 关键步骤：将回调函数和会话ID传递给 RetInfo
    ri.cb = ci.cb                      // 传递回调函数
    ri.clientSessionID = ci.sessionID  // 传递会话ID
    
    // ⚠️ 核心操作：将 RetInfo 发送到客户端的异步返回通道
    ci.chanRet <- ri  // 即 Client.ChanAsynRet <- ri
    return
}
```

**此时状态：**
- `RetInfo` 已进入 `Client.ChanAsynRet` 通道
- `RetInfo.cb` 包含原始的回调函数
- `RetInfo.Ack` 包含处理结果
- `RetInfo.Ctx` 包含回调上下文

---

### 阶段 4：客户端接收结果并执行回调

#### 4.1 客户端消息循环接收返回结果
```go
// 典型的客户端消息循环（用户代码）
func runClient(client *Client) {
    for {
        select {
        case ri := <-client.ChanAsynRet:  // ⚠️ 从异步返回通道取出结果
            client.Cb(ri)                  // ⚠️ 处理回调
        }
    }
}
```

#### 4.2 `Cb` 方法处理回调
```go
// 文件位置: chanrpc.go:455-459
func (c *Client) Cb(ri *RetInfo) {
    c.pendingAsynCall--                     // 递减待处理计数
    delete(c.callList, ri.clientSessionID)  // 清理调用记录
    execCb(ri)                              // ⚠️ 执行回调
}
```

#### 4.3 `execCb` 执行真正的回调函数
```go
// 文件位置: chanrpc.go:438-452
func execCb(ri *RetInfo) {
    defer func() {
        if r := recover(); r != nil {
            // ⚠️ 捕获回调函数中的 panic，防止崩溃
            if conf.LenStackBuf > 0 {
                buf := make([]byte, conf.LenStackBuf)
                l := runtime.Stack(buf, false)
                tlog.Errorf("%v: %s", r, buf[:l])
            } else {
                tlog.Errorf("%v", r)
            }
        }
    }()
    
    ri.cb(ri)  // ⚠️⚠️⚠️ 真正执行用户的回调函数
}
```

#### 4.4 用户回调函数执行
```go
// 用户注册的回调函数
callback := func(ri *RetInfo) {
    if ri.Err != nil {
        log.Printf("调用失败: %v", ri.Err)
        return
    }
    
    user := ri.Ack.(*User)
    log.Printf("登录成功: %v", user.Username)
    
    // 可以访问回调上下文
    if ri.Ctx != nil {
        requestID := ri.Ctx["request_id"]
        log.Printf("请求ID: %v", requestID)
    }
}
```

---

## 完整调用链时序图

```
┌─────────┐                  ┌─────────┐                  ┌─────────┐
│ Client  │                  │ Channel │                  │ Server  │
└────┬────┘                  └────┬────┘                  └────┬────┘
     │                            │                            │
     │ 1. AsynCall(req, cb, ctx)  │                            │
     ├───────────────────────────>│                            │
     │   CallInfo{                │                            │
     │     cb: cb,                │                            │
     │     chanRet: ChanAsynRet   │                            │
     │   }                        │                            │
     │                            │                            │
     │                            │ 2. ci := <-ChanCall        │
     │                            │<───────────────────────────┤
     │                            │                            │
     │                            │         3. Exec(ci)        │
     │                            │         handler(ci)        │
     │                            │         业务逻辑处理        │
     │                            │                            │
     │                            │         4. ci.Ret(result)  │
     │                            │         ci.ret(RetInfo{    │
     │                            │           cb: ci.cb,       │
     │                            │           Ack: result      │
     │                            │         })                 │
     │                            │                            │
     │                            │<───────────────────────────┤
     │                            │   chanRet <- ri            │
     │                            │   (即 ChanAsynRet <- ri)   │
     │                            │                            │
     │ 5. ri := <-ChanAsynRet     │                            │
     │<───────────────────────────┤                            │
     │                            │                            │
     │ 6. Cb(ri)                  │                            │
     │    execCb(ri)              │                            │
     │    ri.cb(ri) ──> 用户回调执行                            │
     │                            │                            │
```

---

## 关键设计要点

### 1. 回调函数的传递路径
```
用户代码
  ↓
Client.AsynCall(cb) → CallInfo.cb
  ↓
通道传递 (ChanCall)
  ↓
Server.Exec(ci) → handler(ci)
  ↓
ci.Ret() → ci.ret() → RetInfo.cb = ci.cb
  ↓
通道传递 (ChanAsynRet)
  ↓
Client.Cb(ri) → execCb(ri) → ri.cb(ri)
  ↓
用户回调函数执行
```

### 2. 通道桥接机制
- **请求通道**: `Client.chanCall` → `Server.ChanCall`
- **返回通道**: `CallInfo.chanRet` → `Client.ChanAsynRet`
- 通过这两个通道实现客户端和服务端的解耦

### 3. 会话跟踪
- `CallInfo.sessionID` 用于追踪每个异步调用
- `Client.callList` 记录所有待处理的调用
- `Client.pendingAsynCall` 计数器用于监控待处理数量

### 4. 错误处理与恢复
- **服务端 panic 捕获**: `Server.exec` 的 defer-recover 机制
- **回调 panic 捕获**: `execCb` 的 defer-recover 机制
- **重复返回检测**: `CallInfo.hasRet` 标志防止重复调用 `Ret`

### 5. 并发安全
- 使用 channel 天然的并发安全特性
- `hasRet` 标志防止重复返回（但注意：不是原子操作，依赖单 goroutine 处理）

---

## 三种调用模式对比

### 1. Cast - 投递模式（无返回值）
```go
server.Cast(req)
// 特点：chanRet = nil，无回调，无等待
```

### 2. Call - 同步调用
```go
ri := client.Call(req)
// 流程：
// 1. CallInfo.chanRet = client.chanSyncRet (缓冲区为1)
// 2. ri := <-client.chanSyncRet  // 阻塞等待
// 3. 直接返回结果，无回调
```

### 3. AsynCall - 异步调用（有回调）
```go
client.AsynCall(req, callback, ctx)
// 流程：
// 1. CallInfo.chanRet = client.ChanAsynRet
// 2. CallInfo.cb = callback
// 3. 立即返回，不阻塞
// 4. 结果通过 ChanAsynRet 通道返回
// 5. 客户端消息循环调用 client.Cb(ri) 执行回调
```

---

## 超时机制详解

### 超时机制概述

ChanRPC 框架通过 **定时器 (Timer)** 实现异步调用的超时控制，`CallInfo.TimerID` 字段用于关联超时定时器。超时机制确保异步调用不会无限期等待，在指定时间内未收到响应时自动触发超时回调。

### 超时机制完整流程

#### 1. 发起带超时的异步调用

```go
// 文件位置: natsrpc/external.go:43-52
func ACallWithTimeout(s *module.Skeleton, serverType string, serverID int32, 
                       handleName string, msg interface{}, cb chanrpc.Callback, 
                       timeout int64, ctx cbctx.M) error {
    servCh := core.DefaultApp().GetChanRPC(internal.NatsMod)
    return s.AsynCall(servCh, &internal.ClusterCall{
        NodeType:   serverType,
        ServerID:   serverID,
        HandleServ: handleName,
        Msg:        msg,
        Timeout:    timeout,  // ⚠️ 设置超时时间（单位：毫秒）
    }, cb, ctx)
}
```

**关键点：**
- `Timeout` 字段单位是 **毫秒**（不是秒）
- 例如：`Timeout: 5000` 表示 5 秒超时

#### 2. 服务端创建超时定时器

```go
// 文件位置: natsrpc/internal/handler.go:158-193
func (m *Module) onClusterCall(ci *chanrpc.CallInfo) {
    req := ci.Req.(*ClusterCall)
    
    if !req.Cast {  // 非 Cast 调用才需要超时控制
        // 发送 RPC 请求，获取会话ID
        sid, err := m.ns.Send(req.GameID, req.NodeType, serverId, req.HandleServ, req.Msg)
        
        // ⚠️ 保存 CallInfo 到等待队列，key 为会话ID
        m.rpcqueue[sid] = ci
        
        // ⚠️ 设置超时时间（默认 5 秒）
        var timeout int64 = types.Second * 5  // 5000 毫秒
        if req.Timeout > 0 {
            timeout = req.Timeout  // 使用用户指定的超时时间
        }
        
        // ⚠️⚠️⚠️ 创建超时定时器，将 TimerID 保存到 CallInfo
        ci.TimerID = m.timerMgr.NewTimer(
            timeout, 
            types.TimerNatsCall,  // 定时器类型
            cbctx.M{types.ID: cbctx.Int64(sid)}  // 传递会话ID作为参数
        )
    }
}
```

**此时状态：**
- `CallInfo` 被保存到 `m.rpcqueue[sessionID]`
- 超时定时器已创建，`CallInfo.TimerID` 保存了定时器 ID
- 定时器到期后会调用 `timerNatsCall` 函数

#### 3. 正常返回时取消定时器

```go
// 文件位置: natsrpc/internal/handler.go:106-134
func (m *Module) handleAck(ci *chanrpc.CallInfo) {
    ack0 := ci.Req.(*cspb.RPCPackAck)
    req := ack0.Pack
    sid := req.SessionID
    
    // 从等待队列中取出 CallInfo
    ci, ok := m.rpcqueue[sid]
    if ok {
        // ⚠️⚠️⚠️ 收到响应，立即取消超时定时器
        m.timerMgr.CancelTimer(ci.TimerID)
        
        // 解析响应消息
        _, msg, err := m.processor.Unmarshal(req.Data)
        
        // 返回结果（触发回调）
        if ack, ok := msg.(*cspb.RPCErrorNtf); ok {
            ci.RetWithError(ack, err)
        } else {
            ci.Ret(msg)
        }
        
        // 从等待队列中删除
        delete(m.rpcqueue, sid)
    }
}
```

**正常流程：**
1. 远程服务返回响应 → `handleAck` 被调用
2. 通过 `sessionID` 从等待队列中找到 `CallInfo`
3. **立即取消超时定时器**，防止误触发
4. 调用 `ci.Ret(msg)` 返回结果，触发用户回调
5. 从等待队列中删除 `CallInfo`

#### 4. 超时触发时的处理

```go
// 文件位置: natsrpc/internal/handler.go:195-207
func (m *Module) timerNatsCall(_ int64, args cbctx.M) {
    // 从定时器参数中获取会话ID
    sid := args[types.ID].Int64()
    
    // 从等待队列中查找对应的 CallInfo
    if ci, ok := m.rpcqueue[sid]; ok {
        if call, ok := ci.Req.(*ClusterCall); ok {
            name := reflect.TypeOf(call.Msg).Elem().Name()
            tlog.Infof("timerNatsCall %+v src:%s %d", name, ci.SrcNodeType, ci.SrcServerID)
            
            // ⚠️⚠️⚠️ 返回超时错误，触发回调
            ci.Ret(&natsutil.ClusterCallAck{
                Err:              fmt.Errorf("timeout"),
                TargetServerType: call.NodeType,
                TargetServerID:   call.ServerID,
                Msg:              call.Msg,
            })
        } else {
            ci.Ret(&natsutil.ClusterCallAck{Err: fmt.Errorf("timeout")})
        }
        
        // 从等待队列中删除
        delete(m.rpcqueue, sid)
    }
}
```

**超时流程：**
1. 定时器到期 → `timerNatsCall` 被调用
2. 根据会话 ID 从等待队列中找到 `CallInfo`
3. 调用 `ci.Ret()` 返回包含 `timeout` 错误的结果
4. 用户的回调函数被执行，`ri.Err` 或 `ri.Ack` 包含超时错误
5. 从等待队列中删除 `CallInfo`
6. **后续如果响应到达，会被忽略**（因为 `rpcqueue[sid]` 已删除）

### 超时机制时序图

```
┌─────────┐         ┌──────────┐         ┌─────────┐         ┌──────────┐
│ Client  │         │ NatsRPC  │         │  Timer  │         │ Remote   │
└────┬────┘         └────┬─────┘         └────┬────┘         └────┬─────┘
     │                   │                    │                   │
     │ ACallWithTimeout  │                    │                   │
     ├──────────────────>│                    │                   │
     │  (timeout=3000ms) │                    │                   │
     │                   │                    │                   │
     │                   │ NewTimer(3000ms)   │                   │
     │                   ├───────────────────>│                   │
     │                   │ TimerID=123        │                   │
     │                   │ ci.TimerID = 123   │                   │
     │                   │ rpcqueue[sid] = ci │                   │
     │                   │                    │                   │
     │                   │ Send RPC Request   │                   │
     │                   ├───────────────────────────────────────>│
     │                   │                    │                   │
     │                   │                    │                   │
     ├─── 场景 A：正常返回（2000ms 内响应）────────────────────────────┤
     │                   │                    │                   │
     │                   │         RPC Response (2000ms)          │
     │                   │<───────────────────────────────────────┤
     │                   │                    │                   │
     │                   │ CancelTimer(123)   │                   │
     │                   ├───────────────────>│                   │
     │                   │                    │ ✓ 定时器已取消     │
     │                   │                    │                   │
     │                   │ ci.Ret(result)     │                   │
     │<──────────────────┤ (触发回调)          │                   │
     │  callback(ri)     │                    │                   │
     │  ri.Err = nil     │                    │                   │
     │  ri.Ack = result  │                    │                   │
     │                   │                    │                   │
     │                   │                    │                   │
     ├─── 场景 B：超时（超过 3000ms 无响应）───────────────────────────┤
     │                   │                    │                   │
     │                   │                    │ ⏰ 3000ms 到期    │
     │                   │                    │                   │
     │                   │ timerNatsCall()    │                   │
     │                   │<───────────────────┤                   │
     │                   │                    │                   │
     │                   │ ci.Ret(timeout err)│                   │
     │<──────────────────┤ (触发回调)          │                   │
     │  callback(ri)     │                    │                   │
     │  ri.Ack.Err = timeout                 │                   │
     │                   │                    │                   │
     │                   │ delete(rpcqueue[sid])                  │
     │                   │                    │                   │
     │                   │ (后续响应到达会被忽略，因为 rpcqueue 已删除) │
     │                   │         RPC Response (4000ms, 被忽略)   │
     │                   │<───────────────────────────────────────┤
     │                   │ rpcqueue[sid] 不存在，丢弃响应          │
```

### 超时相关数据结构

#### ClusterCall - 跨服务器调用请求
```go
// 文件位置: natsrpc/internal/handler.go:23-32
type ClusterCall struct {
    GameID     string
    NodeType   string        // 目标节点类型
    ServerID   int32         // 目标服务器ID
    HandleServ string        // 处理服务名
    Msg        interface{}   // 请求消息
    Cast       bool          // 是否是 Cast 调用（Cast 不需要超时）
    Subj       string        // NATS 主题
    Timeout    int64         // ⚠️ 超时时间（单位：毫秒）
}
```

#### Module 等待队列
```go
type Module struct {
    rpcqueue map[int64]*chanrpc.CallInfo  // 会话ID → CallInfo 映射
    timerMgr *timer.Manager                // 定时器管理器
    // ...
}
```

**关键字段：**
- `rpcqueue` - 保存所有待响应的异步调用
- `timerMgr` - 管理所有超时定时器
- `CallInfo.TimerID` - 关联的定时器 ID，用于取消

### 超时配置与使用示例

#### 1. 使用默认超时（5秒）
```go
// 使用框架默认的 5 秒超时
natsrpc.ACall(skeleton, "game", serverID, "HandleLogin", req, func(ri *chanrpc.RetInfo) {
    if ri.Err != nil {
        log.Printf("调用失败: %v", ri.Err)
        return
    }
    // 处理正常响应
    result := ri.Ack.(*LoginAck)
    log.Printf("登录成功: %v", result.UserID)
}, ctx)
```

#### 2. 自定义超时时间
```go
// 自定义超时时间为 3 秒
natsrpc.ACallWithTimeout(
    skeleton, 
    "game", 
    serverID, 
    "HandleLogin", 
    req, 
    func(ri *chanrpc.RetInfo) {
        if ri.Err != nil {
            log.Printf("调用失败: %v", ri.Err)
            return
        }
        result := ri.Ack.(*LoginAck)
        log.Printf("登录成功: %v", result.UserID)
    },
    3000,  // 3000 毫秒 = 3 秒
    ctx,
)
```

#### 3. 区分超时错误与其他错误
```go
callback := func(ri *chanrpc.RetInfo) {
    // 检查是否有错误
    if ri.Err != nil {
        log.Errorf("RPC调用出错: %v", ri.Err)
        return
    }
    
    // 检查 Ack 是否是超时错误响应
    if ack, ok := ri.Ack.(*natsutil.ClusterCallAck); ok {
        if ack.Err != nil {
            if ack.Err.Error() == "timeout" {
                // ⚠️⚠️ 超时处理逻辑
                log.Warnf("调用 %s:%d 超时，消息类型：%T", 
                    ack.TargetServerType, 
                    ack.TargetServerID, 
                    ack.Msg)
                
                // 可以尝试重试、降级处理等
                handleTimeout(ack)
                return
            }
            // 其他业务错误
            log.Errorf("业务错误: %v", ack.Err)
            return
        }
    }
    
    // 正常业务逻辑
    processResult(ri.Ack)
}
```

### 超时处理最佳实践

#### 1. 根据业务场景设置合理超时

```go
// 快速查询（缓存、配置）：1 秒超时
natsrpc.ACallWithTimeout(s, "cache", sid, "GetUser", req, cb, 1000, ctx)

// 普通业务逻辑：3-5 秒超时
natsrpc.ACallWithTimeout(s, "game", sid, "HandleBattle", req, cb, 3000, ctx)

// 复杂计算（排行榜、统计）：10-30 秒超时
natsrpc.ACallWithTimeout(s, "logic", sid, "CalcRank", req, cb, 10000, ctx)

// 长耗时操作（数据导出、报表生成）：60 秒以上
natsrpc.ACallWithTimeout(s, "report", sid, "ExportData", req, cb, 60000, ctx)
```

#### 2. 超时后的重试策略

```go
type RetryConfig struct {
    MaxRetries int
    Timeout    int64
}

func callWithRetry(s *module.Skeleton, config RetryConfig, req interface{}, 
                   finalCallback chanrpc.Callback) {
    var attempt int
    
    var retryCallback chanrpc.Callback
    retryCallback = func(ri *chanrpc.RetInfo) {
        // 检查是否超时
        isTimeout := false
        if ack, ok := ri.Ack.(*natsutil.ClusterCallAck); ok && ack.Err != nil {
            isTimeout = ack.Err.Error() == "timeout"
        }
        
        if isTimeout && attempt < config.MaxRetries {
            attempt++
            log.Warnf("调用超时，第 %d 次重试", attempt)
            
            // 指数退避：每次重试增加超时时间
            newTimeout := config.Timeout * int64(attempt+1)
            natsrpc.ACallWithTimeout(s, "game", sid, "HandleXXX", 
                req, retryCallback, newTimeout, ctx)
        } else {
            // 达到最大重试次数或成功，调用最终回调
            finalCallback(ri)
        }
    }
    
    // 首次调用
    natsrpc.ACallWithTimeout(s, "game", sid, "HandleXXX", 
        req, retryCallback, config.Timeout, ctx)
}
```

#### 3. 超时监控与告警

```go
// 超时统计
type TimeoutStats struct {
    sync.Mutex
    totalCalls   int64
    timeoutCalls int64
    timeoutByTarget map[string]int64  // 按目标服务器统计
}

var stats = &TimeoutStats{
    timeoutByTarget: make(map[string]int64),
}

func wrapCallbackWithStats(cb chanrpc.Callback) chanrpc.Callback {
    return func(ri *chanrpc.RetInfo) {
        stats.Lock()
        stats.totalCalls++
        
        // 检查是否超时
        if ack, ok := ri.Ack.(*natsutil.ClusterCallAck); ok && ack.Err != nil {
            if ack.Err.Error() == "timeout" {
                stats.timeoutCalls++
                target := fmt.Sprintf("%s:%d", ack.TargetServerType, ack.TargetServerID)
                stats.timeoutByTarget[target]++
            }
        }
        stats.Unlock()
        
        // 调用原始回调
        cb(ri)
    }
}

// 定期输出统计
func startStatsReporter() {
    ticker := time.NewTicker(time.Minute)
    for range ticker.C {
        stats.Lock()
        if stats.totalCalls > 0 {
            rate := float64(stats.timeoutCalls) / float64(stats.totalCalls) * 100
            log.Infof("超时率: %.2f%% (%d/%d)", rate, stats.timeoutCalls, stats.totalCalls)
            
            // 输出超时最多的目标
            for target, count := range stats.timeoutByTarget {
                if count > 10 {
                    log.Warnf("目标 %s 超时次数: %d", target, count)
                }
            }
        }
        stats.Unlock()
    }
}
```

#### 4. 超时后的资源清理

```go
callback := func(ri *chanrpc.RetInfo) {
    // 从上下文中获取相关资源ID
    resourceID := ri.Ctx["resource_id"].Int64()
    
    // 检查超时
    if ack, ok := ri.Ack.(*natsutil.ClusterCallAck); ok && ack.Err != nil {
        if ack.Err.Error() == "timeout" {
            log.Warnf("调用超时，清理资源: %d", resourceID)
            
            // 清理锁定的资源
            releaseLock(resourceID)
            
            // 回滚临时状态
            rollbackTempState(resourceID)
            
            // 通知相关系统
            notifyTimeout(ack.TargetServerType, ack.TargetServerID)
            
            return
        }
    }
    
    // 正常处理
    processResult(ri.Ack)
}
```

### 超时机制的潜在问题

#### 1. 超时后响应仍然到达
```
问题：超时触发后，远程服务的响应可能稍后才到达
影响：响应被忽略（因为 rpcqueue[sid] 已删除），可能导致资源泄漏
建议：
  - 合理设置超时时间，避免过短
  - 在服务端实现幂等性，重复请求不会产生副作用
  - 使用请求ID追踪，即使超时也能关联后续响应
```

#### 2. 定时器泄漏风险
```
问题：如果 handleAck 中忘记调用 CancelTimer，定时器会继续存在
影响：超时回调仍会触发，但 rpcqueue[sid] 已不存在（空操作）
当前保护：timerNatsCall 中检查 rpcqueue[sid] 是否存在
建议：确保所有返回路径都调用 CancelTimer
```

#### 3. 超时时间单位混淆
```
⚠️ 常见错误：
  错误示例：Timeout: 5      // 容易误以为是 5 秒，实际是 5 毫秒！
  正确示例：Timeout: 5000   // 5000 毫秒 = 5 秒

建议：
  - 使用常量定义：const DefaultTimeout = 5 * time.Second.Milliseconds()
  - 或使用辅助函数：func ToMillis(d time.Duration) int64 { return d.Milliseconds() }
```

#### 4. Cast 调用无超时保护
```go
// Cast 调用不创建定时器，无超时保护
if req.Cast {
    _, err := m.ns.Cast(req.GameID, req.NodeType, serverId, req.HandleServ, req.Msg)
    // 没有返回值，无法知道是否成功
}
```

**建议：** Cast 用于不关心结果的场景（日志、通知等），关键操作应使用 AsynCall

#### 5. 等待队列无限增长风险
```
问题：如果大量请求超时，rpcqueue 会持续增长
影响：内存泄漏，影响性能
当前保护：超时触发时会从 rpcqueue 删除
建议：监控 rpcqueue 大小，设置告警阈值
```

### 监控与调试

#### 查看超时日志
```go
// 超时触发时的日志
// 文件位置: natsrpc/internal/handler.go:200
tlog.Infof("timerNatsCall %+v src:%s %d", name, ci.SrcNodeType, ci.SrcServerID)
```

#### 查看等待队列状态
```go
// 添加调试接口
func (m *Module) GetRPCQueueSize() int {
    return len(m.rpcqueue)
}

// 定期输出
go func() {
    ticker := time.NewTicker(10 * time.Second)
    for range ticker.C {
        size := module.GetRPCQueueSize()
        if size > 100 {
            log.Warnf("RPC等待队列过大: %d", size)
        }
    }
}()
```

#### 超时调试技巧
```go
// 1. 记录请求发送时间
ctx := cbctx.M{
    "send_time": cbctx.Int64(time.Now().UnixMilli()),
    "req_type":  cbctx.Str(reflect.TypeOf(req).Name()),
}

// 2. 在回调中计算实际耗时
callback := func(ri *chanrpc.RetInfo) {
    sendTime := ri.Ctx["send_time"].Int64()
    elapsed := time.Now().UnixMilli() - sendTime
    reqType := ri.Ctx["req_type"].Str()
    
    if ack, ok := ri.Ack.(*natsutil.ClusterCallAck); ok && ack.Err != nil {
        if ack.Err.Error() == "timeout" {
            log.Warnf("请求 %s 超时，耗时: %dms", reqType, elapsed)
        }
    } else {
        log.Debugf("请求 %s 完成，耗时: %dms", reqType, elapsed)
    }
}
```

---

## 使用示例

### 完整的异步调用示例

#### 服务端代码
```go
// 1. 创建服务器
server := chanrpc.NewServer(100)

// 2. 注册处理函数
server.Register(&LoginRequest{}, func(ci *chanrpc.CallInfo) {
    req := ci.Req.(*LoginRequest)
    
    // 业务逻辑
    user := &User{
        ID:       123,
        Username: req.Username,
    }
    
    // 返回结果
    ci.Ret(user)
})

// 3. 启动消息循环
go func() {
    for ci := range server.ChanCall {
        server.Exec(ci)
    }
}()
```

#### 客户端代码
```go
// 1. 创建客户端并连接服务器
client := chanrpc.NewClient(100)
client.Attach(server)

// 2. 启动异步返回处理循环
go func() {
    for ri := range client.ChanAsynRet {
        client.Cb(ri)  // 这里会执行回调
    }
}()

// 3. 发起异步调用
ctx := cbctx.M{"request_id": "req-001"}
err := client.AsynCall(
    &LoginRequest{Username: "alice", Password: "123456"},
    func(ri *chanrpc.RetInfo) {
        // 回调函数
        if ri.Err != nil {
            log.Printf("登录失败: %v", ri.Err)
            return
        }
        
        user := ri.Ack.(*User)
        log.Printf("登录成功: %v", user.Username)
        
        // 访问上下文
        requestID := ri.Ctx["request_id"]
        log.Printf("请求ID: %v", requestID)
    },
    ctx,
)

if err != nil {
    log.Printf("调用失败: %v", err)
}
```

---

## 潜在问题与注意事项

### 1. 回调执行线程
- 回调在客户端的消息循环 goroutine 中执行，不是发起调用的 goroutine
- 如果回调耗时长，会阻塞其他异步调用的回调执行
- **建议**: 回调中耗时操作应启动新 goroutine

### 2. 通道缓冲区满
- `ChanAsynRet` 缓冲区满时，服务端 `ci.ret()` 会阻塞
- **建议**: 合理设置缓冲区大小，及时处理异步返回

### 3. 资源泄漏风险
- 如果客户端消息循环停止，待处理的异步调用会永久阻塞
- `Client.Close()` 提供了优雅关闭机制（5秒超时）

### 4. hasRet 并发安全性
- `hasRet` 不是原子操作，依赖于单 goroutine 处理 `CallInfo`
- 如果多 goroutine 同时调用 `ci.Ret()`，可能导致竞态条件
- **当前设计假设**: 每个 `CallInfo` 只由一个 goroutine 处理

### 5. 回调 panic 影响
- 回调中的 panic 会被 `execCb` 捕获，不会导致程序崩溃
- 但当前回调会失败，后续回调继续执行

---

## 调试与监控

### 查看待处理调用
```go
func (c *Client) Idle() bool {
    return c.pendingAsynCall == 0
}

// 调试输出
log.Printf("待处理异步调用数: %d", c.pendingAsynCall)
for sessionID, callInfo := range c.callList {
    log.Printf("会话 %d: %v", sessionID, callInfo)
}
```

### 优雅关闭
```go
// 关闭客户端，等待所有异步调用完成（最多5秒）
client.Close()

// 关闭服务器，拒绝新请求，返回错误给待处理调用
server.Close()
```

---

## 总结

这是一个设计精巧的基于 channel 的异步 RPC 框架，具备完善的超时控制机制：

1. **解耦设计**: 客户端和服务端通过 channel 通信，互不依赖
2. **回调机制**: 通过 `CallInfo` 和 `RetInfo` 传递回调函数
3. **通道桥接**: `chanRet` 是返回结果的关键桥梁
4. **错误保护**: 多层 defer-recover 保证稳定性
5. **会话跟踪**: `sessionID` 机制支持调用追踪和调试
6. **超时控制**: 基于定时器的超时机制，防止无限等待

核心思想：**回调函数随着调用信息流转，最终在客户端的消息循环中执行**。

### 完整的异步调用生命周期（包含超时）

```
1. 发起调用（ACallWithTimeout）
   ↓
2. 创建 CallInfo（包含回调函数、超时时间）
   ↓
3. 将 CallInfo 发送到服务端通道
   ↓
4. 服务端接收，创建超时定时器
   ↓  ci.TimerID = NewTimer(timeout)
5. 发送 RPC 请求到远程服务
   ↓
6. 保存到等待队列 rpcqueue[sessionID] = ci
   ↓
[等待响应或超时，竞争条件]
   ↓
7a. 正常响应到达（响应时间 < 超时时间）
    ├─ handleAck() 被调用
    ├─ CancelTimer(ci.TimerID)  ← 取消定时器
    ├─ ci.Ret(result)           ← 返回结果
    ├─ 执行用户回调 cb(ri)
    └─ delete(rpcqueue[sid])
   ↓
7b. 超时触发（响应时间 >= 超时时间）
    ├─ timerNatsCall() 被调用
    ├─ ci.Ret(timeout error)    ← 返回超时错误
    ├─ 执行用户回调 cb(ri) with timeout error
    ├─ delete(rpcqueue[sid])
    └─ 后续响应到达会被忽略（rpcqueue 已删除）
   ↓
8. 清理完成
```

### 核心设计亮点

#### 1. 回调的全生命周期管理
- 回调函数在 `CallInfo` 中创建
- 通过 channel 传递到服务端
- 通过 `RetInfo` 传回客户端
- 在客户端消息循环中执行
- **不管正常返回还是超时，都保证回调被执行一次**

#### 2. 超时的双重保护
- **正常路径**: 响应到达 → 取消定时器 → 执行回调
- **超时路径**: 定时器到期 → 执行超时回调 → 后续响应被忽略
- **互斥保证**: 通过 `delete(rpcqueue[sid])` 确保只执行一次

#### 3. 资源管理
- `rpcqueue` 作为中央注册表，管理所有待响应请求
- `TimerID` 关联定时器，支持精确取消
- `sessionID` 唯一标识每个请求，防止串扰

#### 4. 错误处理层次
```
Level 1: 网络层错误（连接失败、序列化失败）
  ↓ 立即返回错误，不进入等待队列
  
Level 2: 超时错误（响应未在指定时间内到达）
  ↓ 定时器触发，返回 timeout error
  
Level 3: 业务层错误（远程服务返回错误）
  ↓ 正常响应通道，错误码在响应中
  
Level 4: 回调执行错误（回调函数 panic）
  ↓ execCb 的 defer-recover 捕获，记录日志
```

### 设计权衡与注意事项

| 设计选择 | 优点 | 缺点 | 应对措施 |
|---------|------|------|---------|
| 默认 5 秒超时 | 防止无限等待，保护系统稳定 | 对慢服务可能误杀 | 提供 `ACallWithTimeout` 自定义 |
| 超时后删除 rpcqueue | 防止内存泄漏 | 后续响应被丢弃 | 业务层实现幂等性 |
| 单 goroutine 执行回调 | 保证顺序性，简化并发 | 慢回调阻塞其他回调 | 耗时操作启动新 goroutine |
| Cast 无超时保护 | 性能好，开销低 | 无法确认是否成功 | 关键操作使用 AsynCall |
| TimerID 存储在 CallInfo | 解耦定时器与业务逻辑 | 需要手动取消 | 在所有返回路径调用 CancelTimer |

### 性能考虑

#### 1. 超时时间设置建议
- **低延迟服务**（缓存、配置）: 500ms - 1s
- **普通业务逻辑**: 3s - 5s
- **复杂计算**: 10s - 30s
- **批处理任务**: 30s - 120s

#### 2. 内存占用
- 每个待响应请求占用 `CallInfo` + 定时器 + `rpcqueue` 条目
- 估算：约 500 字节/请求
- 建议：监控 `len(rpcqueue)`，超过 10000 触发告警

#### 3. 定时器开销
- 每个异步调用创建一个定时器
- Go 的定时器实现高效（红黑树）
- 建议：避免大量短超时调用（< 100ms）

### 故障场景分析

#### 场景 1: 远程服务宕机
```
请求发送 → 无响应 → 超时触发 → 回调收到 timeout error
影响：业务可感知，可降级处理
恢复：远程服务重启后，新请求正常
```

#### 场景 2: 网络分区
```
请求发送 → 网络断开 → 超时触发 → 回调收到 timeout error
风险：远程服务可能已执行（但响应丢失）
建议：使用请求 ID 实现幂等性
```

#### 场景 3: 定时器泄漏
```
响应到达 → 忘记 CancelTimer → 定时器仍触发
影响：timerNatsCall 查不到 rpcqueue[sid]，空操作
当前保护：timerNatsCall 检查 ok 再处理
```

#### 场景 4: 回调 panic
```
响应到达 → 执行回调 → panic
影响：execCb 捕获，记录日志，不影响其他回调
建议：回调中使用 defer-recover
```

### 扩展思路

#### 1. 优先级队列
```go
// 为紧急请求设置更短超时
type PriorityCall struct {
    Priority int  // 1=高, 2=中, 3=低
    Timeout  int64
}

// 高优先级：1秒超时
// 中优先级：3秒超时
// 低优先级：10秒超时
```

#### 2. 熔断机制
```go
// 连续超时达到阈值，熔断目标服务
type CircuitBreaker struct {
    failureThreshold int
    failureCount     int
    state           string  // "closed", "open", "half-open"
}

// 熔断后快速失败，不发送请求
```

#### 3. 超时自适应
```go
// 根据历史响应时间动态调整超时
type AdaptiveTimeout struct {
    p95Latency time.Duration  // 95 分位延迟
    timeout    time.Duration  // = p95 * 1.5
}
```

#### 4. 请求追踪
```go
// 记录完整请求链路
type TraceInfo struct {
    TraceID   string
    SpanID    string
    StartTime time.Time
    EndTime   time.Time
    Status    string  // "success", "timeout", "error"
}
```

---

## 快速参考

### 常用 API

```go
// 1. 异步调用（默认 5 秒超时）
natsrpc.ACall(skeleton, serverType, serverID, handleName, msg, callback, ctx)

// 2. 异步调用（自定义超时）
natsrpc.ACallWithTimeout(skeleton, serverType, serverID, handleName, msg, 
    callback, 3000, ctx)  // 3 秒超时

// 3. 投递（无返回值，无超时）
natsrpc.Cast(serverType, serverID, handleName, msg)

// 4. 同步调用（阻塞等待，建议避免）
ri := natsrpc.Call(serverType, serverID, handleName, msg)
```

### 回调模板

```go
callback := func(ri *chanrpc.RetInfo) {
    // 1. 检查错误
    if ri.Err != nil {
        log.Errorf("调用失败: %v", ri.Err)
        return
    }
    
    // 2. 检查超时
    if ack, ok := ri.Ack.(*natsutil.ClusterCallAck); ok {
        if ack.Err != nil && ack.Err.Error() == "timeout" {
            log.Warnf("调用超时: %s:%d", ack.TargetServerType, ack.TargetServerID)
            // 超时处理：重试、降级、清理资源
            return
        }
    }
    
    // 3. 正常业务逻辑
    result := ri.Ack.(*YourResponseType)
    processResult(result)
}
```

### 故障排查清单

1. **调用无响应**
   - 检查目标服务是否在线
   - 检查 handleName 是否注册
   - 检查网络连通性
   - 查看 rpcqueue 大小

2. **频繁超时**
   - 检查超时时间设置是否合理
   - 查看目标服务负载
   - 检查网络延迟
   - 分析超时日志中的目标分布

3. **内存增长**
   - 检查 rpcqueue 大小
   - 确认超时机制是否正常工作
   - 检查是否有定时器泄漏

4. **回调未执行**
   - 确认客户端消息循环是否运行
   - 检查 ChanAsynRet 通道是否阻塞
   - 查看 pendingAsynCall 计数

---

**文档版本**: v2.0  
**最后更新**: 2026-06-10  
**包含内容**: 
- ✅ 完整回调流程
- ✅ 超时机制详解
- ✅ 时序图与代码示例
- ✅ 最佳实践与故障排查
- ✅ 性能考虑与扩展思路
