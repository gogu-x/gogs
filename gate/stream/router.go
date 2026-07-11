package stream

import (
	"log"

	"github.com/gogu-x/gogs/pb/protoGateway"
	actor "github.com/gogu-x/tree"
)

func initRouter(s *Actor) {
	s.router.Register(&protoGateway.StreamMsg{}, s.onStreamMsg)
	s.router.Register(&stopMsg{}, s.onStop)
}

func (s *Actor) onStreamMsg(ctx actor.Context, msg interface{}) {
	if s.stream == nil {
		return
	}
	if err := s.stream.Send(msg.(*protoGateway.StreamMsg).Frame); err != nil {
		log.Printf("StreamActor[%s]: send error: %v", s.serverID, err)
		ctx.Stop()
	}
}

func (s *Actor) onStop(ctx actor.Context, _ interface{}) {
	ctx.Stop()
}
