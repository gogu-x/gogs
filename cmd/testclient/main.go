// cmd/testclient/main.go
// 测试客户端：使用 protobuf 协议连接 Gate WebSocket
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/pb/protoChat"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/gogs/pb/protoGuild"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

var addr = flag.String("addr", "ws://127.0.0.1:8081/ws", "gate websocket address")

func init() {
	codec.RegisterMsg(
		&protoGateway.LoginReq{},
		&protoGateway.LoginResp{},
		&protoChat.ChatReq{},
		&protoGuild.GetGuildReq{},
	)
}

func main() {
	flag.Parse()

	dialer := websocket.Dialer{Subprotocols: []string{"protobuf"}}
	conn, _, err := dialer.Dial(*addr, nil)
	if err != nil {
		log.Fatalf("dial error: %v", err)
	}
	defer conn.Close()
	log.Printf("connected to %s (subprotocol: %s)", *addr, conn.Subprotocol())

	// 接收协程
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				log.Printf("recv closed: %v", err)
				return
			}
			msg, err := codec.ProtoCodec.Unmarshal(data)
			if err != nil {
				log.Printf("<<< unmarshal error: %v (raw %d bytes)", err, len(data))
				continue
			}
			log.Printf("<<< %T: %+v", msg, msg)
		}
	}()

	send := func(msg proto.Message) {
		data, err := codec.ProtoCodec.Marshal(msg)
		if err != nil {
			log.Fatalf("marshal error: %v", err)
		}
		log.Printf(">>> %T", msg)
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			log.Fatalf("send error: %v", err)
		}
	}

	// 1. 登录
	send(&protoGateway.LoginReq{Uid: 2001, Token: "test-token", ServerId: 1})
	time.Sleep(500 * time.Millisecond)

	// 2. 聊天
	send(&protoChat.ChatReq{Type: 1, Content: "hello gogs"})

	//工会
	send(&protoGuild.GetGuildReq{GuildId: 1})
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	fmt.Println("bye")
}
