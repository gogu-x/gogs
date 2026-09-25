package configcompiler

import (
	"fmt"
	"math"
	"strconv"

	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/battle/battle/internal/statuskind"
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	cspb "github.com/gogu-x/gogs/pb/cspb/pb_common"
)

func (c *Compiler) buildRoleUnit(role *pb.Role, team pb.BattleTeam) (engine.UnitConfig, error) {
	if role == nil || role.GetRoleId() == "" || role.GetRoleConfigId() == 0 {
		return engine.UnitConfig{}, fmt.Errorf("role id and role config id are required")
	}
	config := glconf.GetBattleRoleCfg(role.GetRoleConfigId())
	if config == nil {
		return engine.UnitConfig{}, fmt.Errorf("role battle config not found: role_config_id=%d", role.GetRoleConfigId())
	}
	basic, err := c.buildSkill(config.BasicSkillID)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	active, err := c.buildRoleSkills(role, config)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	attributes, err := c.applyRoleEquips(role, config)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	statuses, err := c.buildInitialStatuses(config.InitialStatusIDs)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	return engine.UnitConfig{InstanceID: role.GetRoleId(), Team: team, Position: config.DefaultPosition, MaxHP: attributes.MaxHP, Attack: attributes.Attack, Defense: attributes.Defense, Speed: attributes.Speed, BasicSkill: basic, ActiveSkills: active, InitialStatus: statuses}, nil
}

func (c *Compiler) buildRoleSkills(role *pb.Role, config *glconf.BattleRoleCfg) ([]engine.SkillConfig, error) {
	skills := role.GetSkills()
	if len(skills) == 0 {
		ids := config.ActiveSkillIDs
		active := make([]engine.SkillConfig, 0, len(ids))
		for _, id := range ids {
			skill, err := c.buildSkill(id)
			if err != nil {
				return nil, err
			}
			active = append(active, skill)
		}
		return active, nil
	}
	active := make([]engine.SkillConfig, 0, len(skills))
	for _, roleSkill := range skills {
		if roleSkill == nil {
			return nil, fmt.Errorf("role id=%s has nil skill", role.GetRoleId())
		}
		skill, err := c.buildSkill(roleSkill.GetSkillConfigId())
		if err != nil {
			return nil, err
		}
		if c.skills != nil {
			skill, err = c.skills.Apply(roleSkill, skill)
			if err != nil {
				return nil, fmt.Errorf("role id=%s skill config id=%d progression: %w", role.GetRoleId(), roleSkill.GetSkillConfigId(), err)
			}
		}
		active = append(active, skill)
	}
	return active, nil
}

func (c *Compiler) applyRoleEquips(role *pb.Role, config *glconf.BattleRoleCfg) (RoleAttributes, error) {
	equips := make([]*glconf.BattleEquipCfg, 0, len(role.GetEquips()))
	for _, roleEquip := range role.GetEquips() {
		if roleEquip == nil {
			return RoleAttributes{}, fmt.Errorf("role id=%s has nil equip", role.GetRoleId())
		}
		equip := glconf.GetBattleEquipCfg(roleEquip.GetEquipConfigId())
		if equip == nil {
			return RoleAttributes{}, fmt.Errorf("equip battle config not found: equip_config_id=%d", roleEquip.GetEquipConfigId())
		}
		equips = append(equips, equip)
	}
	if c.roleStats == nil {
		return RoleAttributes{}, fmt.Errorf("battle role stat calculator is not configured")
	}
	return c.roleStats.Calculate(role, config, equips)
}

func battleAttributeValues(modifiers []*cspb.TypIDVal) [4]int64 {
	var totals [4]int64
	for _, modifier := range modifiers {
		if modifier == nil || modifier.Typ != "battle_attribute" {
			continue
		}
		// BattleEquipCfg and BattleStatusCfg define IDs 1..4 as
		// max_hp, attack, defense, and speed, respectively.
		index := int(modifier.Id) - 1
		if index >= 0 && index < len(totals) {
			totals[index] += modifier.Val
		}
	}
	return totals
}

func monsterMemberValues(member *cspb.TypIDVal) (int32, int32, error) {
	if member == nil {
		return 0, 0, fmt.Errorf("has nil monster member")
	}
	if member.Typ != "battle_monster" {
		return 0, 0, fmt.Errorf("has invalid monster member type=%q", member.Typ)
	}
	if member.Id == 0 {
		return 0, 0, fmt.Errorf("has empty monster config id")
	}
	if member.Val <= 0 || member.Val > math.MaxInt32 {
		return 0, 0, fmt.Errorf("has invalid monster position=%d", member.Val)
	}
	return member.Id, int32(member.Val), nil
}

