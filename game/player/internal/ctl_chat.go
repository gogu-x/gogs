package internal

import (
	"fmt"

	"github.com/gogu-x/gogs/game/player/internal/base"
	"github.com/gogu-x/gogs/pb/protoChat"
)

func ChatService(s *base.Session, msg interface{}) {
	req := msg.(*protoChat.ChatReq)
	fmt.Printf("game ChatService: %s", req.Content)
	s.Reply(&protoChat.ChatAck{State: 2})
}
