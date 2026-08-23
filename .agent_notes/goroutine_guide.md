# Goroutine 从基础到进阶

## 一、基础概念

### 1.1 什么是 goroutine

goroutine 是 Go 运行时（runtime）调度的轻量级执行单元，不是操作系统线程。可以理解为"用户态协程"：

- 创建成本极低：初始栈只有 2KB（会按需自动增长/收缩，最大到 1GB 量级），因此可以轻松创建几十万个 goroutine。
- 由 Go runtime 的调度器（GMP 模型）负责调度到少量 OS 线程上运行，而不是一对一映射到线程。
- 通过 `go` 关键字启动，无需手动管理生命周期（但需要手动处理同步/退出逻辑）。

```go
go func() {
    fmt.Println("running in a goroutine")
}()
```

### 1.2 main goroutine 退出即程序结束

`main` 函数所在的 goroutine 退出时，整个程序立即终止，不会等待其他 goroutine 跑完。

```go
func main() {
    go fmt.Println("可能永远不会被打印")
    // main 立刻结束，上面的 goroutine 可能还没被调度执行
}
```

这是新手最容易踩的坑：必须显式等待（`sync.WaitGroup`、channel、`time.Sleep` 仅用于临时调试）。

### 1.3 GMP 调度模型（简要）

- **G (Goroutine)**：待执行的任务，包含栈、程序计数器等。
- **M (Machine)**：OS 线程，真正执行代码的实体。
- **P (Processor)**：逻辑处理器，持有可运行 G 的本地队列，M 必须绑定 P 才能执行 G。

`GOMAXPROCS` 控制 P 的数量，默认等于 CPU 核数。调度器会在 G 发生阻塞（如系统调用、channel 阻塞）时，将 M 与 P 解绑，让其他 M 接管 P 继续跑其他 G，从而实现高并发下的高效调度。这也是为什么 goroutine 阻塞在 channel 上代价很低——不会阻塞底层线程本身。

---

## 二、基础用法

### 2.1 闭包变量捕获陷阱（经典坑）

```go
// 错误示例：Go 1.21 及之前，for 循环变量是共享的
for i := 0; i < 3; i++ {
    go func() {
        fmt.Println(i) // 可能全部打印 3，而不是 0,1,2
    }()
}

// 正确写法1：显式传参
for i := 0; i < 3; i++ {
    go func(n int) {
        fmt.Println(n)
    }(i)
}

// 正确写法2：循环内重新声明局部变量
for i := 0; i < 3; i++ {
    i := i
    go func() {
        fmt.Println(i)
    }()
}
```

> 注意：Go 1.22 起，`for` 循环每次迭代会创建新的变量作用域，这个坑在新版本里已经不存在了，但了解原理仍然重要（很多存量代码库还在用旧语义或习惯这种写法）。

### 2.2 sync.WaitGroup：等待一组 goroutine 完成

（已在之前的问答中详细讲过，此处简要回顾）

```go
var wg sync.WaitGroup
for i := 0; i < n; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        doWork(id)
    }(i)
}
wg.Wait()
```

要点：
- `Add` 必须在 `go` 之前调用，且不能与 `Wait()` 并发竞争。
- `Done()` 用 `defer` 保证即使 panic 也会执行。

### 2.3 channel：goroutine 间通信

Go 的哲学是"不要用共享内存来通信，要用通信来共享内存"（Don't communicate by sharing memory; share memory by communicating）。

```go
ch := make(chan int)       // 无缓冲，发送阻塞直到有人接收
ch := make(chan int, 10)   // 有缓冲，缓冲区满前不阻塞

// 生产者-消费者
go func() {
    for i := 0; i < 5; i++ {
        ch <- i
    }
    close(ch) // 通知消费者没有更多数据
}()

for v := range ch { // channel 关闭且清空后自动退出循环
    fmt.Println(v)
}
```

关键规则：
- 向已关闭的 channel 发送数据会 panic。
- 从已关闭的 channel 接收，会立即返回该类型的零值，可用 `v, ok := <-ch` 中的 `ok` 判断是否已关闭。
- 只有发送方应该关闭 channel，不要在接收方关闭（否则容易导致 panic）。

### 2.4 select：多路复用 + 非阻塞操作

```go
select {
case v := <-ch1:
    fmt.Println("from ch1", v)
case ch2 <- data:
    fmt.Println("sent to ch2")
case <-time.After(time.Second):
    fmt.Println("timeout")
default:
    fmt.Println("非阻塞：所有 case 都不能立即执行时走这里")
}
```

