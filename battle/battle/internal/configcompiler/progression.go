package configcompiler

import (
	"fmt"
	"math"

	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// RoleAttributes contains compiled battle attributes.
//
//go:generate make -C ../../../.. accessor GEN_DIR=battle/battle/internal/configcompiler STRUCTS=RoleAttributes
type RoleAttributes struct {
	MaxHP   int64 // Maximum health.
	Attack  int64 // Attack power.
	Defense int64 // Defense.
	Speed   int64 // Action speed.
}

// RoleStatCalculator applies equipment and growth once during compilation.
type RoleStatCalculator interface {
	Calculate(*pb.Role, *glconf.BattleRoleCfg, []*glconf.BattleEquipCfg) (RoleAttributes, error)
}

type RoleSkillProgression interface {
	Apply(*pb.RoleSkill, engine.SkillConfig) (engine.SkillConfig, error)
}

type configuredRoleSkillProgression struct{}

func (configuredRoleSkillProgression) Apply(_ *pb.RoleSkill, skill engine.SkillConfig) (engine.SkillConfig, error) {
	return skill, nil
}

type configuredRoleStatCalculator struct{}

// CalculateRoleAttributes shares authoritative attribute compilation with read-only previews.
func (c *Compiler) CalculateRoleAttributes(role *pb.Role) (RoleAttributes, error) {
	if c == nil {
		return RoleAttributes{}, fmt.Errorf("battle config compiler is not initialized")
	}
	if role == nil {
		return RoleAttributes{}, fmt.Errorf("role is required")
	}
	config := glconf.GetBattleRoleCfg(role.GetRoleConfigId())
	if config == nil {
		return RoleAttributes{}, fmt.Errorf("role battle config not found: role_config_id=%d", role.GetRoleConfigId())
	}
	return c.applyRoleEquips(role, config)
}

func (configuredRoleStatCalculator) Calculate(roleState *pb.Role, role *glconf.BattleRoleCfg, equips []*glconf.BattleEquipCfg) (RoleAttributes, error) {
	attributes := RoleAttributes{MaxHP: role.MaxHP, Attack: role.Attack, Defense: role.Defense, Speed: role.Speed}
	for _, equip := range equips {
		modifiers := battleAttributeValues(equip.Modifier)
		attributes.MaxHP += modifiers[0]
		attributes.Attack += modifiers[1]
		attributes.Defense += modifiers[2]
		attributes.Speed += modifiers[3]
	}
	bonus := roleState.GetGrowthBonus()
	if bonus == nil {
		return attributes, nil
	}
	var err error
	if attributes.MaxHP, err = scaleGrowth(attributes.GetMaxHP(), bonus.GetMaxHpPermille()); err != nil {
		return RoleAttributes{}, err
	}
	if attributes.Attack, err = scaleGrowth(attributes.GetAttack(), bonus.GetAttackPermille()); err != nil {
		return RoleAttributes{}, err
	}
	if attributes.Defense, err = scaleGrowth(attributes.GetDefense(), bonus.GetDefensePermille()); err != nil {
		return RoleAttributes{}, err
	}
	if attributes.Speed, err = scaleGrowth(attributes.GetSpeed(), bonus.GetSpeedPermille()); err != nil {
		return RoleAttributes{}, err
	}
	return attributes, nil
}

func scaleGrowth(base int64, permille int32) (int64, error) {
	if permille < 0 {
		return 0, fmt.Errorf("growth modifier out of range: %d", permille)
	}
	factor := int64(1000) + int64(permille)
	if base < 0 || base > math.MaxInt64/factor {
		return 0, fmt.Errorf("battle attribute out of range: %d", base)
	}
	return base * factor / 1000, nil
}
