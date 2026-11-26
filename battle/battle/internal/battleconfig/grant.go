package battleconfig

import (
	"fmt"
	"math"

	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	common "github.com/gogu-x/gogs/pb/cspb/pb_common"
	"google.golang.org/protobuf/proto"
)

// BuildGrantRole 在私有角色副本上叠加装备或天赋，复用开场的属性和技能编译流程。
func (c *Compiler) BuildGrantRole(role *pb.Role, grant *pb.BattleGrant) (*pb.Role, engine.UnitConfig, error) {
	var empty engine.UnitConfig
	if role == nil {
		return nil, empty, fmt.Errorf("缺少角色入场快照")
	}
	next := proto.Clone(role).(*pb.Role)
	switch grant.GetEffect().(type) {
	case *pb.BattleGrant_EquipConfigId:
		next.Equips = append(next.GetEquips(), &pb.RoleEquip{EquipId: grant.GetGrantId(), EquipConfigId: grant.GetEquipConfigId()})
	case *pb.BattleGrant_TalentNodeId:
		cfg := glconf.GetTalentNodeCfgNoErr(grant.GetTalentNodeId())
		if cfg == nil {
			return nil, empty, fmt.Errorf("缺少天赋节点 %d", grant.GetTalentNodeId())
		}
		old := next.GetGrowthBonus()
		values := [4]int64{
			int64(old.GetMaxHpPermille()) + int64(cfg.GetMaxHpPermille()),
			int64(old.GetAttackPermille()) + int64(cfg.GetAttackPermille()),
			int64(old.GetDefensePermille()) + int64(cfg.GetDefensePermille()),
			int64(old.GetSpeedPermille()) + int64(cfg.GetSpeedPermille()),
		}
		for _, v := range values {
			if v < 0 || v > math.MaxInt32 {
				return nil, empty, fmt.Errorf("天赋加成超出范围")
			}
		}
		next.GrowthBonus = &common.GrowthBonuses{MaxHpPermille: int32(values[0]), AttackPermille: int32(values[1]), DefensePermille: int32(values[2]), SpeedPermille: int32(values[3])}
	default:
		return nil, empty, fmt.Errorf("未知角色增益")
	}
	compiled, err := c.BuildJoinedRole(next)
	return next, compiled, err
}

// BuildGrantStatus 通过战斗状态表解析玩法发放的状态。
func (c *Compiler) BuildGrantStatus(id int32) (engine.StatusConfig, error) { return c.buildStatus(id) }