- 多个 case 同时就绪时，随机选择一个执行（避免饥饿）。
- `default` 存在时，`select` 变成非阻塞操作（前面 BI 日志代码里 `select+default` 做非阻塞发送就是这个用法）。
- `time.After` 常用来给阻塞操作加超时保护，但注意每次调用都会创建一个新 timer，高频调用场景有性能开销，应改用 `time.NewTimer` + `Reset`（见 4.4）。

### 2.5 互斥锁：sync.Mutex / sync.RWMutex

保护共享状态的传统方式，配合 goroutine 使用：

```go
type Counter struct {
    mu    sync.Mutex
    count int
}

func (c *Counter) Inc() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.count++
}
```

- `RWMutex` 适合读多写少场景：`RLock`/`RUnlock` 允许多个读并发，`Lock`/`Unlock` 是独占写锁。
- 优先考虑 channel 传递数据而非共享变量+锁，但对性能敏感、简单计数器等场景，锁往往更直接高效。

---

## 三、进阶用法

### 3.1 context：超时、取消、传值的标准方案

```go
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()

go func() {
    select {
    case <-ctx.Done():
        fmt.Println("cancelled:", ctx.Err()) // context.DeadlineExceeded 或 context.Canceled
        return
    case result := <-doSomethingAsync():
        fmt.Println(result)
    }
}()
```

常用构造函数：
- `context.WithCancel(parent)`：手动调用返回的 `cancel()` 触发取消。
- `context.WithTimeout(parent, d)` / `context.WithDeadline(parent, t)`：超时自动取消。
- `context.WithValue(parent, key, value)`：传递请求范围内的元数据（不建议用来传递业务参数，只适合传递如 trace id、用户身份等横切数据）。

**父子传播**：取消父 context 会级联取消所有派生的子 context，反过来不会。这是构建"请求级联取消"（比如 HTTP 请求超时后，所有下游调用都应该停止）的核心机制。

### 3.2 errgroup：并发任务 + 错误传播

标准库没有内置，但 `golang.org/x/sync/errgroup` 是事实标准，弥补了 `WaitGroup` 不能收集错误、不能级联取消的短板：

```go
g, ctx := errgroup.WithContext(context.Background())

for _, url := range urls {
    url := url
    g.Go(func() error {
        return fetch(ctx, url) // 任意一个返回 error，ctx 会被自动 cancel
    })
}

if err := g.Wait(); err != nil {
    log.Println("first error:", err)
}
```

- 任意一个 `g.Go` 里的函数返回非 nil error，`g.Wait()` 就会返回第一个出现的 error。
- 配合 `WithContext` 时，一旦有 goroutine 出错，会自动 cancel 关联的 ctx，其他 goroutine 可以借此提前退出，避免资源浪费。

### 3.3 sync.Once：只执行一次的初始化

```go
var once sync.Once
var instance *Singleton

func GetInstance() *Singleton {
    once.Do(func() {
        instance = &Singleton{}
    })
    return instance
}
```

多个 goroutine 并发调用 `GetInstance()`，`once.Do` 内部的函数保证只被执行一次，且所有调用者会等待第一次执行完成后再返回，是并发安全的懒加载/单例标准写法。

### 3.4 定时器复用：time.Timer + Reset

高频循环里反复调用 `time.After` 会不断创建新的 timer 对象，造成 GC 压力，正确做法（也是你项目 `worker.go` 里用到的模式）：

```go
tickTimer := time.NewTimer(interval)
for {
    select {
    case <-tickTimer.C:
        tickTimer.Reset(interval) // 复用同一个 timer
        doPeriodicWork()
    case data := <-ch:
        handle(data)
    }
}
```

注意 `Reset` 前如果 timer 可能还没被 `<-tickTimer.C` 消费过，要先 `Stop()` 并判断返回值，避免竞态（Go 1.23 之后这个问题已大幅简化，但要留意所用 Go 版本）。

### 3.5 worker pool（工作池）模式

限制并发数，避免无限制创建 goroutine 压垮系统资源：

```go
func workerPool(tasks <-chan Task, workerCount int) {
    var wg sync.WaitGroup
    for i := 0; i < workerCount; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for task := range tasks { // 多个 worker 竞争同一个 channel，天然实现负载均衡
                task.Do()
            }
        }()
    }
    wg.Wait()
}
```

这也是你项目 BI 日志模块用的核心模式：固定数量的 `logBIWorker` goroutine 各自消费自己的 channel。

### 3.6 pipeline（流水线）模式

把处理过程拆成多个阶段，通过 channel 串联，每个阶段可以并发处理：

