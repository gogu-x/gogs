package ws

import (
	"log"
	"net/http"
	"time"

	"github.com/gogu-x/gogs/gate/conn"
	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gogu-x/tree/comm"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	Subprotocols:    []string{"protobuf", "json"},
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

type Server struct {
	addr    string
	clients map[uint64]struct{}
	router  tree.Router
	idGen   *comm.IDGenerator
}

// New 创建 GateServer，gateID 用作 connID 生成器的节点位，
// 确保同一 gate 进程内的所有连接 ID 全局唯一（跨 gate 进程也不冲突）。
func New(addr string, gateID int64) *Server {
	idGen, err := comm.NewIDGenerator(gateID)
	if err != nil {
		panic(err)
	}
	return &Server{addr: addr, clients: make(map[uint64]struct{}), idGen: idGen}
}

func (s *Server) Name() string { return constant.ActorGateServer }

func (s *Server) OnInit(_ tree.Context) {
	initRouter(s)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.wsHandler)
	srv := &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Printf("GateServer: %v", err)
		}
	}()
}

func (s *Server) HandleMessage(ctx tree.Context, msg interface{}) {
	s.router.Route(ctx, msg)
}

func (s *Server) OnStop(_ tree.Context) {}

func (s *Server) wsHandler(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	var cd codec.Codec = codec.ProtoCodec
	if c.Subprotocol() == "json" {
		cd = codec.JsonCodec
	}
	tree.SpawnOne(conn.New(s.idGen.NextUint64(), c, cd))
}
