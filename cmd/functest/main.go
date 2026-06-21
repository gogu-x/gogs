// cmd/functest/main.go
// 压测：N 个并发用户各自注册 → 登录 → 发 Chat，统计成功率与耗时。
// 用法: go run ./cmd/functest -addr ws://127.0.0.1:8081/ws -server-id 1 -users 10000
package main

import (
	"flag"
	"fmt"
	"log"
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
	addr        = flag.String("addr", "ws://127.0.0.1:8081/ws", "gate websocket address")
	serverID    = flag.Int("server-id", 1, "game server id")
	users       = flag.Int("users", 10000, "total user count")
	concurrency = flag.Int("c", 200, "max concurrent connections")
	timeout     = flag.Duration("timeout", 5*time.Second, "per-message read timeout")
)

// ── 轻量 ws 客户端 ────────────────────────────────────────────────────────────

type client struct{ conn *websocket.Conn }

var dialer = websocket.Dialer{
	Subprotocols:     []string{"protobuf"},
	HandshakeTimeout: 5 * time.Second,
}

func dial() (*client, error) {
	conn, _, err := dialer.Dial(*addr, nil)
	if err != nil {
		return nil, err
	}
	return &client{conn}, nil
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
	c, err := dial()
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
	log.Printf("压测开始: addr=%s users=%d", *addr, *users)

	var (
		wg         sync.WaitGroup
		success    int64
		failure    int64
		start      = time.Now()
		sampleOnce sync.Once
		sem        = make(chan struct{}, *concurrency)
	)

	for i := range *users {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := runUser(idx); err != nil {
				atomic.AddInt64(&failure, 1)
				sampleOnce.Do(func() { log.Printf("sample error: %v", err) })
			} else {
				n := atomic.AddInt64(&success, 1)
				if n%500 == 0 {
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
}
