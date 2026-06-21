// cmd/functest/main.go
// 功能测试：注册 → 登录 → 其他消息，验证服务端响应正确性。
// 用法: go run ./cmd/functest -addr ws://127.0.0.1:8081/ws -server-id 1
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/gogu-x/gogs/codec"
	_ "github.com/gogu-x/gogs/pb/pbregister"
	"github.com/gogu-x/gogs/pb/protoChat"
	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/gogs/pb/protoGuild"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

var (
	addr     = flag.String("addr", "ws://127.0.0.1:8081/ws", "gate websocket address")
	serverID = flag.Int("server-id", 1, "game server id")
	timeout  = flag.Duration("timeout", 3*time.Second, "per-message read timeout")
)


type client struct {
	conn *websocket.Conn
}

func dial() (*client, error) {
	d := websocket.Dialer{
		Subprotocols:     []string{"protobuf"},
		HandshakeTimeout: 5 * time.Second,
	}
	conn, _, err := d.Dial(*addr, nil)
	if err != nil {
		return nil, err
	}
	return &client{conn: conn}, nil
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
		return nil, fmt.Errorf("decoded msg is not proto.Message: %T", msg)
	}
	return pm, nil
}

var (
	testUser   = fmt.Sprintf("functest_%d", time.Now().UnixNano())
	testPass   = "Test@1234"
	pass, fail int
)

func check(name string, err error) {
	if err != nil {
		log.Printf("FAIL  [%s]: %v", name, err)
		fail++
	} else {
		log.Printf("PASS  [%s]", name)
		pass++
	}
}

func main() {
	flag.Parse()
	log.Printf("功能测试开始: addr=%s server-id=%d user=%s", *addr, *serverID, testUser)

	c, err := dial()
	if err != nil {
		log.Fatalf("dial failed: %v", err)
	}
	defer c.close()

	// 1. 注册新用户
	check("注册新用户", func() error {
		if err := c.send(&protoGateway.RegisterReq{Account: testUser, Password: testPass, ServerId: int32(*serverID)}); err != nil {
			return err
		}
		msg, err := c.recv()
		if err != nil {
			return err
		}
		resp, ok := msg.(*protoGateway.RegisterAck)
		if !ok {
			return fmt.Errorf("unexpected type: %T", msg)
		}
		if resp.Code != protoCommon.ErrCode_OK {
			return fmt.Errorf("code=%v msg=%s", resp.Code, resp.Msg)
		}
		return nil
	}())

	// 2. 重复注册
	check("重复注册返回 ERR_USERNAME_EXISTS", func() error {
		if err := c.send(&protoGateway.RegisterReq{Account: testUser, Password: testPass, ServerId: int32(*serverID)}); err != nil {
			return err
		}
		msg, err := c.recv()
		if err != nil {
			return err
		}
		resp, ok := msg.(*protoGateway.RegisterAck)
		if !ok {
			return fmt.Errorf("unexpected type: %T", msg)
		}
		if resp.Code != protoCommon.ErrCode_ERR_USERNAME_EXISTS {
			return fmt.Errorf("expected ERR_USERNAME_EXISTS, got code=%v msg=%s", resp.Code, resp.Msg)
		}
		return nil
	}())

	// 3. 登录
	check("正确密码登录", func() error {
		if err := c.send(&protoGateway.LoginReq{Account: testUser, Password: testPass, ServerId: int32(*serverID)}); err != nil {
			return err
		}
		msg, err := c.recv()
		if err != nil {
			return err
		}
		resp, ok := msg.(*protoGateway.LoginAck)
		if !ok {
			return fmt.Errorf("unexpected type: %T", msg)
		}
		if resp.Code != protoCommon.ErrCode_OK {
			return fmt.Errorf("code=%v msg=%s", resp.Code, resp.Msg)
		}
		return nil
	}())

	// 4. 发 ChatReq
	check("登录后发 ChatReq", c.send(&protoChat.ChatReq{Type: 1, Content: "hello functest"}))

	// 5. 查询公会
	check("登录后查询公会", func() error {
		if err := c.send(&protoGuild.GetGuildReq{GuildId: 0}); err != nil {
			return err
		}
		msg, err := c.recv()
		if err != nil {
			return err
		}
		resp, ok := msg.(*protoGuild.GetGuildAck)
		if !ok {
			return fmt.Errorf("unexpected type: %T", msg)
		}
		if resp.Code != protoCommon.ErrCode_OK && resp.Code != protoCommon.ErrCode_ERR_GUILD_NOT_FOUND {
			return fmt.Errorf("unexpected code=%v", resp.Code)
		}
		return nil
	}())

	// 6. 错误密码登录
	check("错误密码登录返回 ERR_WRONG_PASSWORD", func() error {
		c2, err := dial()
		if err != nil {
			return err
		}
		defer c2.close()
		if err := c2.send(&protoGateway.LoginReq{Account: testUser, Password: "wrongpass", ServerId: int32(*serverID)}); err != nil {
			return err
		}
		msg, err := c2.recv()
		if err != nil {
			return err
		}
		resp, ok := msg.(*protoGateway.LoginAck)
		if !ok {
			return fmt.Errorf("unexpected type: %T", msg)
		}
		if resp.Code != protoCommon.ErrCode_ERR_WRONG_PASSWORD {
			return fmt.Errorf("expected ERR_WRONG_PASSWORD, got code=%v msg=%s", resp.Code, resp.Msg)
		}
		return nil
	}())

	// 7. 不存在的 server
	check("不存在的 server 返回 ERR_SERVER_NOT_FOUND", func() error {
		c3, err := dial()
		if err != nil {
			return err
		}
		defer c3.close()
		if err := c3.send(&protoGateway.LoginReq{Account: testUser, Password: testPass, ServerId: 9999}); err != nil {
			return err
		}
		msg, err := c3.recv()
		if err != nil {
			return err
		}
		resp, ok := msg.(*protoGateway.LoginAck)
		if !ok {
			return fmt.Errorf("unexpected type: %T", msg)
		}
		if resp.Code != protoCommon.ErrCode_ERR_SERVER_NOT_FOUND {
			return fmt.Errorf("expected ERR_SERVER_NOT_FOUND, got code=%v msg=%s", resp.Code, resp.Msg)
		}
		return nil
	}())

	log.Printf("===== 测试结束: PASS=%d FAIL=%d =====", pass, fail)
	if fail > 0 {
		log.Fatal("有测试用例失败")
	}
}
