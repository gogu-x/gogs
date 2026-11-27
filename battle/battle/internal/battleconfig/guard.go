package battleconfig

import (
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// heavyStrikeWindupMS 是守关怪物从举刀到落刀的战斗毫秒数。
const heavyStrikeWindupMS = 1500

// supportsGuard 为爬塔和首领战的玩家开放基础防守动作。
func supportsGuard(battleType pb.BattleType) bool {
	return battleType == pb.BattleType_BATTLE_TYPE_TOWER || battleType == pb.BattleType_BATTLE_TYPE_BOSS
}

// isGuardableHeavyStrike 识别技能表中五档单体重击，同编号的玩家技能仍即时结算。
func isGuardableHeavyStrike(skillID int32) bool {
	switch skillID {
	case 3,
		23,
		43,
		63,
		83:
		return true
	}
	return false
}

// BuildEncounterRole 编译初始或增援玩家，并按本场玩法授予防守动作。
func (c *Compiler) BuildEncounterRole(role *pb.Role, battleType pb.BattleType) (engine.UnitConfig, error) {
	unit, err := c.BuildJoinedRole(role)
	if err == nil {
		unit.GuardEnabled = supportsGuard(battleType)
	}
	return unit, err
}
