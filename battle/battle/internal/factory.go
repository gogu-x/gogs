package internal

import (
	"fmt"
	"math"
	"strconv"

	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	cspb "github.com/gogu-x/gogs/pb/cspb/pb_common"
)

const (
	battleAttributeType             = "battle_attribute"
	battleMonsterType               = "battle_monster"
	battleDamageType                = "battle_damage"
	battleHealType                  = "battle_heal"
	battleStatusType                = "battle_status"
	defaultSkillWindupTicks         = int64(3)
	defaultSkillRecoveryTicks       = int64(3)
	battleAttributeMaxHP      int32 = iota + 1
	battleAttributeAttack
	battleAttributeDefense
	battleAttributeSpeed
)

func newBattleFromRequest(battleID string, seed uint64, request *pb.StartBattleReq) (*engine.Battle, error) {
	if request == nil {
		return nil, fmt.Errorf("battle request is required")
	}
	battleType := request.GetBattleType()
	if err := validateBattleRequest(request); err != nil {
		return nil, err
	}
	rule := glconf.GetBattleRuleCfg(int32(battleType))
	if rule == nil {
		return nil, fmt.Errorf("battle rule config not found: battle_type=%d", battleType)
	}
	units, err := buildBattleUnits(request)
	if err != nil {
		return nil, err
	}
	return engine.NewBattle(engine.Setup{
		BattleID:   battleID,
		BattleType: battleType,
		Seed:       seed,
		Rules: engine.Rules{
			ATBThreshold:           rule.ATBThreshold,
			MaxActions:             int(rule.MaxActions),
			TickDurationMS:         rule.TickDurationMS,
			DamageVariancePermille: rule.DamageVariancePermille,
			CritChancePermille:     rule.CritChancePermille,
			CritMultiplierPermille: rule.CritMultiplierPermille,
		},
		Units: units,
	})
}

func validateBattleRequest(request *pb.StartBattleReq) error {
	attackerRoles := request.GetAttackerRoles()
	defenderRoles := request.GetDefenderRoles()
	monsterGroupID := request.GetMonsterGroupConfigId()
	switch request.GetBattleType() {
	case pb.BattleType_BATTLE_TYPE_TOWER:
		fallthrough
	case pb.BattleType_BATTLE_TYPE_BOSS:
		fallthrough
	case pb.BattleType_BATTLE_TYPE_STORY:
		fallthrough
	case pb.BattleType_BATTLE_TYPE_TEST:
		if len(attackerRoles) == 0 {
			return fmt.Errorf("battle type=%d requires attacker roles", request.GetBattleType())
		}
		if len(defenderRoles) != 0 {
			return fmt.Errorf("battle type=%d forbids defender roles", request.GetBattleType())
		}
		if monsterGroupID == 0 {
			return fmt.Errorf("battle type=%d requires monster group config id", request.GetBattleType())
		}
	case pb.BattleType_BATTLE_TYPE_PVP:
		if len(attackerRoles) != 1 || len(defenderRoles) != 1 {
			return fmt.Errorf("pvp requires exactly one attacker role and one defender role")
		}
		if monsterGroupID != 0 {
			return fmt.Errorf("pvp forbids monster group config id")
		}
	default:
		return fmt.Errorf("unsupported battle type=%d", request.GetBattleType())
	}
	return nil
}

