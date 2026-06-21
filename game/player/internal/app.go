package internal

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/player/module/asset"
	"github.com/gogu-x/gogs/game/player/module/bag"
	"github.com/gogu-x/gogs/game/player/module/cardgroup"
	"github.com/gogu-x/gogs/game/player/module/shop"
	mongoRpc "github.com/gogu-x/gogs/rpc/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const collPlayer = "player"

// PlayerData 玩家全量数据，随 PlayerActor 生命周期存活，同时作为 MongoDB 文档
type PlayerData struct {
	UID   uint64 `bson:"_id"`
	Name  string `bson:"name"`
	Level uint32 `bson:"level"`
	State int    `bson:"state"`

	AssetMgr     *asset.Mgr     `bson:"asset"`
	BagMgr       *bag.Mgr       `bson:"bag"`
	CardGroupMgr *cardgroup.Mgr `bson:"cardgroup"`
	ShopMgr      *shop.Mgr      `bson:"shop"`
}

func NewPlayerData(uid uint64) *PlayerData {
	return &PlayerData{
		UID:          uid,
		AssetMgr:     &asset.Mgr{},
		BagMgr:       &bag.Mgr{},
		CardGroupMgr: cardgroup.New(),
		ShopMgr:      &shop.Mgr{},
	}
}

// Load 异步从 MongoDB 加载玩家数据，结果通过 cb 回调（在 Actor goroutine 内执行）。
// cb 参数：data 非 nil 表示加载成功；data 为 nil 且 err 为 nil 表示新玩家（不存在）；err 非 nil 表示 DB 故障。
func Load(ctx actor.ActorContext, uid uint64, cb func(data *PlayerData, err error)) {
	data := NewPlayerData(uid)
	ctx.Request(
		actor.MustLookup(constant.ActorGameMongo),
		&mongoRpc.FindOne{
			Collection: collPlayer,
			Filter:     bson.M{"_id": uid},
			Result:     data,
		},
	).Callback(ctx, func(ret interface{}, err error) {
		if err != nil {
			// ErrNoDocuments 也通过 err 传入，由 cb 自行判断
			cb(nil, err)
			return
		}
		cb(data, nil)
	})
}

// Save fire-and-forget，upsert 玩家全量数据，不等待结果。
func (p *PlayerData) Save() {
	actor.Send(
		actor.MustLookup(constant.ActorGameMongo),
		&mongoRpc.UpdateOne{
			Collection: collPlayer,
			Filter:     bson.M{"_id": p.UID},
			Update:     bson.M{"$set": p},
			Upsert:     true,
		},
	)
}
