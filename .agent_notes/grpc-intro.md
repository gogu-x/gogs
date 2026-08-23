# gRPC 深入详解

> 本文脱离具体业务项目,介绍 gRPC 的原理、通信模式、Go 生态用法与常见陷阱。
> 适合:想彻底搞懂 gRPC"普通请求 vs 流式请求"差异、需要排查 gRPC 问题的开发者。

---

## 目录

1. [gRPC 是什么](#1-grpc-是什么)
2. [HTTP/2 基础:一切的底层](#2-http2-基础一切的底层)
3. [四种 RPC 模式](#3-四种-rpc-模式)
4. [Unary 普通请求完整链路](#4-unary-普通请求完整链路)
5. [Streaming 流式请求完整链路](#5-streaming-流式请求完整链路)
6. [ClientConn、Client、Stream 三者关系](#6-clientconn-client-stream-三者关系)
7. [生命周期与连接管理](#7-生命周期与连接管理)
8. [超时、取消与元数据](#8-超时取消与元数据)
9. [常见陷阱与最佳实践](#9-常见陷阱与最佳实践)
10. [调试手段](#10-调试手段)

---

## 1. gRPC 是什么

gRPC 是 Google 开源的高性能 RPC 框架,核心特征:

- **基于 HTTP/2**:连接复用、多路复用、双向流、头部压缩。
- **默认使用 Protobuf**:强类型接口(IDL 驱动)、高效的二进制序列化。
- **四种 RPC 模式**:Unary / Server-streaming / Client-streaming / Bidi-streaming。
- **多语言**:Go / C++ / Java / Python / Node 等。

**核心工作流(IDL 驱动)**:

```
写 .proto 文件(定义 service + message)
        │   protoc 编译生成代码
        ▼
服务端:实现 service 接口,注册到 grpc.Server
客户端:用生成的 Client 结构体调用远端方法
```

gRPC 与 RPC 的本质区别:传统 RPC(如基于 TCP 的自定义协议)每次请求都要建立连接、自己处理消息边界和序列化;gRPC 通过 HTTP/2 把连接建立、多路复用、头部压缩、流式传输全部标准化,业务只需关心"调一个方法"。

---

## 2. HTTP/2 基础:一切的底层

无论普通请求还是流式请求,gRPC 底层都跑在 **HTTP/2** 之上。理解 HTTP/2 是理解 gRPC 的关键。

### 2.1 关键概念

| 概念 | 说明 |
|------|------|
| **Connection(连接)** | 一个 TCP 连接,HTTP/2 在这上面多路复用。重量级资源,应全局复用 |
| **Stream(流)** | 连接内的**逻辑双向字节流**,用整数 Stream ID 标识。可并行多条 |
| **Frame(帧)** | 流上传输的最小数据单元(HEADERS / DATA / PING / RST_STREAM / GOAWAY…) |
| **多路复用** | 多个 Stream 的帧在同一连接上交错传输,互不阻塞 |

```
       ┌────────────────── 一条 TCP Connection ──────────────────┐
       │  Stream #1  [HEADERS][DATA][DATA]      ← unary 调用A     │
       │  Stream #3  [HEADERS][DATA][DATA]...   ← 双向流(玩家会话)│
       │  Stream #5  [HEADERS][DATA]            ← unary 调用B     │
       └──────────────────────────────────────────────────────────┘
       帧交错发送,按 Stream ID 归位
```

### 2.2 头部压缩(HPACK)

HTTP/2 对头部做 HPACK 压缩,并维护"动态表"。同一连接上**首次发送方法名、路径等头部后,后续请求头部体积极小**。这意味着:同一连接上发起大量小请求时,头部开销被摊薄。

### 2.3 为什么 gRPC 选 HTTP/2

- 一条连接承载海量并发 RPC,减少 TCP 握手与连接数。
- 双向流是天然支持,服务端可主动推送。
- 流级取消、超时、优先级控制。

---

## 3. 四种 RPC 模式

proto 中通过 `stream` 关键字的位置区分四种模式:

| 模式 | proto 写法 | 方向 | 典型场景 |
|------|-----------|------|---------|
| **Unary(一元)** | `rpc Foo(A) returns (B)` | 请求 1 个,响应 1 个 | 支付回调、GM 指令、查询 |
| **Server-streaming** | `rpc Foo(A) returns (stream B)` | 请求 1 个,响应多个 | 日志推送、订阅列表 |
| **Client-streaming** | `rpc Foo(stream A) returns (B)` | 请求多个,响应 1 个 | 批量上传、聚合上报 |
| **Bidi(双向流)** | `rpc Foo(stream A) returns (stream B)` | 双方任意时刻收发 | 网关长连接、聊天、实时对战 |

**stream 关键字的语义**:

```
rpc Unary(A) returns (B);            // 普通:一呼一应
rpc SrvStream(A) returns (stream B); // 服务端流:一次请求,多次响应
rpc CliStream(stream A) returns (B); // 客户端流:多次请求,一次响应
rpc Bidi(stream A) returns (stream B);// 双向流:两边各自持续收发
```

---

## 4. Unary 普通请求完整链路

### 4.1 proto 定义

```protobuf
syntax = "proto3";
package echo;

service EchoService {
    rpc Echo(EchoReq) returns (EchoResp);
}

message EchoReq { string msg = 1; }
message EchoResp { string reply = 1; }
```

### 4.2 客户端调用

```go
// 客户端初始化(全局一次)
conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
cli := echopb.NewEchoServiceClient(conn)   // 极轻的包装,可反复创建

// 普通调用
ctx, cancel := context.WithTimeout(context.Background(), time.Second)
defer cancel()
resp, err := cli.Echo(ctx, &echopb.EchoReq{Msg: "hello"})
if err != nil {
    log.Fatalf("echo failed: %v", err)
}
```

### 4.3 底层发生了什么(ClientConn 视角)

一次 `cli.Echo(ctx, req)` 实际走:

1. 在共享 ClientConn 上取/开一条 HTTP/2 Stream(stream ID = N)。
2. 发送 `HEADERS` 帧:包含路径 `/echo.EchoService/Echo`、超时、metadata。
3. 发送 `DATA` 帧:EchoReq 序列化后的 protobuf 字节。
4. **半关闭发送方向**(End Stream),等待响应。
5. 收到对端 `HEADERS` + `DATA`,反序列化出 EchoResp。
6. 流结束,stream ID 回收(后续可复用)。

### 4.4 服务端实现

```go
type echoServer struct{ echopb.UnimplementedEchoServiceServer }

func (s *echoServer) Echo(ctx context.Context, req *echopb.EchoReq) (*echopb.EchoResp, error) {
    return &echopb.EchoResp{Reply: "echo: " + req.Msg}, nil
}

srv := grpc.NewServer()
echopb.RegisterEchoServiceServer(srv, &echoServer{})
lis, _ := net.Listen("tcp", ":50051")
srv.Serve(lis)   // 阻塞,接受连接;每个 RPC 在独立 goroutine 执行
```

### 4.5 Unary 特征

- 语义简单:**一呼一应**,天然适配阻塞/超时模型(`ctx` 控制取消)。
- 服务端每次调用是"完整请求到达后才执行",执行期间不会收到第二个请求。
- 每次调用有建流/拆流的框架开销。
- 适合低频、离散、无状态调用。

---

## 5. Streaming 流式请求完整链路

### 5.1 proto 定义(双向流)

```protobuf
service ChatService {
    rpc Chat(stream ChatMsg) returns (stream ChatMsg);
}
message ChatMsg { string text = 1; }
```

### 5.2 客户端

```go
cli := chatpb.NewChatServiceClient(conn)
stream, err := cli.Chat(ctx)   // 建立双向流(阻塞到流建立)
if err != nil { ... }

// 下行:循环收(独立 goroutine)
go func() {
    for {
        msg, err := stream.Recv()
        if err == io.EOF { break }        // 对端半关闭
        if err != nil { /* 处理错误 */ break }
        handle(msg)
    }
}()

// 上行:按需发
_ = stream.Send(&chatpb.ChatMsg{Text: "hi"})
```

### 5.3 服务端

```go
func (s *chatServer) Chat(stream chatpb.ChatService_ChatServer) error {
    for {
        msg, err := stream.Recv()
        if err == io.EOF {
            return nil                    // 客户端半关闭,正常结束
        }
        if err != nil { return err }
        // 处理消息;可以随时 stream.Send() 主动推送
        if err := stream.Send(&chatpb.ChatMsg{Text: "reply: " + msg.Text}); err != nil {
            return err
        }
    }
}
```

### 5.4 底层发生了什么

- 客户端 `cli.Chat(ctx)` 建立**一条长期存在的 HTTP/2 Stream**。
- 流的生命周期内,`Send` / `Recv` 各自独立方向,**互不阻塞、可并发**。
- 每个 `Send(msg)` = 序列化后写一个 DATA 帧;每个 `Recv()` = 阻塞读下一个帧。
- 消息边界由 DATA 帧天然分隔,无需业务自定义分割协议。

### 5.5 Streaming 特征

- 一条流 = 一个**长期会话**,两端各有一个循环 Send/Recv。
- 服务端可以**主动推送**(Unary 做不到——服务端只能被动应答)。
- 生命周期:任一侧 `CloseSend()`(半关闭发送方向)、ctx 取消、网络断开都会结束流。
- 高频交互时,握手成本只在流建立时发生一次,后续每帧开销极小。

### 5.6 判断流结束的信号

| 事件 | 客户端 `Recv()` 返回 | 服务端 `Recv()` 返回 |
|------|---------------------|---------------------|
| 服务端正常结束 | `io.EOF` | - |
| 客户端 `CloseSend()` | - | `io.EOF` |
| 对端网络断开 | `codes.Unavailable` / `Canceled` 等 | 同左 |
| 主动取消 ctx | `codes.Canceled` | `codes.Canceled` |

---

## 6. ClientConn、Client、Stream 三者关系

这是最容易混淆的部分。回答"这个 client 是每玩家建一个还是全局一个"这类问题,必须先分清三者:

```
grpc.ClientConn(重量级)         grpc.Client(极轻)            Stream(会话)
┌─────────────────────┐       ┌─────────────┐       ┌─────────────────────┐
│ TCP 连接             │       │ 只是包一层    │       │ 一条 HTTP/2 流       │
│ HTTP/2 帧调度 goroutine│      │ cc 指针      │       │ Send/Recv 循环      │
│ 连接池/重连/负载均衡  │       │ 无独立资源   │       │ 生命周期=会话       │
│ 每节点全局一份        │       │ 可反复创建   │       │ 每会话一条          │
└─────────────────────┘       └─────────────┘       └─────────────────────┘
        ▲                            ▲                        ▲
        └──── 共享 ────────────────────┘                        └─ 唯一不可共享

  NewXxxClient(conn) 只是 &xxxClient{cc: conn},零成本包装
  每次 Unary 调用 或 NewStream 都复用它
```

### 关键结论

1. **`grpc.ClientConn` 是真正的重量级资源**:包含 TCP 连接、HTTP/2 调度 goroutine、负载均衡、重连。**必须全局共享一份**(或按对端节点共享一份)。
2. **`NewXxxClient(conn)` 是零成本包装**:一个 8 字节指针的堆分配。每请求/每对象创建都无所谓,性能上无差别。
3. **`Stream`(由 `cli.Stream(ctx)` 或 `cli.NewStream` 创建)才是真正"每会话独享"的资源**:`SendMsg` 并发不安全,不能多 goroutine 同发一条流。
4. 全局共享一个 `Client`,依然可以为每个会话 `cli.Stream()` 开独立流——**client 不限制流数量**。

---

## 7. 生命周期与连接管理

### 7.1 连接建立

```go
// 老 API(grpc.Dial,1.62 前)
conn, err := grpc.Dial(target,
    grpc.WithTransportCredentials(creds),
    grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64<<20)))

// 新 API(grpc.NewClient,建议)
conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
```

- `grpc.Dial` 默认**异步连接**(不阻塞);`grpc.WithBlock()` 可改为阻塞等待连接就绪(生产慎用,会导致启动卡住)。
- `grpc.NewClient` 是 lazy 连接,首次 RPC 时才真正建立连接。

### 7.2 重连与断连

- ClientConn 内建**自动重连**:连接断开后按 backoff 策略重试。
- 对端进程重启、网络闪断,同一 ClientConn 会透明恢复,不需要业务重建。
- 应用层通常无需处理连接层错误,只需处理 RPC 返回的错误码。

### 7.3 关闭

```go
defer conn.Close()   // 关闭连接,等待 in-flight RPC 结束(可带 ctx 版本)
```

- `conn.Close()` 会关闭所有活跃流,挂起的 RPC 返回错误。
- 服务端 `srv.GracefulStop()`:停止接受新连接/新 RPC,等存量 RPC 完成;`srv.Stop()` 强制立即关闭。

### 7.4 负载均衡

- 客户端可通过 `grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}")` 启用轮询。
- 基于服务发现(etcd / DNS / consul)解析出多个后端地址,在连接层面分配请求。

---

## 8. 超时、取消与元数据

### 8.1 超时

```go
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()
resp, err := cli.Unary(ctx, req)   // 超时返回 codes.DeadlineExceeded
```

- 超时信息通过 `grpc-timeout` 头传给服务端;服务端用 `ctx.Done()` 感知。
- **客户端超时对服务端生效**:服务端可检查 `ctx.Err()` 提前终止重活。

### 8.2 取消

```go
ctx, cancel := context.WithCancel(context.Background())
go func() { time.Sleep(time.Second); cancel() }()   // 任意时刻取消
```

- 取消会向对端发 `RST_STREAM`(或对应的取消帧),对端 `Recv()` 返回 `codes.Canceled`。
- 流式场景取消后,流不可再复用,需重新 `cli.Stream()`。

### 8.3 元数据(Metadata)

```go
// 客户端附加
md := metadata.Pairs("authorization", "token-123")
ctx = metadata.NewOutgoingContext(ctx, md)
cli.Call(ctx, req)

// 服务端读取
md, _ := metadata.FromIncomingContext(ctx)
token := md.Get("authorization")
```

- 适合传 trace ID、鉴权 token、语言等**非业务负载**信息。
- metadata 会写入 HEADERS 帧,参与 HPACK 压缩。

---

## 9. 常见陷阱与最佳实践

### 9.1 陷阱

| 陷阱 | 说明与规避 |
|------|-----------|
| **每请求创建 ClientConn** | 每请求建 TCP+HTTP/2,资源爆炸。ClientConn 必须全局复用 |
| **多 goroutine 同时 Send 一条流** | `SendMsg` 并发不安全,会数据交错/报错。每条流串行 Send,或用单独 send goroutine + channel |
| **忘记处理流错误 / EOF** | `Recv()` 的错误要分类处理:EOF 正常结束,其余按错误码处理,避免僵尸 goroutine |
| **消息过大** | 默认收发 4MB。超限返回 `codes.ResourceExhausted`。用 `grpc.MaxCallRecvMsgSize` / `grpc.MaxRecvMsgSize` 调大 |
| **连接状态被忽略** | 应用层别假定连接永久可用;Unary 有 error 返回,流有 Recv 错误,都要兜底 |
| **超时设置缺失** | 不设超时的阻塞调用可能永久挂起。**所有 RPC 都该带 ctx 超时/取消** |
| **proto 字段变更破坏兼容** | 已发布的字段号/类型不能改;新增字段加新编号;废弃字段用 reserved |
| **流不关闭泄漏 goroutine** | 流式 RPC 双方都要保证"正常结束就退出循环",否则 Recv goroutine 泄漏 |

### 9.2 最佳实践

- **ClientConn 全局单例**,按对端节点/服务粒度共享。
- **每个 RPC 都带 ctx 超时**,可配 `grpc.WithDefaultCallOptions` 设默认。
- **Unary 用于低频离散调用;长连接/推送用流式**。
- **区分错误码**:`DeadlineExceeded` 重试无意义;`Unavailable` 可短退避重试。
- 大响应用流式分块;大批量上报用客户端流。
- 生产环境启用 `grpc.ConnectivityStateIndicator` 或健康检查监控连接状态。

---

## 10. 调试手段

### 10.1 客户端调试开关

```go
// 环境变量:打印完整 HTTP/2 帧
GRPC_TRACE=all GRPC_VERBOSITY=DEBUG ./your_server
GRPC_TRACE=http2 GRPC_VERBOSITY=INFO ./your_client
```

常用 trace 项:`http2`(帧)、`transport`(连接)、`api`(调用)、`streaming`。

### 10.2 服务器端

```go
grpc.ChainUnaryInterceptor(loggingInterceptor)
func loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
    log.Printf("call: %v", info.FullMethod)
    return handler(ctx, req)
}
```

拦截器统一做日志、鉴权、限流、trace 注入,是最常用的服务端扩展点。

### 10.3 工具

- **grpcurl**:命令行调用 gRPC 服务,`grpcurl -plaintext localhost:50051 list`。
- **grpcui**:gRPC 的图形化调试界面。
- **tcpdump / wireshark**:抓 HTTP/2 帧(需开启 HTTP/2 解析)。

---

## 附:一页速查

| 问题 | 答案 |
|------|------|
| gRPC 底层协议 | HTTP/2(帧、流、多路复用、HPACK) |
| 有几种 RPC 模式 | 4 种:Unary / Server-streaming / Client-streaming / Bidi |
| ClientConn | 重量级,全局共享一份 |
| Client(NewXxxClient) | 零成本包装,可反复创建 |
| Stream | 每会话一条,不可共享,Send 需串行 |
| 服务端如何推送 | 用流式(server-streaming 或 bidi),Unary 无法主动推 |
| 消息默认大小上限 | 4MB,可用 MaxCallRecvMsgSize 调整 |
| 所有调用必须带 | ctx(超时/取消) |
| 连接断了怎么办 | ClientConn 自动重连,业务按错误码处理 |
| 如何判断流结束 | Recv() 返回 io.EOF(正常)/ 非 EOF(异常) |
