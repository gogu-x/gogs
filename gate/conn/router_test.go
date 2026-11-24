package conn

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "github.com/gogu-x/gogs/pb"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_equipment"
	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gorilla/websocket"
)

// dialTestSocket 返回网关侧 WebSocket 连接，以及读取客户端收到消息的通道。
func dialTestSocket(t *testing.T) (*websocket.Conn, <-chan []byte) {
	t.Helper()
	serverSide := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		serverSide <- c
	}))
	t.Cleanup(srv.Close)
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	received := make(chan []byte, 4)
	go func() {
		for {
			_, data, err := client.ReadMessage()
			if err != nil {
				return
			}
			received <- data
		}
	}()
	gateSide := <-serverSide
	t.Cleanup(func() { gateSide.Close() })
	return gateSide, received
}

func encode(t *testing.T, msg interface{}) []byte {
	t.Helper()
	data, err := codec.ProtoCodec.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestOnFrameForwardsRawBytesAndKeepsHooks 验证普通下行消息原样转发，钩子消息仍由钩子处理且不转发。
func TestOnFrameForwardsRawBytesAndKeepsHooks(t *testing.T) {
	ws, received := dialTestSocket(t)
	c := New(1, ws, codec.ProtoCodec)
	var hooked interface{}
	c.RegHookHandle((*pb_auth.LoginGameAck)(nil), func(_ tree.Context, msg interface{}) { hooked = msg })

	plain := encode(t, &pb_equipment.GetRoleEquipmentAck{})
	c.onFrame(nil, &pb_gateway.Frame{Payload: plain})
	select {
	case got := <-received:
		if !bytes.Equal(got, plain) {
			t.Fatalf("转发内容被改变: %x != %x", got, plain)
		}
	case <-time.After(time.Second):
		t.Fatal("普通消息未转发")
	}

	c.onFrame(nil, &pb_gateway.Frame{Payload: encode(t, &pb_auth.LoginGameAck{Uid: 7})})
	if ack, ok := hooked.(*pb_auth.LoginGameAck); !ok || ack.GetUid() != 7 {
		t.Fatalf("钩子未收到解码后的消息: %#v", hooked)
	}
	select {
	case got := <-received:
		t.Fatalf("钩子消息不应直接转发: %x", got)
	case <-time.After(50 * time.Millisecond):
	}

	c.onFrame(nil, &pb_gateway.Frame{Payload: []byte{1}})
	select {
	case got := <-received:
		t.Fatalf("过短的消息不应转发: %x", got)
	case <-time.After(50 * time.Millisecond):
	}
}