```go
func gen(nums ...int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for _, n := range nums {
            out <- n
        }
    }()
    return out
}

func square(in <-chan int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for n := range in {
            out <- n * n
        }
    }()
    return out
}

// 使用: for v := range square(gen(1,2,3,4)) { fmt.Println(v) }
```

要点：每个阶段负责关闭自己产出的 channel，形成"谁生产谁关闭"的一致约定，下游用 `range` 自然感知上游结束。

### 3.7 fan-out / fan-in：并发扇出与合并

```go
func fanIn(chans ...<-chan int) <-chan int {
    out := make(chan int)
    var wg sync.WaitGroup
    wg.Add(len(chans))
    for _, c := range chans {
        go func(c <-chan int) {
            defer wg.Done()
            for v := range c {
                out <- v
            }
        }(c)
    }
    go func() {
        wg.Wait()
        close(out) // 所有输入 channel 消费完才关闭合并后的输出
    }()
    return out
}
```

用于把一个任务拆给多个 goroutine 并行处理（fan-out），再把结果汇聚到一个 channel（fan-in）。

### 3.8 优雅关闭 / 退出信号传播

结合 `context` 取消 + `WaitGroup` 等待，是生产级服务的标准退出方案：

```go
func run(ctx context.Context) {
    var wg sync.WaitGroup
    for i := 0; i < 3; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            for {
                select {
                case <-ctx.Done():
                    return // 收到取消信号，清理并退出
                case job := <-jobs:
                    process(job)
                }
            }
        }(i)
    }
    wg.Wait()
}
```

这跟你项目里 `close(channel)` 触发 worker 退出、再用 `wg.Wait()` 等待的模式是同一思路，只是取消信号换成了更标准的 `context.Done()`。

### 3.9 goroutine 泄漏与检测

常见泄漏原因：
- 向无缓冲/已满的 channel 发送数据，但没有任何 goroutine 会接收 → 发送方永久阻塞。
- `for range channel` 但 channel 永远不会被关闭。
- 启动的 goroutine 忘记设置退出条件（没有 `ctx.Done()` 或退出 channel）。

排查手段：
- `runtime.NumGoroutine()` 监控数量是否持续增长。
- `pprof`（`net/http/pprof` 或 `runtime/pprof`）抓 goroutine profile，看阻塞栈。
- go vet / staticcheck 等静态检查工具能发现部分明显的泄漏模式。

### 3.10 竞态检测

开发和 CI 阶段应始终用 race detector 跑测试：

```bash
go test -race ./...
go run -race main.go
```

它能捕获大部分未加锁访问共享变量的场景，是排查"偶发性错误/数据错乱"问题的第一步。

---

## 四、常见反模式与最佳实践总结

| 反模式 | 问题 | 正确做法 |
|---|---|---|
| 无限制 `go func(){}()` 处理海量任务 | 瞬间创建过多 goroutine，内存/调度开销暴涨 | 用 worker pool 限制并发数 |
| 忘记 `wg.Wait()` 或忘记 `Add` | 主流程提前退出，数据丢失或 panic | Add 在 go 之前，Done 用 defer |
| 在 goroutine 内部读写外部共享变量不加锁 | 数据竞争，行为未定义 | 用 channel 传递数据，或加 mutex |
| 用 `time.Sleep` 等待 goroutine 完成 | 不可靠，要么等太久要么不够 | 用 WaitGroup / channel / context |
| 忘记处理 panic | 一个 goroutine panic 会导致整个进程崩溃（不像其他语言的线程隔离） | 每个长期运行的 goroutine 顶部加 `defer recover()` |
| 接收方关闭 channel | 后续发送会 panic | 只由发送方关闭 |

---

## 五、与本项目代码的对照

`game/bi` 模块（`logger.go` / `log_mgr.go` / `worker.go`）体现了以下几个进阶模式的组合：

1. **worker pool**：每个日志文件对应一个 `logBIWorker` goroutine，长期运行消费自己的 `biChan`。
2. **select + default 非阻塞发送**：`WriteChan` 里避免写日志阻塞主逻辑（对应之前讨论的"是否阻塞"问题）。
3. **定时器复用**：`worker.go` 里 `tickTimer.Reset(timerTick)` 避免重复创建 timer。
4. **WaitGroup 优雅关闭**：`BILogMgr.wg` 保证进程退出前所有日志 worker 处理完 channel 里剩余数据（对应 channel 关闭后 `Done()` 上报）。

如果后续要改进，可以考虑：
- 给每个 worker 的 `defer recover()` 里补充监控上报（目前只打日志），防止某个 worker panic 后无声无息地少了一个消费者。
- `WriteChan` 非阻塞丢弃时增加监控指标（当前只有 `tlog.Errorf`），方便观察丢失率。
