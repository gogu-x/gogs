package actor

import (
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/db/mongorpc"
	"github.com/gogu-x/tree/tlog"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// actvIndex 描述一个活动集合需要的查询索引。
type actvIndex struct {
	model   mongorpc.Model
	indexes []mongo.IndexModel
}

// actvIndexes 返回活动相关集合的索引定义。索引名固定，修改定义时需同步更名并手动删除旧索引。
func (actor *Actv) actvIndexes() []actvIndex {
	ownerKeys := bson.D{{Key: "owner_scope", Value: 1}, {Key: "owner_id", Value: 1}}
	return []actvIndex{
		{
			// actv：启动按 owner_scope 加载服务器/联盟活动；玩家上线按 owner_scope + owner_id 加载。
			model: actor.model,
			indexes: []mongo.IndexModel{
				{Keys: ownerKeys, Options: options.Index().SetName("idx_owner")},
				{
					Keys:    bson.D{{Key: "owner_scope", Value: 1}, {Key: "owner_id", Value: 1}, {Key: "end_ts", Value: 1}},
					Options: options.Index().SetName("idx_owner_end_ts"),
				},
			},
		},
		{
			model: actor.dataModel,
			indexes: []mongo.IndexModel{
				// actv_data：玩家上线按归属者及有效活动编号加载组件数据。
				{Keys: ownerKeys, Options: options.Index().SetName("idx_owner")},
				{
					Keys:    bson.D{{Key: "owner_scope", Value: 1}, {Key: "owner_id", Value: 1}, {Key: "actv_id", Value: 1}},
					Options: options.Index().SetName("idx_owner_actv"),
				},
				// actv_data：启动按活动实例加载服务器/联盟单条数据，也用于按活动清理数据。
				{
					Keys:    bson.D{{Key: "actv_id", Value: 1}, {Key: "owner_scope", Value: 1}},
					Options: options.Index().SetName("idx_actv_owner_scope"),
				},
			},
		},
	}
}

// ensureIndexes 在加载数据前创建索引。Mongo Actor 串行处理请求，索引请求先于加载请求执行；
// 创建失败只记录日志，不阻塞活动加载（缺索引只影响查询性能）。
func (actor *Actv) ensureIndexes(ctx tree.Context) {
	for _, item := range actor.actvIndexes() {
		collection := item.model.GetCollection()
		if !actor.store.CreateIndexes(ctx, item.model.GetPrototype(), item.indexes, func(_ tree.Context, err error) {
			if err != nil {
				tlog.Log.Error("[actv/Actv.ensureIndexes] 活动索引创建失败, collection=%v err=%v", collection, err)
			}
		}) {
			tlog.Log.Error("[actv/Actv.ensureIndexes] 活动索引创建请求未发送, collection=%v", collection)
		}
	}
}
