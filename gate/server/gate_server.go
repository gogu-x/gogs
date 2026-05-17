package server

import (
	"fmt"
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:  func(r *http.Request) bool { return true },
	Subprotocols: []string{"protobuf", "json"},
}

type GateServer struct {
	sys  *actor.ActorSystem
	addr string
}

func NewGateServer(sys *actor.ActorSystem, addr string) *GateServer {
	return &GateServer{sys: sys, addr: addr}
}

func (s *GateServer) Start() error {
	http.HandleFunc("/ws", s.wsHandler)
	return http.ListenAndServe(s.addr, nil)
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
	pid := s.sys.Spawn(name, connActor)

	go func() {
		defer s.sys.Send(pid, &stopMsg{})
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			s.sys.Send(pid, &WsMsg{Data: data})
		}
	}()
}
