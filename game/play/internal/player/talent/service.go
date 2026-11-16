package talent

import (
	"github.com/gogu-x/gogs/game/play/internal/player/role"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_talent"
	"github.com/gogu-x/tree/tlog"
)

// Player 提供本模块业务需要的玩家内存数据。
type Player interface {
	GetUID() uint64
	GetRoleMgr() *role.Mgr
	GetTalentMgr() *Mgr
	MarkDirty()
}

// BuildData 根据玩家当前天赋数据组装快照；玩家没有天赋数据时返回 nil。
func BuildData(p Player) *pb_talent.RoleTalentData {
	roleID, level, breakthrough := p.GetRoleMgr().State()
	return View(p.GetTalentMgr(), roleID, EarnedPoints(level, breakthrough))
}

// apply 执行天赋变更并返回最新快照；业务校验全部通过后才修改数据，失败时数据保持不变。
func apply(p Player, operation func(*Mgr) pb_common.ErrCode) (*pb_talent.RoleTalentData, pb_common.ErrCode) {
	code := operation(p.GetTalentMgr())
	if code == pb_common.ErrCode_OK {
		p.MarkDirty()
	}
	tlog.Log.Info("[talent/apply] 天赋操作处理完成, code=%v, pid=%v", code, p.GetUID())
	return BuildData(p), code
}

// LearnTalent 按角色成长获得的点数学习指定天赋节点。
func LearnTalent(p Player, req *pb_talent.LearnRoleTalentReq) (*pb_talent.RoleTalentData, pb_common.ErrCode) {
	return apply(p, func(mgr *Mgr) pb_common.ErrCode {
		_, level, breakthrough := p.GetRoleMgr().State()
		return Learn(mgr, req.GetNodeId(), EarnedPoints(level, breakthrough))
	})
}

// ResetTalent 重置指定分支或全部分支的天赋。
func ResetTalent(p Player, req *pb_talent.ResetRoleTalentReq) (*pb_talent.RoleTalentData, pb_common.ErrCode) {
	return apply(p, func(mgr *Mgr) pb_common.ErrCode {
		return Reset(mgr, req.GetBranch())
	})
}
