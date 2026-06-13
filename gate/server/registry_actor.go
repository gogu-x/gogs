package server

import (
	"context"
	"log"
	"sync/atomic"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/cluster"
)

// Registry 全局单例，ConnActor 登录时调用 Pick 分配 serverID
var Registry *RegistryActor

// RegistryActor 监听 etcd，维护在线 Game 节点列表，提供轮询 Pick
type RegistryActor struct {
	servers []string // 在线 serverID 列表
	cursor  atomic.Uint64
	cancel  context.CancelFunc
}

// Pick 轮询返回一个在线 serverID，无节点时返回 ""
func (r *RegistryActor) Pick() string {
	if len(r.servers) == 0 {
		return ""
	}
	idx := r.cursor.Add(1) - 1
	return r.servers[idx%uint64(len(r.servers))]
}

// HasServer 检查指定 serverID 是否在线
func (r *RegistryActor) HasServer(serverID string) bool {
	for _, id := range r.servers {
		if id == serverID {
			return true
		}
	}
	return false
}

func (r *RegistryActor) OnInit(ctx actor.ActorContext) {
	Registry = r // 设置全局单例
	nodes, err := cluster.GetAll()
	if err != nil {
		log.Printf("RegistryActor: get all error: %v", err)
	} else {
		for serverID := range nodes {
			r.servers = append(r.servers, serverID)
		}
		log.Printf("RegistryActor: loaded %d game nodes", len(r.servers))
	}

	watchCtx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	self := ctx.Self()

	go func() {
		for ev := range cluster.Watch(watchCtx) {
			actor.Send(self, &ev)
		}
	}()
}

func (r *RegistryActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	ev, ok := msg.(*cluster.Event)
	if !ok {
		return
	}
	switch ev.Type {
	case "put":
		for _, id := range r.servers {
			if id == ev.ServerID {
				return // 已存在
			}
		}
		r.servers = append(r.servers, ev.ServerID)
		log.Printf("RegistryActor: game [%s] online, total=%d", ev.ServerID, len(r.servers))
	case "delete":
		for i, id := range r.servers {
			if id == ev.ServerID {
				r.servers = append(r.servers[:i], r.servers[i+1:]...)
				break
			}
		}
		log.Printf("RegistryActor: game [%s] offline, total=%d", ev.ServerID, len(r.servers))
	}
}

func (r *RegistryActor) OnStop(ctx actor.ActorContext) {
	if r.cancel != nil {
		r.cancel()
	}
}
