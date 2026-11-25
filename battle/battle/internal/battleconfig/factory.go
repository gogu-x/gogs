package battleconfig

import (
	"fmt"
	"github.com/gogu-x/gogs/battle/robot/strategy"
	"strconv"

	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
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
	attributes, err := c.CalculateRoleAttributes(role)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	statuses, err := c.buildInitialStatuses(config.InitialStatusIDs)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	charges, err := buildSoulJadeCharges(role)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	return engine.UnitConfig{InstanceID: role.GetRoleId(), Team: team, Position: config.DefaultPosition, MaxHP: attributes.MaxHP, Attack: attributes.Attack, Defense: attributes.Defense, Speed: attributes.Speed, BasicSkill: basic, ActiveSkills: active, InitialStatus: statuses, PassiveCharges: charges}, nil
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

func (c *Compiler) buildMonsterUnit(monsterID, position int32) (engine.UnitConfig, error) {
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
	charges := make([]engine.ChargeEffectConfig, 0)
	for index, effect := range config.Effects {
		if effect == nil {
			return engine.SkillConfig{}, fmt.Errorf("skill battle config id=%d has nil effect at index=%d", id, index)
		}
		definition, exists := c.effectDefinitions[effect.Typ]
		if !exists {
			return engine.SkillConfig{}, fmt.Errorf("skill battle config id=%d has unknown effect type=%q", id, effect.Typ)
		}
		if definition.Charge {
			threshold, gain, coefficient := effect.GetId(), effect.GetVal(), effect.GetPro()
			triggers := definition.Triggers
			if threshold <= 0 || gain < 0 || (!triggers && gain == 0) || (triggers && coefficient <= 0) {
				return engine.SkillConfig{}, fmt.Errorf("skill battle config id=%d has invalid charge effect at index=%d", id, index)
			}
			charges = append(charges, engine.ChargeEffectConfig{
				Kind: definition.Kind, Threshold: int64(threshold), Gain: gain,
				Trigger:       triggers,
				TriggerEffect: engine.EffectConfig{Kind: definition.Kind, CoefficientPermille: int64(coefficient)},
			})
			continue
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
	return engine.SkillConfig{ID: strconv.Itoa(int(config.CfgID)), Cooldown: int(config.Cooldown), TargetRule: pb.TargetRule(config.TargetRule), Effects: effects, Charges: charges, WindupTicks: int64(config.WindupTicks), RecoveryTicks: int64(config.RecoveryTicks)}, nil
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
	modifiers := glconf.GetBattleStatusAttributes(config)
	status := engine.StatusConfig{
		ID: strconv.Itoa(int(config.CfgID)), Kind: pb.StatusType(config.Kind),
		DurationTurns: int(config.DurationTurns), Potency: config.Potency,
		DurationMS: config.DurationMS, IntervalMS: config.IntervalMS,
		Modifier: engine.AttributeModifier{AttackFlat: modifiers[1], DefenseFlat: modifiers[2], SpeedFlat: modifiers[3]},
	}
	if err := status.Validate(); err != nil {
		return engine.StatusConfig{}, err
	}
	return status, nil
}

// addEquippedSkills 仅从游戏服确认的已穿戴装备读取法宝主动技能。
func (c *Compiler) addEquippedSkills(unit *engine.UnitConfig, role *pb.Role) error {
	seen := map[string]bool{unit.BasicSkill.ID: true}
	for _, skill := range unit.ActiveSkills {
		seen[skill.ID] = true
	}
	for _, equipped := range role.GetEquips() {
		equip := glconf.GetBattleEquipCfg(equipped.GetEquipConfigId())
		if equip == nil {
			return fmt.Errorf("缺少装备配置 %d", equipped.GetEquipConfigId())
		}
		id := equip.GetActiveSkillID()
		if id == 0 {
			continue
		}
		skill, err := c.buildSkill(id)
		if err != nil {
			return err
		}
		if !seen[skill.ID] {
			unit.ActiveSkills = append(unit.ActiveSkills, skill)
			seen[skill.ID] = true
		}
	}
	return nil
}

// configureMonsterAI 校验配置的每一步均属于怪物自身技能。
func configureMonsterAI(monsterID int32, unit *engine.UnitConfig) error {
	cfg := glconf.GetBattleMonsterCfg(monsterID)
	if cfg == nil {
		return fmt.Errorf("缺少怪物配置 %d", monsterID)
	}
	if cfg.AIFirstDelayTicks < 0 {
		return fmt.Errorf("怪物 %d 的首次出手延迟不能为负数", monsterID)
	}
	allowed := map[string]bool{unit.BasicSkill.ID: true}
	for _, skill := range unit.ActiveSkills {
		allowed[skill.ID] = true
	}
	var sequence []string
	for _, id := range cfg.AISkillSequence {
		skill := strconv.FormatInt(int64(id), 10)
		if !allowed[skill] {
			return fmt.Errorf("怪物 %d 的出招序列包含未携带技能 %d", monsterID, id)
		}
		sequence = append(sequence, skill)
	}
	rules, err := strategy.Compile(cfg.AICastRules, allowed)
	if err != nil {
		return fmt.Errorf("怪物 %d: %w", monsterID, err)
	}
	unit.CastRules = rules
	unit.RobotSequence = sequence
	unit.RobotFirstDelay = int64(cfg.AIFirstDelayTicks)
	return nil
}

// BuildJoinedRole 从 Game 的角色快照编译增援单位，使用与开场相同的装备技能规则。
func (c *Compiler) BuildJoinedRole(role *pb.Role) (engine.UnitConfig, error) {
	u, err := c.buildRoleUnit(role, pb.BattleTeam_BATTLE_TEAM_ATTACKER)
	if err == nil {
		err = c.addEquippedSkills(&u, role)
	}
	return u, err
}
