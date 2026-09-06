package grpc

import (
	"time"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/pb/pfpb/pb_pf"
	"github.com/gogu-x/gogs/platform/service"
	"github.com/gogu-x/tree"
)

type deliverReq struct{ orderID string }

func (a *Platform) onCreateOrder(ctx tree.Context, msg interface{}) {
	f, db, req := ctx.RequestEnvelope(), a.db, msg.(*pb_pf.CreateOrderReq)
	go func() {
		resp, err := service.CreateOrder(db, req)
		f.Respond(resp, err)
	}()
}

func (a *Platform) onQueryOrder(ctx tree.Context, msg interface{}) {
	f, db, req := ctx.RequestEnvelope(), a.db, msg.(*pb_pf.QueryOrderReq)
	go func() {
		resp, err := service.QueryOrder(db, req)
		f.Respond(resp, err)
	}()
}

func (a *Platform) onDeliver(ctx tree.Context, msg interface{}) {
}

// DeliverByOrderID 由 webhook 调用，向 GrpcActor mailbox 发送 deliverReq
func DeliverByOrderID(orderID string) error {
	pid, ok := tree.Default().Lookup(def.PF)
	if !ok {
		return actor.ErrActorNotFound
	}
	_, err := tree.Default().Request(pid, &deliverReq{orderID}).AwaitTimeout(5 * time.Second)
	return err
}
