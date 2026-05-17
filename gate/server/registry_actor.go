package server

import (
	"context"
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/cluster"
	"log"
)

type etcdEvent = cluster.Event

type RegistryActor struct {
	cancel context.CancelFunc
}

func NewRegistryActor() *RegistryActor {
	return &RegistryActor{}
}

func (r *RegistryActor) OnInit(ctx actor.ActorContext) {
	nodes, err := cluster.GetAll()
	if err != nil {
		log.Printf("RegistryActor: get all error: %v", err)
	} else {
		for serverID := range nodes {
			ctx.System().Spawn(StreamActorName(serverID), NewStreamActor(serverID))
			log.Printf("RegistryActor: loaded server %s", serverID)
		}
	}

	watchCtx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	self := ctx.Self()
	sys := ctx.System()

	go func() {
		for ev := range cluster.Watch(watchCtx) {
			sys.Send(self, &ev)
		}
	}()
}

func (r *RegistryActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	ev, ok := msg.(*cluster.Event)
	if !ok {
		return
	}

	name := StreamActorName(ev.ServerID)

	switch ev.Type {
	case "put":
		if _, ok := ctx.Lookup(name); !ok {
			ctx.System().Spawn(name, NewStreamActor(ev.ServerID))
			log.Printf("RegistryActor: server %s online -> %s", ev.ServerID, ev.Addr)
		}
	case "delete":
		if pid, ok := ctx.Lookup(name); ok {
			ctx.System().Send(pid, &stopMsg{})
		}
		log.Printf("RegistryActor: server %s offline", ev.ServerID)
	}
}

func (r *RegistryActor) OnStop(ctx actor.ActorContext) {
	if r.cancel != nil {
		r.cancel()
	}
}
