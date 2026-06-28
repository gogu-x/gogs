package registry

import (
	"context"
	"log"
	"sync"
	"sync/atomic"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/natsrpc"
)

var Global *Actor

type Actor struct {
	mu      sync.RWMutex
	active  map[string]string
	pending map[string]bool
	cursor  atomic.Uint64
	cancel  context.CancelFunc
	router  actor.Router
}

func (r *Actor) OnInit(ctx actor.ActorContext) {
	Global = r
	r.active = make(map[string]string)
	r.pending = make(map[string]bool)
	initRouter(r)

	all, err := cluster.GetAll()
	if err != nil {
		log.Printf("RegistryActor: GetAll error: %v", err)
	} else {
		for serverID := range all {
			instances, _ := cluster.GetInstances(serverID)
			if len(instances) == 0 {
				continue
			}
			r.active[serverID] = instances[0].NodeID
			for _, inst := range instances[1:] {
				natsrpc.PublishShutdown(serverID, inst.NodeID)
			}
		}
		log.Printf("RegistryActor: loaded %d servers", len(r.active))
	}

	watchCtx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	self := ctx.Self()
	go func() {
		for ev := range cluster.WatchInstances(watchCtx) {
			actor.Send(self, &ev)
		}
	}()
}

func (r *Actor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	r.router.Route(ctx, msg)
}

func (r *Actor) OnStop(_ actor.ActorContext) {
	if r.cancel != nil {
		r.cancel()
	}
}

func (r *Actor) HasServer(serverID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.active[serverID]
	return ok
}

func (r *Actor) Pick() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	servers := make([]string, 0, len(r.active))
	for id := range r.active {
		servers = append(servers, id)
	}
	if len(servers) == 0 {
		return ""
	}
	return servers[r.cursor.Add(1)%uint64(len(servers))]
}
