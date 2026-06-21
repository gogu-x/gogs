package service

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	actor "github.com/gogu-x/bigTree"
	rpcmongo "github.com/gogu-x/gogs/rpc/mongo"
	"github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoPlatform"
	"github.com/gogu-x/gogs/platform/store"
)

func CreateOrder(mongoPID actor.PID, req *protoPlatform.CreateOrderReq) (*protoPlatform.OrderAck, error) {
	orderID := fmt.Sprintf("%d-%d", req.Uid, time.Now().UnixNano())
	o := &store.Order{
		ID: bson.NewObjectID(), OrderID: orderID, UID: req.Uid,
		ProductID: req.ProductId, ServerID: req.ServerId,
		Status: protoPlatform.OrderStatus_PENDING, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := dbCall(mongoPID, &rpcmongo.InsertOne{Collection: store.ColOrders, Doc: o}); err != nil {
		return &protoPlatform.OrderAck{Code: protoCommon.ErrCode_ERR_INTERNAL, Msg: err.Error()}, nil
	}
	return &protoPlatform.OrderAck{Code: protoCommon.ErrCode_OK, OrderId: orderID}, nil
}

func QueryOrder(mongoPID actor.PID, req *protoPlatform.QueryOrderReq) (*protoPlatform.OrderDetail, error) {
	o := &store.Order{}
	if err := callFindOne(mongoPID, store.ColOrders, bson.M{"order_id": req.OrderId}, o); err != nil {
		return &protoPlatform.OrderDetail{}, nil
	}
	return &protoPlatform.OrderDetail{
		OrderId: o.OrderID, Uid: o.UID, ProductId: o.ProductID,
		ServerId: o.ServerID, Status: o.Status,
	}, nil
}

func DeliverOrder(mongoPID actor.PID, orderID string) error {
	o := &store.Order{}
	if err := callFindOne(mongoPID, store.ColOrders, bson.M{"order_id": orderID}, o); err != nil {
		return fmt.Errorf("order not found: %s", orderID)
	}
	_, err := dbCall(mongoPID, &rpcmongo.UpdateOne{
		Collection: store.ColOrders,
		Filter:     bson.M{"order_id": orderID},
		Update:     bson.M{"$set": bson.M{"status": protoPlatform.OrderStatus_PAID, "updated_at": time.Now()}},
	})
	if err != nil {
		return err
	}
	return natsrpc.PublishDeliver(o.ServerID, &protoPlatform.DeliverNotify{
		Uid: o.UID, OrderId: o.OrderID, ProductId: o.ProductID, ServerId: o.ServerID,
	})
}
