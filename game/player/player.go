package player

// Player 玩家基础数据，驻留在 PlayerActor 内存中
type Player struct {
	UID   uint64
	Name  string
	Level uint32
	State int
}
