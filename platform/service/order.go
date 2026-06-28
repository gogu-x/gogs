package service

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoPlatform"
	"github.com/gogu-x/gogs/platform/store"
)

func CreateOrder(db *mongo.Database, req *protoPlatform.CreateOrderReq) (*protoPlatform.OrderAck, error) {
	orderID := fmt.Sprintf("%d-%d", req.Uid, time.Now().UnixNano())
	o := &store.Order{
		ID: bson.NewObjectID(), OrderID: orderID, UID: req.Uid,
		ProductID: req.ProductId, ServerID: req.ServerId,
		Status: protoPlatform.OrderStatus_PENDING, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if _, err := db.Collection(store.ColOrders).InsertOne(ctx, o); err != nil {
		return &protoPlatform.OrderAck{Code: protoCommon.ErrCode_ERR_INTERNAL, Msg: err.Error()}, nil
	}
	return &protoPlatform.OrderAck{Code: protoCommon.ErrCode_OK, OrderId: orderID}, nil
}

func QueryOrder(db *mongo.Database, req *protoPlatform.QueryOrderReq) (*protoPlatform.OrderDetail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	o := &store.Order{}
	if err := db.Collection(store.ColOrders).FindOne(ctx, bson.M{"order_id": req.OrderId}).Decode(o); err != nil {
		return &protoPlatform.OrderDetail{}, nil
	}
	return &protoPlatform.OrderDetail{
		OrderId: o.OrderID, Uid: o.UID, ProductId: o.ProductID,
		ServerId: o.ServerID, Status: o.Status,
	}, nil
}

func DeliverOrder(db *mongo.Database, orderID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	o := &store.Order{}
	if err := db.Collection(store.ColOrders).FindOne(ctx, bson.M{"order_id": orderID}).Decode(o); err != nil {
		return fmt.Errorf("order not found: %s", orderID)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel2()
	_, err := db.Collection(store.ColOrders).UpdateOne(ctx2,
		bson.M{"order_id": orderID},
		bson.M{"$set": bson.M{"status": protoPlatform.OrderStatus_PAID, "updated_at": time.Now()}},
	)
	if err != nil {
		return err
	}
	return natsrpc.PublishDeliver(o.ServerID, &protoPlatform.DeliverNotify{
		Uid: o.UID, OrderId: o.OrderID, ProductId: o.ProductID, ServerId: o.ServerID,
	})
}
