package player

import (
	"time"

	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

const playerSaveInterval = time.Minute

func (mgr *PlayerMgr) GetCollection() string { return "player" }

func (mgr *PlayerMgr) GetPrototype() interface{} { return (*Player)(nil) }

func (mgr *PlayerMgr) GetIDField() string { return "_id" }

func (mgr *PlayerMgr) GetKey(data interface{}) interface{} {
	p, ok := data.(*Player)
	if !ok || p == nil {
		return nil
	}
	return p.GetUID()
}

func (mgr *PlayerMgr) Clone(data interface{}) interface{} {
	p, ok := data.(*Player)
	if !ok || p == nil {
		return nil
	}
	return p.DeepCopy()
}

func (mgr *PlayerMgr) Range(v func(interface{}) bool) {
	mgr.RangePlayer(func(p *Player) bool {
		return v(p)
	})
}

func (mgr *PlayerMgr) GetSaveInterval() time.Duration { return playerSaveInterval }

func (mgr *PlayerMgr) AfterSave(_ tree.Context, data interface{}) error {

	p, ok := data.(*Player)
	if !ok || p == nil {
		return nil
	}
	uid := p.GetUID()
	current := mgr.Get(uid)
	if current != nil && !mgr.IsOnline(uid) && mgr.RemoveSavedOffline(uid, current) {
		tlog.Log.Info("[player/PlayerMgr.AfterSave] 玩家离线存盘完成并移除, pid=%v", uid)
	}
	return nil
}

func (mgr *PlayerMgr) MackDirty() bool {
	dirty := false
	mgr.RangePlayer(func(p *Player) bool {
		if p.Dirty {
			dirty = true
			return false
		}
		return true
	})
	return dirty
}
