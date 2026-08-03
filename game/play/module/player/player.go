package base

import (
	"time"

	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/play/module/asset"
	"github.com/gogu-x/gogs/game/play/module/bag"
	"github.com/gogu-x/gogs/game/play/module/cardgroup"
	"github.com/gogu-x/gogs/game/play/module/shop"
	mongoRpc "github.com/gogu-x/gogs/rpc/mongo"
	"github.com/gogu-x/tree"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const collPlayer = "player"

// loadTimeout 是同步加载玩家数据的最大等待时间，超时则放弃，避免 goroutine 永久阻塞。
const loadTimeout = 5 * time.Second

// Player  玩家全量数据，随 PlayerActor 生命周期存活，同时作为 MongoDB 文档
type Player struct {
	UID   uint64 `bson:"_id"`
	Name  string `bson:"name"`
	Level uint32 `bson:"level"`
	State int    `bson:"state"`

	AssetMgr     *asset.Mgr     `bson:"asset"`
	BagMgr       *bag.Mgr       `bson:"bag"`
	CardGroupMgr *cardgroup.Mgr `bson:"card_group"`
	ShopMgr      *shop.Mgr      `bson:"shop"`
}

func NewPlayerData(uid uint64) *Player {
	return &Player{
		UID:          uid,
		AssetMgr:     &asset.Mgr{},
		BagMgr:       &bag.Mgr{},
		CardGroupMgr: cardgroup.New(),
		ShopMgr:      &shop.Mgr{},
	}
}

// Load 用 AwaitTimeout 兜底，避免 mongo 卡死导致 PlayerActor goroutine 永久阻塞泄漏。
func Load(ctx tree.Context, uid uint64) (*Player, error) {
	data := NewPlayerData(uid)
	_, err := ctx.Request(
		tree.MustLookup(constant.Mongo),
		&mongoRpc.FindOne{
			Collection: collPlayer,
			Filter:     bson.M{"_id": uid},
			Result:     data,
		},
	).AwaitTimeout(loadTimeout)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Save fire-and-forget，upsert 玩家全量数据，不等待结果。
func (p *Player) Save() {
	tree.Send(
		tree.MustLookup(constant.Mongo),
		&mongoRpc.UpdateOne{
			Collection: collPlayer,
			Filter:     bson.M{"_id": p.UID},
			Update:     bson.M{"$set": p},
			Upsert:     true,
		},
	)
}
