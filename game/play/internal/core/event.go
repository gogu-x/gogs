package core

// 模块级事件名。业务包通过 Context.Event() 注册与触发。
const (
	ServerStart    = "ServerStart"
	PlayerOnLogin  = "PlayerOnLogin"
	PlayerOnLogout = "PlayerOnLogout"
	BattleSettled  = "BattleSettled"
)
