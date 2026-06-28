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
	f, db, req := ctx.Future(), a.db, msg.(*protoPlatform.CreateOrderReq)
	go func() {
		resp, err := service.CreateOrder(db, req)
		f.Respond(resp, err)
	}()
}

func (a *Actor) onQueryOrder(ctx actor.ActorContext, msg interface{}) {
	f, db, req := ctx.Future(), a.db, msg.(*protoPlatform.QueryOrderReq)
	go func() {
		resp, err := service.QueryOrder(db, req)
		f.Respond(resp, err)
	}()
}

func (a *Actor) onDeliver(ctx actor.ActorContext, msg interface{}) {
	f, db, req := ctx.Future(), a.db, msg.(*deliverReq)
	go func() {
		err := service.DeliverOrder(db, req.orderID)
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
