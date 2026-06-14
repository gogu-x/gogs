package server

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/cluster"
	natsclient "github.com/gogu-x/gogs/nats"
)

var Registry *RegistryActor

// RegistryActor 监听 etcd，始终使用最新上线的实例，新实例上线时通知旧实例退出
type RegistryActor struct {
	mu      sync.RWMutex
	active  map[string]string // serverID → instID
	pending map[string]bool   // serverID → active 实例正在等待确认是否真的下线
	cursor  atomic.Uint64
	cancel  context.CancelFunc
}

func (r *RegistryActor) OnInit(ctx actor.ActorContext) {
	Registry = r
	r.active = make(map[string]string)
	r.pending = make(map[string]bool)

	all, err := cluster.GetAll()
	if err != nil {
		log.Printf("RegistryActor: GetAll error: %v", err)
	} else {
		for serverID := range all {
			instances, _ := cluster.GetInstances(serverID)
			if len(instances) == 0 {
				continue
			}
			// 最新实例设为 active，其余全部通知退出
			r.active[serverID] = instances[0].InstID
			log.Printf("RegistryActor: server=%s active inst=%s", serverID, instances[0].InstID)
			for _, inst := range instances[1:] {
				natsclient.PublishShutdown(serverID, inst.InstID)
				log.Printf("RegistryActor: server=%s shutdown old inst=%s", serverID, inst.InstID)
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

func (r *RegistryActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch m := msg.(type) {
	case *cluster.InstanceEvent:
		r.handleEvent(ctx, m)
	case *checkDeleteMsg:
		if !r.pending[m.ServerID] || r.active[m.ServerID] != m.InstID {
			return
		}
		delete(r.active, m.ServerID)
		delete(r.pending, m.ServerID)
		log.Printf("RegistryActor: server=%s inst=%s confirmed down, no active", m.ServerID, m.InstID)
	}
}

func (r *RegistryActor) handleEvent(_ actor.ActorContext, ev *cluster.InstanceEvent) {
	switch ev.Type {
	case "put":
		activeInstID := r.active[ev.ServerID]
		if activeInstID == ev.InstID {
			r.pending[ev.ServerID] = false
			log.Printf("RegistryActor: server=%s inst=%s keepalive recovery", ev.ServerID, ev.InstID)
			return
		}
		// 新实例上线：通知旧 active 退出，切换到新实例
		if activeInstID != "" {
			natsclient.PublishShutdown(ev.ServerID, activeInstID)
			log.Printf("RegistryActor: server=%s shutdown old inst=%s", ev.ServerID, activeInstID)
		}
		r.active[ev.ServerID] = ev.InstID
		r.pending[ev.ServerID] = false
		natsPID, ok := actor.Lookup(ActorNats)
		if ok {
			actor.Send(natsPID, &SwitchMsg{ServerID: ev.ServerID, NewAddr: ev.Addr})
		}
		log.Printf("RegistryActor: server=%s switch to inst=%s addr=%s", ev.ServerID, ev.InstID, ev.Addr)

	case "delete":
		if r.active[ev.ServerID] != ev.InstID {
			return
		}
		r.pending[ev.ServerID] = true
		serverID, instID := ev.ServerID, ev.InstID
		self := actor.MustLookup(ActorRegistry)
		go func() {
			time.Sleep(3 * time.Second)
			actor.Send(self, &checkDeleteMsg{ServerID: serverID, InstID: instID})
		}()
	}
}

func (r *RegistryActor) OnStop(_ actor.ActorContext) {
	if r.cancel != nil {
		r.cancel()
	}
}

// HasServer 检查指定 serverID 是否有活跃连接
func (r *RegistryActor) HasServer(serverID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.active[serverID]
	return ok
}

// Pick 轮询返回一个有活跃连接的 serverID
func (r *RegistryActor) Pick() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var servers []string
	for serverID := range r.active {
		servers = append(servers, serverID)
	}
	if len(servers) == 0 {
		return ""
	}
	return servers[r.cursor.Add(1)%uint64(len(servers))]
}
