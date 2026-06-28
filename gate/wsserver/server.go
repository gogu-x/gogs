package wsserver

import (
	"fmt"
	"log"
	"net/http"
	"time"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/gate/conn"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	Subprotocols:    []string{"protobuf", "json"},
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

type Server struct {
	addr   string
	conns  map[uint64]struct{}
	router actor.Router
}

func New(addr string) *Server {
	return &Server{addr: addr, conns: make(map[uint64]struct{})}
}

func (s *Server) OnInit(_ actor.ActorContext) {
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

func (s *Server) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	s.router.Route(ctx, msg)
}

func (s *Server) OnStop(_ actor.ActorContext) {}

func (s *Server) wsHandler(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	var cd codec.Codec = codec.ProtoCodec
	if c.Subprotocol() == "json" {
		cd = codec.JsonCodec
	}

	// gate 链接全部在线的game rpc stream 启动的时候按照etcd 中注册的服务进行链接
	// 根据用户把携带的serverId 把消息分配到对应的RPC Stream 里面
	// game和gate 建立的是一个rpc stream 双向流
	// 如果多个gate 和同一个game 建立双向流。他们之间数据传输是什么样的？ 多个gate和同一组game server 1 建议链接数据又是什么样的？ 这种方案是否合理？
	// 对于这一套gate 和game 整理的架构 设计师傅合理
	actor.Spawn(fmt.Sprintf("conn-%p", c), conn.New(c, cd))
}
