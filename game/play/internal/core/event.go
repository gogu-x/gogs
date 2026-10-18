package core

// 玩家业务事件名，由玩家角色注册监听器并触发。
const (
	// PlayerOnLogin 表示玩家完成登录时触发的业务事件。
	PlayerOnLogin = "PlayerOnLogin"
	// PlayerOnLogout 表示玩家会话退出时触发的业务事件。
	PlayerOnLogout = "PlayerOnLogout"
	// PlayerOnServerOpen 表示玩家角色收到服务器开启事件时触发的业务事件。
	PlayerOnServerOpen = "PlayerOnServerOpen"
	// PlayerOnServerClose 表示玩家角色收到服务器关闭事件时触发的业务事件。
	PlayerOnServerClose = "PlayerOnServerClose"
	// BattleSettled 表示玩家参与的战斗结算完成时触发的业务事件。
	BattleSettled = "BattleSettled"
)
