package configcompiler

import (
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

type RoleAttributes struct {
	MaxHP   int64
	Attack  int64
	Defense int64
	Speed   int64
}

// RoleStatCalculator applies player growth and equipment modifiers once,
// during compilation of the authoritative battle creation request.
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

func (configuredRoleStatCalculator) Calculate(_ *pb.Role, role *glconf.BattleRoleCfg, equips []*glconf.BattleEquipCfg) (RoleAttributes, error) {
	attributes := RoleAttributes{MaxHP: role.MaxHP, Attack: role.Attack, Defense: role.Defense, Speed: role.Speed}
	for _, equip := range equips {
		modifiers := battleAttributeValues(equip.Modifier)
		attributes.MaxHP += modifiers[0]
		attributes.Attack += modifiers[1]
		attributes.Defense += modifiers[2]
		attributes.Speed += modifiers[3]
	}
	return attributes, nil
}
