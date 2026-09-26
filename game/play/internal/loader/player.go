package loader

import (
	"errors"
	"fmt"

	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/tree"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// OnLoadPlayer 登录时加载玩家数据
func OnLoadPlayer(p *core.Play, uid uint64, callback func(*core.Play, *player.Player, error)) bool {
	if p == nil {
		return false
	}
	if playerInfo := p.GetPlayerMgr().Get(uid); playerInfo != nil {
		callback(p, playerInfo, nil)
		return true
	}

	if p.Lifecycle.LoadOne(p.SystemContext(), (*player.Player)(nil), uid, func(callbackCtx tree.Context, value interface{}, err error) {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = nil
			value = nil
		}
		var loaded *player.Player
		if err == nil && value != nil {
			var valid bool
			loaded, valid = value.(*player.Player)
			if !valid || loaded == nil || loaded.GetUID() != uid {
				err = fmt.Errorf("invalid player document for uid=%d", uid)
				loaded = nil
			}
		}
		callback(p, loaded, nil)
	}) {
		return true
	}
	return true
}
