package grpc

import (
	"time"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/pb/protoPlatform"
	"github.com/gogu-x/gogs/platform/service"
)

type deliverReq struct{ orderID string }

func (a *Actor) onCreateOrder(ctx actor.ActorContext, msg interface{}) {
	resp, err := service.CreateOrder(a.mongoPID, msg.(*protoPlatform.CreateOrderReq))
	ctx.Response(resp, err)
}

func (a *Actor) onQueryOrder(ctx actor.ActorContext, msg interface{}) {
	resp, err := service.QueryOrder(a.mongoPID, msg.(*protoPlatform.QueryOrderReq))
	ctx.Response(resp, err)
}

func (a *Actor) onDeliver(ctx actor.ActorContext, msg interface{}) {
	err := service.DeliverOrder(a.mongoPID, msg.(*deliverReq).orderID)
	ctx.Response(nil, err)
}

// DeliverByOrderID 由 webhook 调用，向 GrpcActor mailbox 发送 deliverReq
func DeliverByOrderID(orderID string) error {
	pid, ok := actor.Default().Lookup(constant.ActorPlatformGrpc)
	if !ok {
		return actor.ErrActorNotFound
	}
	_, err := actor.Default().Request(pid, &deliverReq{orderID}).AwaitTimeout(5 * time.Second)
	return err
}
