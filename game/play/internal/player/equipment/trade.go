package equipment

import (
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/google/uuid"
)

// PrepareTradeItems 校验交易装备配置并构造待发放实例，不修改玩家装备数据。
func PrepareTradeItems(id int32, count int64) ([]*Item, pb_common.ErrCode) {
	cfg := glconf.GetBattleEquipCfgNoErr(id)
	if cfg == nil || count <= 0 || count > 10000 {
		return nil, pb_common.ErrCode_CONFIG_UNAVAILABLE
	}
	items := make([]*Item, 0, count)
	for i := int64(0); i < count; i++ {
		items = append(items, &Item{ID: uuid.NewString(), ConfigID: id, Slot: cfg.GetSlot(), Level: cfg.GetInitialLevel(), RefineLevel: cfg.GetInitialRefineLevel()})
	}
	return items, pb_common.ErrCode_OK
}
