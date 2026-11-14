package grpc

import (
	"time"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/pb/pfpb/pb_pf"
	"github.com/gogu-x/gogs/platform/service"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
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
	orderID := msg.(*deliverReq).orderID
	tlog.Log.Error("Payment delivery unavailable: signature verification is not configured, order=%s", orderID)
	ctx.Response(pb_pf.ErrCode_INTERNAL, nil)
}

// DeliverByOrderID forwards payment confirmation to the platform actor.
func DeliverByOrderID(orderID string) pb_pf.ErrCode {
	if orderID == "" {
		return pb_pf.ErrCode_PARAM
	}
	pid, ok := tree.Lookup(def.PF)
	if !ok {
		return pb_pf.ErrCode_INTERNAL
	}
	value, err := tree.Request(pid, &deliverReq{orderID}).AwaitTimeout(5 * time.Second)
	code, valid := value.(pb_pf.ErrCode)
	if err != nil || !valid {
		tlog.Log.Error("DeliverByOrderID failed: order=%s err=%v response=%T", orderID, err, value)
		return pb_pf.ErrCode_INTERNAL
	}
	return code
}
