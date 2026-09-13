package core

// 模块级事件名。业务包通过 Context.Event() 注册与触发。
const (
	ServerStart    = "ServerStart"
	PlayerOnLogin  = "PlayerOnLogin"
	PlayerOnLogout = "PlayerOnLogout"

	// BattleSettled 在一场战斗结算完成后广播。arg 里带 "settlement"
	// （*battle.Settlement）。任务、成就、活动进度这类模块订阅它即可，
	// 不需要知道结果是从哪个 Actor 投递过来的。
	BattleSettled = "BattleSettled"
)
