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

// BuildData 根据已加载的角色和模块状态组装快照。
func BuildData(p Player) *pb_talent.RoleTalentData {
	if p == nil {
		return nil
	}
	roleMgr := p.GetRoleMgr()
	roleID, level, breakthrough := roleMgr.State()
	return View(p.GetTalentMgr(), roleID, EarnedPoints(level, breakthrough))
}

// apply 校验角色与版本并执行模块变更，失败返回操作前快照。
func apply(
	p Player, requestID, expected uint64, operation func(*Mgr) pb_common.ErrCode,
) (data *pb_talent.RoleTalentData, code pb_common.ErrCode) {
	if p == nil {
		return nil, pb_common.ErrCode_LOADOUT_UNAVAILABLE
	}
	mgr := p.GetTalentMgr()
	before := mgr.GetRevision()
	defer func() {
		tlog.Log.Info("[talent/apply] 养成操作处理完成, request=%v, expected=%v, before=%v, after=%v, code=%v, pid=%v",
			requestID, expected, before, mgr.GetRevision(), code, p.GetUID())
	}()
	data = BuildData(p)
	roleMgr := p.GetRoleMgr()
	roleID, _, _ := roleMgr.State()
	if roleID == "" || mgr == nil || before == 0 {
		return data, pb_common.ErrCode_LOADOUT_UNAVAILABLE
	}
	if requestID == 0 || expected != before {
		return data, pb_common.ErrCode_STALE_REVISION
	}
	if code = operation(mgr); code != pb_common.ErrCode_OK {
		return data, code
	}
	p.MarkDirty()
	return BuildData(p), pb_common.ErrCode_OK
}

// LearnTalent 校验请求并执行对应的模块业务。
func LearnTalent(p Player, req *pb_talent.LearnRoleTalentReq) (*pb_talent.RoleTalentData, pb_common.ErrCode) {
	if req == nil {
		return nil, pb_common.ErrCode_PARAM
	}
	return apply(
		p, req.GetRequestId(), req.GetExpectedRevision(), func(mgr *Mgr) pb_common.ErrCode {
			roleMgr := p.GetRoleMgr()
			_, level, breakthrough := roleMgr.State()
			earned := EarnedPoints(level, breakthrough)
			return Learn(mgr, req.GetNodeId(), earned)
		},
	)
}

// ResetTalent 校验请求并执行对应的模块业务。
func ResetTalent(p Player, req *pb_talent.ResetRoleTalentReq) (*pb_talent.RoleTalentData, pb_common.ErrCode) {
	if req == nil {
		return nil, pb_common.ErrCode_PARAM
	}
	return apply(
		p, req.GetRequestId(), req.GetExpectedRevision(), func(mgr *Mgr) pb_common.ErrCode {
			return Reset(mgr, req.GetBranch())
		},
	)
}