func buildBattleUnits(request *pb.StartBattleReq) ([]engine.UnitConfig, error) {
	units := make([]engine.UnitConfig, 0, len(request.GetAttackerRoles())+len(request.GetDefenderRoles()))
	positions := make(map[int32]struct{})
	for _, role := range request.GetAttackerRoles() {
		unit, err := buildRoleUnit(role, pb.BattleTeam_BATTLE_TEAM_ATTACKER)
		if err != nil {
			return nil, err
		}
		if err := addUnitPosition(positions, unit); err != nil {
			return nil, err
		}
		units = append(units, unit)
	}
	if request.GetBattleType() == pb.BattleType_BATTLE_TYPE_PVP {
		for _, role := range request.GetDefenderRoles() {
			unit, err := buildRoleUnit(role, pb.BattleTeam_BATTLE_TEAM_DEFENDER)
			if err != nil {
				return nil, err
			}
			units = append(units, unit)
		}
		return units, nil
	}
	group := glconf.GetBattleMonsterGroupCfg(request.GetMonsterGroupConfigId())
	if group == nil {
		return nil, fmt.Errorf("battle monster group config not found: group_config_id=%d", request.GetMonsterGroupConfigId())
	}
	if len(group.Members) == 0 {
		return nil, fmt.Errorf("battle monster group config id=%d has no members", group.CfgID)
	}
	groupPositions := make(map[int32]struct{}, len(group.Members))
	for _, member := range group.Members {
		_, position, err := monsterMemberValues(member)
		if err != nil {
			return nil, fmt.Errorf("battle monster group config id=%d: %w", group.CfgID, err)
		}
		if _, exists := groupPositions[position]; exists {
			return nil, fmt.Errorf("battle monster group config id=%d has duplicate position=%d", group.CfgID, position)
		}
		groupPositions[position] = struct{}{}
		unit, err := buildMonsterUnit(member)
		if err != nil {
			return nil, err
		}
		units = append(units, unit)
	}
	return units, nil
}

func addUnitPosition(positions map[int32]struct{}, unit engine.UnitConfig) error {
	if _, exists := positions[unit.Position]; exists {
		return fmt.Errorf("duplicate configured attacker position=%d", unit.Position)
	}
	positions[unit.Position] = struct{}{}
	return nil
}

func buildRoleUnit(role *pb.Role, team pb.BattleTeam) (engine.UnitConfig, error) {
	if role == nil || role.GetRoleId() == "" || role.GetRoleConfigId() == 0 {
		return engine.UnitConfig{}, fmt.Errorf("role id and role config id are required")
	}
	config := glconf.GetBattleRoleCfg(role.GetRoleConfigId())
	if config == nil {
		return engine.UnitConfig{}, fmt.Errorf("role battle config not found: role_config_id=%d", role.GetRoleConfigId())
	}
	basic, err := buildSkill(config.BasicSkillID)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	active, err := buildRoleSkills(role, config)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	maxHP, attack, defense, speed, err := applyRoleEquips(role, config)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	statuses, err := buildInitialStatuses(config.InitialStatusIDs)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	return engine.UnitConfig{InstanceID: role.GetRoleId(), Team: team, Position: config.DefaultPosition, MaxHP: maxHP, Attack: attack, Defense: defense, Speed: speed, BasicSkill: basic, ActiveSkills: active, InitialStatus: statuses}, nil
}

