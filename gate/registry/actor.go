package registry

import (
	"context"

	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/tree"
	"google.golang.org/grpc"
)

type Actor struct {
	active          map[uint64]string
	cancel          context.CancelFunc
	router          tree.Router
	gameGrpcPoolMgr map[uint64]*grpc.ClientConn
}

func NewActor() *Actor {
	return &Actor{
		active: make(map[uint64]string),
	}
}

func (r *Actor) Name() string { return constant.ActorRegistry }

func (r *Actor) OnInit(ctx tree.Context) {
}

func (r *Actor) HandleMessage(ctx tree.Context, msg interface{}) {
	r.router.Route(ctx, msg)
}

func (r *Actor) OnStop(_ tree.Context) {
	if r.cancel != nil {
		r.cancel()
	}
}

func (r *Actor) HasServer(serverID uint64) bool {
	_, ok := r.active[serverID]
	return ok
}

func (r *Actor) Pick() string {
	servers := make([]uint64, 0, len(r.active))
	for id := range r.active {
		servers = append(servers, id)
	}
	if len(servers) == 0 {
		return ""
	}
	return ""
}
