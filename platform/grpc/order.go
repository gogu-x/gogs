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
	f, mongoPID, req := ctx.Future(), a.mongoPID, msg.(*protoPlatform.CreateOrderReq)
	go func() {
		resp, err := service.CreateOrder(mongoPID, req)
		f.Respond(resp, err)
	}()
}

func (a *Actor) onQueryOrder(ctx actor.ActorContext, msg interface{}) {
	f, mongoPID, req := ctx.Future(), a.mongoPID, msg.(*protoPlatform.QueryOrderReq)
	go func() {
		resp, err := service.QueryOrder(mongoPID, req)
		f.Respond(resp, err)
	}()
}

func (a *Actor) onDeliver(ctx actor.ActorContext, msg interface{}) {
	f, mongoPID, req := ctx.Future(), a.mongoPID, msg.(*deliverReq)
	go func() {
		err := service.DeliverOrder(mongoPID, req.orderID)
		f.Respond(nil, err)
	}()
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
