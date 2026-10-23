package actor

import (
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// retainedActivityFilter 在加载查询中排除已过十四天结算期的活动，兼容未设置结束时间的旧文档。
func retainedActivityFilter(filter bson.M, now int64) bson.M {
	filter["$or"] = bson.A{
		bson.M{"end_ts": bson.M{"$gt": now - service.SettlementRetentionSeconds}},
		bson.M{"end_ts": bson.M{"$lte": 0}},
		bson.M{"end_ts": bson.M{"$exists": false}},
	}
	return filter
}
