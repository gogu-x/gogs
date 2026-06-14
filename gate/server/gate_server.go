package server

import (
	"fmt"
	"net/http"
	"time"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	Subprotocols:    []string{"protobuf", "json"},
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

type GateServer struct {
	addr string
}

func NewGateServer(addr string) *GateServer {
	return &GateServer{addr: addr}
}

func (s *GateServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.wsHandler)
	srv := &http.Server{
		Addr:    s.addr,
		Handler: mux,
		// 限制慢握手占用，快速释放 backlog
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *GateServer) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	var c codec.Codec = codec.ProtoCodec
	if conn.Subprotocol() == "json" {
		c = codec.JsonCodec
	}

	connActor := NewConnActor(conn, c)
	name := fmt.Sprintf("conn-%p", conn)
	pid := actor.Spawn(name, connActor)

	go func() {
		defer actor.Send(pid, &stopMsg{})
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			actor.Send(pid, &WsMsg{Data: data})
		}
	}()
}
