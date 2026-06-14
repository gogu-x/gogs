// cmd/testclient/main.go
// 压测客户端：模拟 N 个并发用户登录并持续发消息，统计成功率与吞吐量。
package main

import (
	"flag"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/pb/protoChat"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/gogs/pb/protoGuild"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

var (
	addr     = flag.String("addr", "ws://127.0.0.1:8081/ws", "gate websocket address")
	users    = flag.Int("users", 10000, "concurrent users")
	duration = flag.Duration("duration", 100*time.Second, "test duration")
	interval = flag.Duration("interval", 3*time.Second, "message send interval per user")
	dialRate = flag.Int("dial-rate", 1000, "max concurrent dials per second")
)

func init() {
	codec.RegisterMsg(
		&protoGateway.LoginReq{},
		&protoGateway.LoginResp{},
		&protoChat.ChatReq{},
		&protoGuild.GetGuildReq{},
	)
}

var (
	connOK    int64
	connFail  int64
	sendOK    int64
	sendFail  int64
	recvTotal int64

	failMu  sync.Mutex
	failMap = map[string]int{}
)

func runUser(uid uint64, wg *sync.WaitGroup, stop <-chan struct{}) {
	defer wg.Done()

	dialer := websocket.Dialer{
		Subprotocols:     []string{"protobuf"},
		HandshakeTimeout: 5 * time.Second,
	}
	conn, _, err := dialer.Dial(*addr, nil)
	if err != nil {
		atomic.AddInt64(&connFail, 1)
		failMu.Lock()
		// 只取错误关键词，避免 map key 爆炸
		key := err.Error()
		if len(key) > 60 {
			key = key[:60]
		}
		failMap[key]++
		failMu.Unlock()
		return
	}
	atomic.AddInt64(&connOK, 1)
	defer conn.Close()

	// 接收协程
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
			atomic.AddInt64(&recvTotal, 1)
		}
	}()

	send := func(msg proto.Message) bool {
		data, err := codec.ProtoCodec.Marshal(msg)
		if err != nil {
			return false
		}
		conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			return false
		}
		return true
	}

	// 登录
	if !send(&protoGateway.LoginReq{Uid: uid, Token: "test-token", ServerId: 1}) {
		atomic.AddInt64(&sendFail, 1)
		return
	}
	atomic.AddInt64(&sendOK, 1)
	time.Sleep(200 * time.Millisecond)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if send(&protoChat.ChatReq{Type: 1, Content: fmt.Sprintf("hi from %d", uid)}) {
				atomic.AddInt64(&sendOK, 1)
			} else {
				atomic.AddInt64(&sendFail, 1)
				return
			}
		}
	}
}

func main() {
	flag.Parse()
	log.Printf("压测开始: users=%d duration=%v interval=%v addr=%s", *users, *duration, *interval, *addr)

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// 令牌桶限速建连：每秒最多 dialRate 个并发握手
	dialTicker := time.NewTicker(time.Second / time.Duration(*dialRate))
	defer dialTicker.Stop()
	for i := 0; i < *users; i++ {
		<-dialTicker.C
		wg.Add(1)
		go runUser(uint64(100000+i), &wg, stop)
	}

	// 定时打印统计
	ticker := time.NewTicker(5 * time.Second)
	start := time.Now()
	go func() {
		for range ticker.C {
			elapsed := time.Since(start).Seconds()
			ok := atomic.LoadInt64(&sendOK)
			fail := atomic.LoadInt64(&sendFail)
			total := ok + fail
			rate := 0.0
			if total > 0 {
				rate = float64(ok) / float64(total) * 100
			}
			log.Printf("[%ds] 连接: 成功=%d 失败=%d | 发送: 成功=%d 失败=%d 成功率=%.1f%% | 收到=%d | 吞吐=%.0f msg/s",
				int(elapsed),
				atomic.LoadInt64(&connOK), atomic.LoadInt64(&connFail),
				ok, fail, rate,
				atomic.LoadInt64(&recvTotal),
				float64(ok)/elapsed,
			)
		}
	}()

	time.Sleep(*duration)
	close(stop)
	ticker.Stop()

	wg.Wait()

	// 最终统计
	elapsed := time.Since(start).Seconds()
	ok := atomic.LoadInt64(&sendOK)
	fail := atomic.LoadInt64(&sendFail)
	total := ok + fail
	rate := 0.0
	if total > 0 {
		rate = float64(ok) / float64(total) * 100
	}
	log.Printf("===== 压测结束 =====")
	log.Printf("耗时:     %.1fs", elapsed)
	log.Printf("建连成功: %d  失败: %d", atomic.LoadInt64(&connOK), atomic.LoadInt64(&connFail))
	log.Printf("发送成功: %d  失败: %d  成功率: %.2f%%", ok, fail, rate)
	log.Printf("收到消息: %d", atomic.LoadInt64(&recvTotal))
	log.Printf("平均吞吐: %.0f msg/s", float64(ok)/elapsed)
	if len(failMap) > 0 {
		log.Printf("建连失败原因:")
		for k, v := range failMap {
			log.Printf("  [%d次] %s", v, k)
		}
	}
}
