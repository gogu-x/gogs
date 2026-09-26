package player

// PlayerMgr 管理本节点所有在线玩家，只在 Play Actor goroutine 内访问，无需加锁。
type PlayerMgr struct {
	players      map[uint64]*Player
	online       map[uint64]bool
	sessions     map[uint64]uint64
	nextCallback uint64
}

// NewPlayerMgr 创建在线玩家管理器。
func NewPlayerMgr() *PlayerMgr {
	return &PlayerMgr{
		players:  make(map[uint64]*Player),
		online:   make(map[uint64]bool),
		sessions: make(map[uint64]uint64),
	}
}

// Add 注册（或替换）一个在线玩家。
func (mgr *PlayerMgr) Add(p *Player) {
	if p == nil {
		return
	}
	uid := p.GetUID()
	mgr.players[uid] = p
	mgr.online[uid] = true
	mgr.sessions[uid]++
}

// Get 返回已加载的玩家；不存在返回 nil。
func (mgr *PlayerMgr) Get(uid uint64) *Player {
	return mgr.players[uid]
}

// Remove 移除在线玩家，返回被移除的对象（不存在返回 nil）。
func (mgr *PlayerMgr) Remove(uid uint64) *Player {
	p, ok := mgr.players[uid]
	if !ok {
		return nil
	}
	delete(mgr.players, uid)
	delete(mgr.online, uid)
	delete(mgr.sessions, uid)
	return p
}

// MarkOffline 标记会话断开的玩家，并返回本次会话版本。
func (mgr *PlayerMgr) MarkOffline(uid uint64) (uint64, *Player) {
	p := mgr.players[uid]
	if p == nil {
		return 0, nil
	}
	mgr.sessions[uid]++
	mgr.online[uid] = false
	return mgr.sessions[uid], p
}

// RemoveOffline 在玩家仍处于指定离线会话版本时移除其内存数据。
func (mgr *PlayerMgr) RemoveOffline(uid uint64, session uint64) bool {
	if mgr.online[uid] || mgr.sessions[uid] != session {
		return false
	}
	mgr.Remove(uid)
	return true
}

// RemoveSavedOffline 移除已成功存盘且当前仍离线的玩家。
func (mgr *PlayerMgr) RemoveSavedOffline(uid uint64, p *Player) bool {
	if mgr.online[uid] || mgr.players[uid] != p {
		return false
	}
	mgr.Remove(uid)
	return true
}

// IsOnline 返回玩家当前是否有在线会话。
func (mgr *PlayerMgr) IsOnline(uid uint64) bool { return mgr.online[uid] }

// Count 返回当前在线玩家数。
func (mgr *PlayerMgr) Count() int {
	count := 0
	for _, online := range mgr.online {
		if online {
			count++
		}
	}
	return count
}

// AllCount 返回内存中在线或待存盘玩家的总数。
func (mgr *PlayerMgr) AllCount() int { return len(mgr.players) }

// RangePlayer 遍历所有在线和待离线存盘玩家，cb 返回 false 时终止。
func (mgr *PlayerMgr) RangePlayer(cb func(*Player) bool) {
	for _, p := range mgr.players {
		if !cb(p) {
			return
		}
	}
}
