package store

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/gogu-x/gogs/pb/protoPlatform"
)

type Order struct {
	ID        bson.ObjectID             `bson:"_id,omitempty"  json:"_id,omitempty"`
	OrderID   string                    `bson:"order_id"       json:"order_id"`
	UID       uint64                    `bson:"uid"            json:"uid"`
	ProductID string                    `bson:"product_id"     json:"product_id"`
	ServerID  string                    `bson:"server_id"      json:"server_id"`
	Status    protoPlatform.OrderStatus `bson:"status"         json:"status"`
	CreatedAt time.Time                 `bson:"created_at"     json:"created_at"`
	UpdatedAt time.Time                 `bson:"updated_at"     json:"updated_at"`
}
