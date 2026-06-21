package internal

import (
	"fmt"

	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/pb/protoChat"
)

func ChatService(s *Session, msg interface{}) {
	req := msg.(*protoChat.ChatReq)
	fmt.Printf("game server [%d] player says: %s\n", config.ServerID, req.Content)
	s.Reply(&protoChat.ChatAck{State: 2})
}
