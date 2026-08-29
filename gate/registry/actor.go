package registry

import (
	"context"
	"log"
	"sync/atomic"

	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/tree"
	cluster2 "github.com/gogu-x/tree/cluster"
)

type Actor struct {
	active  map[uint64]string
	pending map[string]bool
	cursor  atomic.Uint64
	cancel  context.CancelFunc
	router  tree.Router
}

func NewActor() *Actor {
	return &Actor{
		active:  make(map[uint64]string),
		pending: make(map[string]bool),
	}
}

func (r *Actor) Name() string { return constant.ActorRegistry }

func (r *Actor) OnInit(ctx tree.Context) {
	// 启动时从 etcd 加载所有节点，初始化 hash 路由缓存
	if all, err := cluster2.GetAll(); err != nil {
		log.Fatal("cluster.GetAll: " + err.Error())
	} else {
		for serverID := range all {
			instances, _ := cluster2.GetInstances(serverID)
			cluster2.UpdateNodes(serverID, instances)
		}
	}

	// 监听 etcd 节点变化，动态维护 hash 路由缓存。
	// cancel 保存到 Actor 字段，交由 OnStop 释放；不能用 defer，
	// 否则 OnInit 函数返回时就会立即取消 watch，导致后续事件全部收不到。
	watchCtx, watchCancel := context.WithCancel(context.Background())
	r.cancel = watchCancel
	go func() {
		for ev := range cluster2.WatchInstances(watchCtx) {
			instances, _ := cluster2.GetInstances(ev.ServerID)
			cluster2.UpdateNodes(ev.ServerID, instances)
			// 节点下线：通知 GateServer 广播 failover，让受影响连接无感切换
			if ev.Type == "delete" {
				//这里关闭，
			}
			if ev.Type == "put" {
			}
		}
	}()
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
