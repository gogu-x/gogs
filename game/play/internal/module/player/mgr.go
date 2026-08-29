package player

import (
	"time"
)

const collPlayer = "player"

// loadTimeout 是同步加载玩家数据的最大等待时间，超时则放弃，避免 goroutine 永久阻塞。
const loadTimeout = 5 * time.Second

// PlayerMgr 管理本节点所有在线玩家，只在 Play Actor goroutine 内访问，无需加锁。
type PlayerMgr struct {
	players map[uint64]*Player
}

func NewPlayerMgr() *PlayerMgr {
	return &PlayerMgr{
		players: make(map[uint64]*Player),
	}
}

// Add 注册（或替换）一个在线玩家。
func (mgr *PlayerMgr) Add(p *Player) {
	if p == nil {
		return
	}
	mgr.players[p.UID] = p
}

// Get 返回在线玩家，不在线返回 nil。
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
	return p
}

// Count 当前在线玩家数。
func (mgr *PlayerMgr) Count() int { return len(mgr.players) }

// Range 遍历所有在线玩家，cb 返回 false 时终止。
func (mgr *PlayerMgr) Range(cb func(*Player) bool) {
	for _, p := range mgr.players {
		if !cb(p) {
			return
		}
	}
}

func (mgr *PlayerMgr) Loader() {

}

// Save fire-and-forget，upsert 玩家全量数据，不等待结果。
func (mgr *PlayerMgr) Save() {
}

// OnSaveTimer 适配 TimeWheel Handler；回调在 Play Actor goroutine 内执行。
func (mgr *PlayerMgr) OnSaveTimer(_ interface{}) {
	mgr.Save()
}
