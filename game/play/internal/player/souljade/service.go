package souljade

import (
	"github.com/gogu-x/gogs/game/play/internal/player/role"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_souljade"
	"github.com/gogu-x/tree/tlog"
)

// Player 提供本模块业务需要的玩家内存数据。
type Player interface {
	GetUID() uint64
	GetRoleMgr() *role.Mgr
	GetSoulJadeMgr() *Mgr
	MarkDirty()
}

// BuildData 根据玩家当前魂玉数据组装快照；玩家没有魂玉数据时返回 nil。
func BuildData(p Player) *pb_souljade.RoleSoulJadeData {
	roleID, _, _ := p.GetRoleMgr().State()
	return View(p.GetSoulJadeMgr(), roleID)
}

// apply 执行魂玉变更并返回最新快照；业务校验全部通过后才修改数据，失败时数据保持不变。
func apply(p Player, operation func(*Mgr) pb_common.ErrCode) (*pb_souljade.RoleSoulJadeData, pb_common.ErrCode) {
	code := operation(p.GetSoulJadeMgr())
	if code == pb_common.ErrCode_OK {
		p.MarkDirty()
	}
	tlog.Log.Info("[souljade/apply] 魂玉操作处理完成, code=%v, pid=%v", code, p.GetUID())
	return BuildData(p), code
}

// ChangeSoulJade 更换或卸下指定槽位的魂玉。
func ChangeSoulJade(p Player, req *pb_souljade.ChangeRoleSoulJadeReq) (*pb_souljade.RoleSoulJadeData, pb_common.ErrCode) {
	return apply(p, func(mgr *Mgr) pb_common.ErrCode {
		return Change(mgr, req.GetSlot(), req.GetInstanceId())
	})
}

// UpgradeSoulJade 升级指定魂玉。
func UpgradeSoulJade(p Player, req *pb_souljade.UpgradeRoleSoulJadeReq) (*pb_souljade.RoleSoulJadeData, pb_common.ErrCode) {
	return apply(p, func(mgr *Mgr) pb_common.ErrCode {
		return Upgrade(mgr, req.GetInstanceId())
	})
}

// RefineSoulJade 洗练指定魂玉的词条。
func RefineSoulJade(p Player, req *pb_souljade.RefineRoleSoulJadeReq) (*pb_souljade.RoleSoulJadeData, pb_common.ErrCode) {
	return apply(p, func(mgr *Mgr) pb_common.ErrCode {
		return Refine(mgr, req.GetInstanceId())
	})
}