func (c *Compiler) buildMonsterUnit(member *cspb.TypIDVal) (engine.UnitConfig, error) {
	monsterID, position, err := monsterMemberValues(member)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	config := glconf.GetBattleMonsterCfg(monsterID)
	if config == nil {
		return engine.UnitConfig{}, fmt.Errorf("monster battle config not found: monster_config_id=%d", monsterID)
	}
	basic, err := c.buildSkill(config.BasicSkillID)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	active := make([]engine.SkillConfig, 0, len(config.ActiveSkillIDs))
	for _, id := range config.ActiveSkillIDs {
		skill, err := c.buildSkill(id)
		if err != nil {
			return engine.UnitConfig{}, err
		}
		active = append(active, skill)
	}
	statuses, err := c.buildInitialStatuses(config.InitialStatusIDs)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	instanceID := "monster-" + strconv.Itoa(int(monsterID)) + "-" + strconv.Itoa(int(position))
	return engine.UnitConfig{InstanceID: instanceID, Team: pb.BattleTeam_BATTLE_TEAM_DEFENDER, Position: position, MaxHP: config.MaxHP, Attack: config.Attack, Defense: config.Defense, Speed: config.Speed, BasicSkill: basic, ActiveSkills: active, InitialStatus: statuses}, nil
}

func (c *Compiler) buildSkill(id int32) (engine.SkillConfig, error) {
	config := glconf.GetBattleSkillCfg(id)
	if config == nil {
		return engine.SkillConfig{}, fmt.Errorf("skill battle config not found: skill_config_id=%d", id)
	}
	effects := make([]engine.EffectConfig, 0, len(config.Effects))
	for index, effect := range config.Effects {
		if effect == nil {
			return engine.SkillConfig{}, fmt.Errorf("skill battle config id=%d has nil effect at index=%d", id, index)
		}
		definition, exists := c.effectDefinitions[effect.Typ]
		if !exists {
			return engine.SkillConfig{}, fmt.Errorf("skill battle config id=%d has unknown effect type=%q", id, effect.Typ)
		}
		compiledEffect := engine.EffectConfig{Kind: definition.Kind, CoefficientPermille: int64(effect.Pro), Flat: effect.Val, StatusID: strconv.Itoa(int(effect.Id))}
		if definition.RequiresStatus {
			status, err := c.buildStatus(effect.Id)
			if err != nil {
				return engine.SkillConfig{}, fmt.Errorf("skill battle config id=%d effect index=%d: %w", id, index, err)
			}
			compiledEffect.Status = &status
		}
		effects = append(effects, compiledEffect)
	}
	if config.WindupTicks <= 0 || config.RecoveryTicks <= 0 {
		return engine.SkillConfig{}, fmt.Errorf("skill battle config id=%d requires positive windup_ticks and recovery_ticks", id)
	}
	return engine.SkillConfig{ID: strconv.Itoa(int(config.CfgID)), Cooldown: int(config.Cooldown), TargetRule: pb.TargetRule(config.TargetRule), Effects: effects, WindupTicks: int64(config.WindupTicks), RecoveryTicks: int64(config.RecoveryTicks)}, nil
}

func (c *Compiler) buildInitialStatuses(ids []int32) ([]engine.StatusConfig, error) {
	statuses := make([]engine.StatusConfig, 0, len(ids))
	for _, id := range ids {
		status, err := c.buildStatus(id)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func (c *Compiler) buildStatus(id int32) (engine.StatusConfig, error) {
	config := glconf.GetBattleStatusCfg(id)
	if config == nil {
		return engine.StatusConfig{}, fmt.Errorf("battle status config not found: status_config_id=%d", id)
	}
	modifiers := battleAttributeValues(config.Modifier)
	return engine.StatusConfig{
		ID: strconv.Itoa(int(config.CfgID)), Kind: statuskind.Kind(config.Kind),
		DurationTurns: int(config.DurationTurns), Potency: config.Potency,
		Modifier: engine.AttributeModifier{AttackFlat: modifiers[1], DefenseFlat: modifiers[2], SpeedFlat: modifiers[3]},
	}, nil
}
