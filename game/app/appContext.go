package app

import (
	"log"
	"reflect"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/constant"
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
	actor.Send(actor.MustLookup(constant.ActorNats), &natsrpc.SendMsg{
		Module: natsrpc.ModuleGate,
		NodeID: a.GateId,
		Frame: &protoGateway.Frame{
			Uid:     a.Player.UID,
			ConnId:  a.ConnID,
			GateId:  a.GateId,
			Payload: body,
			MsgType: reflect.TypeOf(msg).Elem().Name(),
		},
	})
}
