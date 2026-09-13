package battle

import pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"

// Participant 是一场战斗里的一名参战玩家。
//
// ServerID/NodeID 记的是该玩家当时所在的 Game 节点。同服战斗时它们就等于创建
// 这场战斗的节点；跨服队对战时才会各不相同 —— 正因为要把这个信息留住，
// 结算入口才不能只收一个 uid。
type Participant struct {
	UID      uint64
	ServerID int
	NodeID   int
}

// Settlement 是"一场战斗结束了，请结算"的完整输入。
//
// 刻意不让它退化成 (uid, ntf)：结算体一旦按单个 uid 写死，跨服时结算逻辑本身
// 就要跟着改。目前 Participants 只有一个人（同服版本），但结构按最终形状先定下来。
//
// Units 不区分归属：单位 id 里已经带了玩家信息（见 ctl_battle.buildDemoRequest 的
// "player-<uid>" 约定）。跨服真正落地时如果这个约定不够用，再补 ownership 字段。
type Settlement struct {
	BattleID     string
	BattleType   pb.BattleType
	Outcome      pb.BattleOutcome
	Units        []*pb.BattleUnitResult
	Checksum     string
	Participants []Participant
}
