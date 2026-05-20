package player

import "github.com/gogu-x/gogs/game/constant"

// Mgr 玩家管理器，在 GameActor 内部使用，无需加锁
type Mgr struct {
	players map[uint64]*Player
}

func NewManager() *Mgr {
	return &Mgr{players: make(map[uint64]*Player)}
}

func (m *Mgr) Add(p *Player) {
	m.players[p.UID] = p
}

func (m *Mgr) Remove(uid uint64) {
	delete(m.players, uid)
}

func (m *Mgr) Get(uid uint64) (*Player, bool) {
	p, ok := m.players[uid]
	return p, ok
}

func (m *Mgr) Count() int {
	return len(m.players)
}

func (m *Mgr) SetOnline(uid uint64, online bool) {
	if p, ok := m.players[uid]; ok {
		if online {
			p.State = constant.PlayerStateOnline
		} else {
			p.State = constant.PlayerStateOffline
		}
	}
}
