package core

// 玩家业务事件名，由玩家角色注册监听器并触发。
const (
	PlayerOnLogin       = "PlayerOnLogin"
	PlayerOnLogout      = "PlayerOnLogout"
	PlayerOnServerOpen  = "PlayerOnServerOpen"
	PlayerOnServerClose = "PlayerOnServerClose"
	BattleSettled       = "BattleSettled"
)
