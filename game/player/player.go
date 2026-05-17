package player

import "github.com/gogu-x/gogs/game/constant"

type Player struct {
	UID   uint64
	Name  string
	State int
}

// Manager 玩家管理器，在 GameActor 内部使用，无需加锁
type Manager struct {
	players map[uint64]*Player
}

func NewManager() *Manager {
	return &Manager{players: make(map[uint64]*Player)}
}

func (m *Manager) Add(p *Player) {
	m.players[p.UID] = p
}

func (m *Manager) Remove(uid uint64) {
	delete(m.players, uid)
}

func (m *Manager) Get(uid uint64) (*Player, bool) {
	p, ok := m.players[uid]
	return p, ok
}

func (m *Manager) Count() int {
	return len(m.players)
}

func (m *Manager) SetOnline(uid uint64, online bool) {
	if p, ok := m.players[uid]; ok {
		if online {
			p.State = constant.PlayerStateOnline
		} else {
			p.State = constant.PlayerStateOffline
		}
	}
}
