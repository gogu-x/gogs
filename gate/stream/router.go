package stream

import (
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/pb/protoGateway"
)

func initRouter(s *Actor) {
	s.router.Register(&protoGateway.StreamMsg{}, s.onStreamMsg)
	s.router.Register(&stopMsg{}, s.onStop)
}

func (s *Actor) onStreamMsg(ctx actor.ActorContext, msg interface{}) {
	if s.stream == nil {
		return
	}
	if err := s.stream.Send(msg.(*protoGateway.StreamMsg).Frame); err != nil {
		log.Printf("StreamActor[%s]: send error: %v", s.serverID, err)
		ctx.Stop()
	}
}

func (s *Actor) onStop(ctx actor.ActorContext, _ interface{}) {
	ctx.Stop()
}
