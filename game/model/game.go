package model

import (
	"log"

	actor "github.com/gogu-x/bigTree"
)

// PlayerSupervisor 管理所有 PlayerActor 的生命周期，并启动全局公共 Actor
type PlayerSupervisor struct{}

func (s *PlayerSupervisor) OnInit(ctx actor.ActorContext) {
	Consumer = &NatsConsumer{}
	if err := Consumer.Start(ctx.Self()); err != nil {
		log.Fatalf("PlayerSupervisor: nats consumer start error: %v", err)
	}
}

func (s *PlayerSupervisor) OnStop(ctx actor.ActorContext) {
	if Consumer != nil {
		Consumer.Stop()
	}
}

func (s *PlayerSupervisor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	m, ok := msg.(*inboundMsg)
	if !ok {
		return
	}
	name := playerActorName(m.uid)
	pid, exists := ctx.Lookup(name)
	if !exists {
		pid = actor.Spawn(name, newPlayerActor(m.uid, m.connID))
	}
	ctx.Send(pid, m)
}
