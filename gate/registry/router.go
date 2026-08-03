package registry

import (
	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/pb/protoGateway"
	actor "github.com/gogu-x/tree"
)

func initRouter(r *Actor) {
	r.router.Register(&cluster.InstanceEvent{}, r.onInstanceEvent)
	r.router.Register(&protoGateway.CheckDeleteMsg{}, r.onCheckDelete)
}

func (r *Actor) onInstanceEvent(ctx actor.Context, msg interface{}) {
	r.handleEvent(ctx, msg.(*cluster.InstanceEvent))
}

func (r *Actor) onCheckDelete(_ actor.Context, msg interface{}) {
	//m := msg.(*protoGateway.CheckDeleteMsg)
	//if !r.pending[m.ServerId] || r.active[m.ServerId] != m.NodeId {
	//	return
	//}
	//delete(r.active, m.ServerId)
	//delete(r.pending, m.ServerId)
	//log.Printf("RegistryActor: server=%s inst=%s confirmed down", m.ServerId, m.NodeId)
}

func (r *Actor) handleEvent(_ actor.Context, ev *cluster.InstanceEvent) {
	//switch ev.Type {
	//case "put":
	//	if r.active[ev.ServerID] == ev.NodeID {
	//		r.pending[ev.ServerID] = false
	//		return
	//	}
	//	if old := r.active[ev.ServerID]; old != "" {
	//		natsrpc.PublishShutdown(ev.ServerID, old)
	//	}
	//	r.active[ev.ServerID] = ev.NodeID
	//	r.pending[ev.ServerID] = false
	//	if pid, ok := actor.Lookup(constant.ActorNats); ok {
	//		actor.Send(pid, &protoGateway.SwitchMsg{ServerId: ev.ServerID, NewAddr: ev.Addr})
	//	}
	//	log.Printf("RegistryActor: server=%s switch to %s addr=%s", ev.ServerID, ev.NodeID, ev.Addr)
	//
	//case "delete":
	//	if r.active[ev.ServerID] != ev.NodeID {
	//		return
	//	}
	//	r.pending[ev.ServerID] = true
	//	serverID, NodeID := ev.ServerID, ev.NodeID
	//	self := actor.MustLookup(constant.ActorRegistry)
	//	go func() {
	//		time.Sleep(3 * time.Second)
	//		actor.Send(self, &protoGateway.CheckDeleteMsg{ServerId: serverID, NodeId: NodeID})
	//	}()
	//}
}
