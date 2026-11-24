package store

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const ColAccounts = "accounts"
const ColOrders = "orders"

type Account struct {
	ID           bson.ObjectID `bson:"_id,omitempty"      json:"_id,omitempty"`
	Account      string        `bson:"account"            json:"account"`
	PasswordHash string        `bson:"password_hash"      json:"password_hash"`
	UID          uint64        `bson:"uid"                json:"uid"`
	CreatedAt    time.Time     `bson:"created_at"         json:"created_at"`
	ServerId     uint32        `bson:"server_id"          json:"server_id"`
}

// ColCounters 保存全局自增序列，文档 _id 为序列名。
const ColCounters = "counters"

// uidCounterID 是玩家 UID 序列在 ColCounters 中的文档编号。
const uidCounterID = "uid"

// uidSequence 是 UID 计数器文档。
type uidSequence struct {
	Seq uint64 `bson:"seq"`
}

// SeedUIDCounter 在启动时把 UID 计数器推进到不小于账号表中已有的最大 uid，
// 使历史账号（包括旧实现按启动时间生成的 uid）不会被重新分配。可重复执行。
func SeedUIDCounter(ctx context.Context, db *mongo.Database) (uint64, error) {
	var newest Account
	findOpts := options.FindOne().SetSort(bson.D{{Key: "uid", Value: -1}}).SetProjection(bson.M{"uid": 1})
	err := db.Collection(ColAccounts).FindOne(ctx, bson.M{}, findOpts).Decode(&newest)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return 0, err
	}
	var counter uidSequence
	updateOpts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	err = db.Collection(ColCounters).FindOneAndUpdate(
		ctx,
		bson.M{"_id": uidCounterID},
		bson.M{"$max": bson.M{"seq": newest.UID}},
		updateOpts,
	).Decode(&counter)
	return counter.Seq, err
}

// NextUID 原子地分配下一个全局唯一 UID；多个 platform 实例共享同一计数器，重启不回退。
func NextUID(ctx context.Context, db *mongo.Database) (uint64, error) {
	var counter uidSequence
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	err := db.Collection(ColCounters).FindOneAndUpdate(
		ctx,
		bson.M{"_id": uidCounterID},
		bson.M{"$inc": bson.M{"seq": uint64(1)}},
		opts,
	).Decode(&counter)
	if err != nil {
		return 0, err
	}
	return counter.Seq, nil
}