func buildRoleSkills(role *pb.Role, config *glconf.BattleRoleCfg) ([]engine.SkillConfig, error) {
	skills := role.GetSkills()
	if len(skills) == 0 {
		ids := config.ActiveSkillIDs
		active := make([]engine.SkillConfig, 0, len(ids))
		for _, id := range ids {
			skill, err := buildSkill(id)
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
		skill, err := buildSkill(roleSkill.GetSkillConfigId())
		if err != nil {
			return nil, err
		}
		active = append(active, skill)
	}
	return active, nil
}

func applyRoleEquips(role *pb.Role, config *glconf.BattleRoleCfg) (int64, int64, int64, int64, error) {
	maxHP, attack := config.MaxHP, config.Attack
	defense, speed := config.Defense, config.Speed
	for _, roleEquip := range role.GetEquips() {
		if roleEquip == nil {
			return 0, 0, 0, 0, fmt.Errorf("role id=%s has nil equip", role.GetRoleId())
		}
		equip := glconf.GetBattleEquipCfg(roleEquip.GetEquipConfigId())
		if equip == nil {
			return 0, 0, 0, 0, fmt.Errorf("equip battle config not found: equip_config_id=%d", roleEquip.GetEquipConfigId())
		}
		maxHP += battleAttributeValue(equip.Modifier, battleAttributeMaxHP)
		attack += battleAttributeValue(equip.Modifier, battleAttributeAttack)
		defense += battleAttributeValue(equip.Modifier, battleAttributeDefense)
		speed += battleAttributeValue(equip.Modifier, battleAttributeSpeed)
	}
	return maxHP, attack, defense, speed, nil
}

func battleAttributeValue(modifiers []*cspb.TypIDVal, attributeID int32) int64 {
	var total int64
	for _, modifier := range modifiers {
		if modifier != nil && modifier.Typ == battleAttributeType && modifier.Id == attributeID {
			total += modifier.Val
		}
	}
	return total
}

func monsterMemberValues(member *cspb.TypIDVal) (int32, int32, error) {
	if member == nil {
		return 0, 0, fmt.Errorf("has nil monster member")
	}
	if member.Typ != battleMonsterType {
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

func buildMonsterUnit(member *cspb.TypIDVal) (engine.UnitConfig, error) {
	monsterID, position, err := monsterMemberValues(member)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	config := glconf.GetBattleMonsterCfg(monsterID)
	if config == nil {
		return engine.UnitConfig{}, fmt.Errorf("monster battle config not found: monster_config_id=%d", monsterID)
	}
	basic, err := buildSkill(config.BasicSkillID)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	active := make([]engine.SkillConfig, 0, len(config.ActiveSkillIDs))
	for _, id := range config.ActiveSkillIDs {
		skill, err := buildSkill(id)
		if err != nil {
			return engine.UnitConfig{}, err
		}
		active = append(active, skill)
	}
	statuses, err := buildInitialStatuses(config.InitialStatusIDs)
	if err != nil {
		return engine.UnitConfig{}, err
	}
	instanceID := "monster-" + strconv.Itoa(int(monsterID)) + "-" + strconv.Itoa(int(position))
	return engine.UnitConfig{InstanceID: instanceID, Team: pb.BattleTeam_BATTLE_TEAM_DEFENDER, Position: position, MaxHP: config.MaxHP, Attack: config.Attack, Defense: config.Defense, Speed: config.Speed, BasicSkill: basic, ActiveSkills: active, InitialStatus: statuses}, nil
}

func buildSkill(id int32) (engine.SkillConfig, error) {
	config := glconf.GetBattleSkillCfg(id)
	if config == nil {
		return engine.SkillConfig{}, fmt.Errorf("skill battle config not found: skill_config_id=%d", id)
	}
	effects := make([]engine.EffectConfig, 0, len(config.Effects))
	for index, effect := range config.Effects {
		if effect == nil {
			return engine.SkillConfig{}, fmt.Errorf("skill battle config id=%d has nil effect at index=%d", id, index)
		}
		var kind engine.EffectKind
		switch effect.Typ {
		case battleDamageType:
			kind = engine.EffectKind(1)
		case battleHealType:
			kind = engine.EffectKind(2)
		case battleStatusType:
			kind = engine.EffectKind(3)
		default:
			return engine.SkillConfig{}, fmt.Errorf("skill battle config id=%d has unknown effect type=%q", id, effect.Typ)
		}
		effects = append(effects, engine.EffectConfig{Kind: kind, CoefficientPermille: int64(effect.Pro), Flat: effect.Val, StatusID: strconv.Itoa(int(effect.Id))})
	}
	windupTicks := int64(config.WindupTicks)
	if windupTicks <= 0 {
		windupTicks = defaultSkillWindupTicks
	}
	recoveryTicks := int64(config.RecoveryTicks)
	if recoveryTicks <= 0 {
		recoveryTicks = defaultSkillRecoveryTicks
	}
	return engine.SkillConfig{ID: strconv.Itoa(int(config.CfgID)), Cooldown: int(config.Cooldown), TargetRule: pb.TargetRule(config.TargetRule), Effects: effects, WindupTicks: windupTicks, RecoveryTicks: recoveryTicks}, nil
}

func buildInitialStatuses(ids []int32) ([]engine.StatusConfig, error) {
	statuses := make([]engine.StatusConfig, 0, len(ids))
	for _, id := range ids {
		config := glconf.GetBattleStatusCfg(id)
		if config == nil {
			return nil, fmt.Errorf("battle status config not found: status_config_id=%d", id)
		}
		statuses = append(statuses, engine.StatusConfig{
			ID:            strconv.Itoa(int(config.CfgID)),
			Kind:          engine.StatusKind(config.Kind),
			DurationTurns: int(config.DurationTurns),
			Potency:       config.Potency,
			Modifier: engine.AttributeModifier{
				AttackFlat:  battleAttributeValue(config.Modifier, battleAttributeAttack),
				DefenseFlat: battleAttributeValue(config.Modifier, battleAttributeDefense),
				SpeedFlat:   battleAttributeValue(config.Modifier, battleAttributeSpeed),
			},
		})
	}
	return statuses, nil
}
