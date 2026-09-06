package service

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/gogu-x/gogs/pb/pfpb/pb_pf"
	"github.com/gogu-x/gogs/platform/store"
)

func CreateOrder(db *mongo.Database, req *pb_pf.CreateOrderReq) (*pb_pf.OrderAck, error) {
	orderID := fmt.Sprintf("%d-%d", req.Uid, time.Now().UnixNano())
	o := &store.Order{
		ID: bson.NewObjectID(), OrderID: orderID, UID: req.Uid,
		ProductID: req.ProductId, ServerID: req.ServerId,
		Status: pb_pf.OrderStatus_PENDING, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if _, err := db.Collection(store.ColOrders).InsertOne(ctx, o); err != nil {
		return &pb_pf.OrderAck{Code: pb_pf.ErrCode_INTERNAL, Msg: err.Error()}, nil
	}
	return &pb_pf.OrderAck{Code: pb_pf.ErrCode_OK, OrderId: orderID}, nil
}

func QueryOrder(db *mongo.Database, req *pb_pf.QueryOrderReq) (*pb_pf.OrderDetail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	o := &store.Order{}
	if err := db.Collection(store.ColOrders).FindOne(ctx, bson.M{"order_id": req.OrderId}).Decode(o); err != nil {
		return &pb_pf.OrderDetail{}, nil
	}
	return &pb_pf.OrderDetail{
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
		bson.M{"$set": bson.M{"status": pb_pf.OrderStatus_PAID, "updated_at": time.Now()}},
	)
	if err != nil {
		return err
	}
	return nil
}
