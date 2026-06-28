// cmd/functest/main.go
// 压测：N 个并发用户各自注册 → 登录 → 发 Chat，统计成功率与耗时。
// 用法: go run ./cmd/functest -addr ws://127.0.0.1:8081/ws -server-id 1 -users 10000
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogu-x/gogs/codec"
	_ "github.com/gogu-x/gogs/pb/pbregister"
	"github.com/gogu-x/gogs/pb/protoChat"
	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

var (
	addrs       = flag.String("addrs", "ws://127.0.0.1:8081/ws,ws://127.0.0.1:8082/ws", "gate websocket addresses (comma-separated)")
	serverID    = flag.Int("server-id", 1, "game server id")
	users       = flag.Int("users", 20000, "total user count")
	concurrency = flag.Int("c", 1000, "max concurrent connections")
	dialRate    = flag.Int("dial-rate", 5000, "max new dials per second")
	timeout     = flag.Duration("timeout", 10*time.Second, "per-message read timeout")

	gateAddrs []string
)

// ── 轻量 ws 客户端 ────────────────────────────────────────────────────────────

type client struct{ conn *websocket.Conn }

var dialer = websocket.Dialer{
	Subprotocols:     []string{"protobuf"},
	HandshakeTimeout: 5 * time.Second,
}

func dial(idx int) (*client, error) {
	target := gateAddrs[idx%len(gateAddrs)] // 轮询分配到各 Gate
	var (
		conn *websocket.Conn
		err  error
	)
	for i := range 3 {
		conn, _, err = dialer.Dial(target, nil)
		if err == nil {
			return &client{conn}, nil
		}
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}
	return nil, err
}

func (c *client) close() { c.conn.Close() }

func (c *client) send(msg proto.Message) error {
	data, err := codec.ProtoCodec.Marshal(msg)
	if err != nil {
		return err
	}
	c.conn.SetWriteDeadline(time.Now().Add(*timeout))
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *client) recv() (proto.Message, error) {
	c.conn.SetReadDeadline(time.Now().Add(*timeout))
	_, data, err := c.conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	msg, err := codec.ProtoCodec.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	pm, ok := msg.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("not proto.Message: %T", msg)
	}
	return pm, nil
}

// ── 单用户流程 ────────────────────────────────────────────────────────────────

func runUser(idx int) error {
	c, err := dial(idx)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer c.close()

	account := fmt.Sprintf("bench_%d_%d", time.Now().UnixNano(), idx)
	sid := int32(*serverID)

	// 注册
	if err := c.send(&protoGateway.RegisterReq{Account: account, Password: "Test@1234", ServerId: sid}); err != nil {
		return fmt.Errorf("register send: %w", err)
	}
	msg, err := c.recv()
	if err != nil {
		return fmt.Errorf("register recv: %w", err)
	}
	if ack, ok := msg.(*protoGateway.RegisterAck); !ok || ack.Code != protoCommon.ErrCode_OK {
		return fmt.Errorf("register ack: %v", msg)
	}

	// 登录
	if err := c.send(&protoGateway.LoginReq{Account: account, Password: "Test@1234", ServerId: sid}); err != nil {
		return fmt.Errorf("login send: %w", err)
	}
	msg, err = c.recv()
	if err != nil {
		return fmt.Errorf("login recv: %w", err)
	}
	if ack, ok := msg.(*protoGateway.LoginAck); !ok || ack.Code != protoCommon.ErrCode_OK {
		return fmt.Errorf("login ack: %v", msg)
	}

	// Chat
	if err := c.send(&protoChat.ChatReq{Type: 1, Content: "bench"}); err != nil {
		return fmt.Errorf("chat send: %w", err)
	}

	return nil
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	flag.Parse()
	gateAddrs = strings.Split(*addrs, ",")
	log.Printf("压测开始: addrs=%v users=%d", gateAddrs, *users)

	var (
		wg      sync.WaitGroup
		success int64
		failure int64
		start   = time.Now()
		sem     = make(chan struct{}, *concurrency)

		errMu    sync.Mutex
		errCount = map[string]int{}
	)

	// dialRate 限速：每秒最多建 dialRate 个新连接，避免瞬间冲击 OS backlog
	ticker := time.NewTicker(time.Second / time.Duration(*dialRate))
	defer ticker.Stop()

	wg.Add(*users)
	for i := range *users {
		<-ticker.C // 控制建连速率
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := runUser(idx); err != nil {
				atomic.AddInt64(&failure, 1)
				// 按错误前缀（"dial:"/"register recv:"等）分类计数
				key := err.Error()
				if idx := len(key); idx > 40 {
					key = key[:40]
				}
				errMu.Lock()
				errCount[key]++
				errMu.Unlock()
			} else {
				n := atomic.AddInt64(&success, 1)
				if n%int64(*dialRate) == 0 {
					log.Printf("progress: %d/%d done", n, *users)
				}
			}
		}(i)
	}
	wg.Wait()

	elapsed := time.Since(start)

	total := success + failure
	log.Printf("===== 压测结束 =====")
	log.Printf("总用户: %d  成功: %d  失败: %d  耗时: %v  QPS: %.0f",
		total, success, failure, elapsed, float64(total)/elapsed.Seconds())
	for msg, cnt := range errCount {
		log.Printf("  失败[%d]: %s", cnt, msg)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
}
