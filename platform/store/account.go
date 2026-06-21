package store

import (
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const ColAccounts = "accounts"
const ColOrders = "orders"

type Account struct {
	ID           bson.ObjectID `bson:"_id,omitempty"      json:"_id,omitempty"`
	Account      string        `bson:"account"            json:"account"`
	PasswordHash string        `bson:"password_hash"      json:"password_hash"`
	UID          uint64        `bson:"uid"                json:"uid"`
	CreatedAt    time.Time     `bson:"created_at"         json:"created_at"`
	ServerId     int32         `bson:"server_id"          json:"server_id"`
}

var uidCounter uint64

func init() { atomic.StoreUint64(&uidCounter, uint64(time.Now().Unix())) }

func NextUID() uint64 { return atomic.AddUint64(&uidCounter, 1) }
