package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/gogu-x/gogs/pb/pfpb/pb_pf"
	"github.com/gogu-x/gogs/platform/store"
	"github.com/gogu-x/tree/tlog"
)

func CreateOrder(db *mongo.Database, req *pb_pf.CreateOrderReq) (*pb_pf.OrderAck, error) {
	if req == nil || req.GetUid() == 0 || req.GetServerId() == "" || req.GetProductId() == "" {
		return &pb_pf.OrderAck{Code: pb_pf.ErrCode_PARAM}, nil
	}
	orderID := fmt.Sprintf("%d-%d", req.Uid, time.Now().UnixNano())
	o := &store.Order{
		ID: bson.NewObjectID(), OrderID: orderID, UID: req.Uid,
		ProductID: req.ProductId, ServerID: req.ServerId,
		Status: pb_pf.OrderStatus_PENDING, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if _, err := db.Collection(store.ColOrders).InsertOne(ctx, o); err != nil {
		tlog.Log.Error("CreateOrder failed: uid=%d order=%s err=%v", req.GetUid(), orderID, err)
		return &pb_pf.OrderAck{Code: pb_pf.ErrCode_INTERNAL}, nil
	}
	return &pb_pf.OrderAck{Code: pb_pf.ErrCode_OK, OrderId: orderID}, nil
}

func QueryOrder(db *mongo.Database, req *pb_pf.QueryOrderReq) (*pb_pf.OrderDetail, error) {
	if req == nil || req.GetOrderId() == "" {
		return &pb_pf.OrderDetail{Code: pb_pf.ErrCode_PARAM}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	o := &store.Order{}
	if err := db.Collection(store.ColOrders).FindOne(ctx, bson.M{"order_id": req.OrderId}).Decode(o); err != nil {
		tlog.Log.Error("QueryOrder failed: order=%s err=%v", req.GetOrderId(), err)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return &pb_pf.OrderDetail{Code: pb_pf.ErrCode_ORDER_NOT_FOUND}, nil
		}
		return &pb_pf.OrderDetail{Code: pb_pf.ErrCode_INTERNAL}, nil
	}
	return &pb_pf.OrderDetail{
		OrderId: o.OrderID, Uid: o.UID, ProductId: o.ProductID,
		ServerId: o.ServerID, Status: o.Status, Code: pb_pf.ErrCode_OK,
	}, nil
}

func DeliverOrder(db *mongo.Database, orderID string) pb_pf.ErrCode {
	if orderID == "" {
		return pb_pf.ErrCode_PARAM
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	o := &store.Order{}
	if err := db.Collection(store.ColOrders).FindOne(ctx, bson.M{"order_id": orderID}).Decode(o); err != nil {
		tlog.Log.Error("DeliverOrder lookup failed: order=%s err=%v", orderID, err)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return pb_pf.ErrCode_ORDER_NOT_FOUND
		}
		return pb_pf.ErrCode_INTERNAL
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel2()
	_, err := db.Collection(store.ColOrders).UpdateOne(ctx2,
		bson.M{"order_id": orderID},
		bson.M{"$set": bson.M{"status": pb_pf.OrderStatus_PAID, "updated_at": time.Now()}},
	)
	if err != nil {
		tlog.Log.Error("DeliverOrder update failed: order=%s err=%v", orderID, err)
		return pb_pf.ErrCode_INTERNAL
	}
	return pb_pf.ErrCode_OK
}
