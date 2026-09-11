package internal

import (
	"fmt"
	"strconv"

	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
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
		BattleID: battleID,
		Seed:     seed,
		Rules: engine.Rules{
			ATBThreshold:           rule.GetATBThreshold(),
			MaxActions:             int(rule.GetMaxActions()),
			DamageVariancePermille: rule.GetDamageVariancePermille(),
			CritChancePermille:     rule.GetCritChancePermille(),
			CritMultiplierPermille: rule.GetCritMultiplierPermille(),
		},
		Units: units,
	})
}

func validateBattleRequest(request *pb.StartBattleReq) error {
	attackerRoles := request.GetAttackerRoles()
	defenderRoles := request.GetDefenderRoles()
	monsterGroupID := request.GetMonsterGroupConfigId()
	switch request.GetBattleType() {
	case pb.BattleType_BATTLE_TYPE_PVE:
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
	if err := glconf.ValidateBattleMonsterGroupCfg(group); err != nil {
		return nil, err
	}
	for _, member := range group.GetMembers() {
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
	basic, err := buildSkill(config.GetBasicSkillID())
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
	statuses, err := buildInitialStatuses(config.GetInitialStatusIDs())
	if err != nil {
		return engine.UnitConfig{}, err
	}
	return engine.UnitConfig{InstanceID: role.GetRoleId(), Team: team, Position: config.GetDefaultPosition(), MaxHP: maxHP, Attack: attack, Defense: defense, Speed: speed, BasicSkill: basic, ActiveSkills: active, InitialStatus: statuses}, nil
}

func buildRoleSkills(role *pb.Role, config *glconf.BattleRoleCfg) ([]engine.SkillConfig, error) {
	skills := role.GetSkills()
	if len(skills) == 0 {
		ids := config.GetActiveSkillIDs()
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
	maxHP, attack := config.GetMaxHP(), config.GetAttack()
	defense, speed := config.GetDefense(), config.GetSpeed()
	for _, roleEquip := range role.GetEquips() {
		if roleEquip == nil {
			return 0, 0, 0, 0, fmt.Errorf("role id=%s has nil equip", role.GetRoleId())
		}
		equip := glconf.GetBattleEquipCfg(roleEquip.GetEquipConfigId())
		if equip == nil {
			return 0, 0, 0, 0, fmt.Errorf("equip battle config not found: equip_config_id=%d", roleEquip.GetEquipConfigId())
		}
		modifier := equip.GetModifier()
		maxHP += modifier.GetMaxHP()
		attack += modifier.GetAttack()
		defense += modifier.GetDefense()
		speed += modifier.GetSpeed()
	}
	return maxHP, attack, defense, speed, nil
}

func buildMonsterUnit(member glconf.BattleMonsterGroupMemberCfg) (engine.UnitConfig, error) {
	config := glconf.GetBattleMonsterCfg(member.GetMonsterCfgID())
	if config == nil {
		return engine.UnitConfig{}, fmt.Errorf("monster battle config not found: monster_config_id=%d", member.GetMonsterCfgID())
	}
	basic, err := buildSkill(config.GetBasicSkillID())
	if err != nil {
		return engine.UnitConfig{}, err
	}
	active := make([]engine.SkillConfig, 0, len(config.GetActiveSkillIDs()))
	for _, id := range config.GetActiveSkillIDs() {
		skill, err := buildSkill(id)
		if err != nil {
			return engine.UnitConfig{}, err
		}
		active = append(active, skill)
	}
	statuses, err := buildInitialStatuses(config.GetInitialStatusIDs())
	if err != nil {
		return engine.UnitConfig{}, err
	}
	instanceID := "monster-" + strconv.Itoa(int(member.GetMonsterCfgID())) + "-" + strconv.Itoa(int(member.GetPosition()))
	return engine.UnitConfig{InstanceID: instanceID, Team: pb.BattleTeam_BATTLE_TEAM_DEFENDER, Position: member.GetPosition(), MaxHP: config.GetMaxHP(), Attack: config.GetAttack(), Defense: config.GetDefense(), Speed: config.GetSpeed(), BasicSkill: basic, ActiveSkills: active, InitialStatus: statuses}, nil
}

func buildSkill(id int32) (engine.SkillConfig, error) {
	config := glconf.GetBattleSkillCfg(id)
	if config == nil {
		return engine.SkillConfig{}, fmt.Errorf("skill battle config not found: skill_config_id=%d", id)
	}
	effects := make([]engine.EffectConfig, 0, len(config.GetEffects()))
	for _, effect := range config.GetEffects() {
		effects = append(effects, engine.EffectConfig{Kind: engine.EffectKind(effect.GetKind()), CoefficientPermille: effect.GetCoefficientPermille(), Flat: effect.GetFlat(), StatusID: strconv.Itoa(int(effect.GetStatusID()))})
	}
	return engine.SkillConfig{ID: strconv.Itoa(int(config.GetCfgID())), Cooldown: int(config.GetCooldown()), TargetRule: pb.TargetRule(config.GetTargetRule()), Effects: effects}, nil
}

func buildInitialStatuses(ids []int32) ([]engine.StatusConfig, error) {
	statuses := make([]engine.StatusConfig, 0, len(ids))
	for _, id := range ids {
		config := glconf.GetBattleStatusCfg(id)
		if config == nil {
			return nil, fmt.Errorf("battle status config not found: status_config_id=%d", id)
		}
		modifier := config.GetModifier()
		statuses = append(statuses, engine.StatusConfig{
			ID:            strconv.Itoa(int(config.GetCfgID())),
			Kind:          engine.StatusKind(config.GetKind()),
			DurationTurns: int(config.GetDurationTurns()),
			Potency:       config.GetPotency(),
			Modifier: engine.AttributeModifier{
				AttackFlat:  modifier.GetAttack(),
				DefenseFlat: modifier.GetDefense(),
				SpeedFlat:   modifier.GetSpeed(),
			},
		})
	}
	return statuses, nil
}
