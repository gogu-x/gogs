package internal

import (
	"log"

	"github.com/gogu-x/gogs/game/play/internal/base"
	"github.com/gogu-x/gogs/pb/protoChat"
)

func ChatService(s *base.PlayContext, req *protoChat.ChatReq) {
	log.Printf("play: chat from uid=%d: %s", s.Player.UID, req.GetContent())
	s.Reply(&protoChat.ChatAck{State: 2})
}
