package app

import (
	"log"
	"reflect"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/game/constant"
	"github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"google.golang.org/protobuf/proto"
)

// Reply 将回包通过 NATS 发回 Gate
func (a *App) Reply(msg proto.Message) {
	body, err := codec.ProtoCodec.Marshal(msg)
	if err != nil {
		log.Printf("Reply marshal error: %v", err)
		return
	}
	frame := &protoGateway.Frame{
		Uid:     a.Player.UID,
		ConnId:  a.ConnID,
		GateId:  a.GateId,
		Payload: body,
		MsgType: reflect.TypeOf(msg).Elem().Name(),
	}
	data, err := proto.Marshal(frame)
	if err != nil {
		log.Printf("Reply frame marshal error: %v", err)
		return
	}
	actor.Send(actor.MustLookup(constant.ActorSupervisor), &natsrpc.ReplyMsg{Data: data})
}
