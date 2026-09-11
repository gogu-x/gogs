package glconf

// GetCfgID 返回角色配置 ID。
func (c *BattleRoleCfg) GetCfgID() int32 {
	if c == nil {
		return 0
	}
	return c.CfgID
}

// GetMaxHP 返回角色基础最大生命。
func (c *BattleRoleCfg) GetMaxHP() int64 {
	if c == nil {
		return 0
	}
	return c.MaxHP
}

// GetAttack 返回角色基础攻击。
func (c *BattleRoleCfg) GetAttack() int64 {
	if c == nil {
		return 0
	}
	return c.Attack
}

// GetDefense 返回角色基础防御。
func (c *BattleRoleCfg) GetDefense() int64 {
	if c == nil {
		return 0
	}
	return c.Defense
}

// GetSpeed 返回角色基础速度。
func (c *BattleRoleCfg) GetSpeed() int64 {
	if c == nil {
		return 0
	}
	return c.Speed
}

// GetDefaultPosition 返回角色默认站位。
func (c *BattleRoleCfg) GetDefaultPosition() int32 {
	if c == nil {
		return 0
	}
	return c.DefaultPosition
}

// GetBasicSkillID 返回角色普攻技能 ID。
func (c *BattleRoleCfg) GetBasicSkillID() int32 {
	if c == nil {
		return 0
	}
	return c.BasicSkillID
}

// GetActiveSkillIDs 返回角色主动技能 ID。
func (c *BattleRoleCfg) GetActiveSkillIDs() []int32 {
	if c == nil {
		return nil
	}
	return c.ActiveSkillIDs
}

// GetCfgID 返回技能配置 ID。
func (c *BattleSkillCfg) GetCfgID() int32 {
	if c == nil {
		return 0
	}
	return c.CfgID
}

// GetCfgID 返回装备配置 ID。
func (c *BattleEquipCfg) GetCfgID() int32 {
	if c == nil {
		return 0
	}
	return c.CfgID
}

// GetModifier 返回装备属性修正。
func (c *BattleEquipCfg) GetModifier() BattleAttributeModifierCfg {
	if c == nil {
		return BattleAttributeModifierCfg{}
	}
	return c.Modifier
}

// GetCfgID 返回怪物配置 ID。
func (c *BattleMonsterCfg) GetCfgID() int32 {
	if c == nil {
		return 0
	}
	return c.CfgID
}

// GetCfgID 返回怪物组配置 ID。
func (c *BattleMonsterGroupCfg) GetCfgID() int32 {
	if c == nil {
		return 0
	}
	return c.CfgID
}

// GetMembers 返回怪物组成员。
func (c *BattleMonsterGroupCfg) GetMembers() []BattleMonsterGroupMemberCfg {
	if c == nil {
		return nil
	}
	return c.Members
}

// GetMonsterCfgID 返回成员怪物配置 ID。
func (c *BattleMonsterGroupMemberCfg) GetMonsterCfgID() int32 { return c.MonsterCfgID }

// GetPosition 返回成员配置站位。
func (c *BattleMonsterGroupMemberCfg) GetPosition() int32 { return c.Position }

// GetBattleType 返回规则对应的战斗类型。
func (c *BattleRuleCfg) GetBattleType() int32 {
	if c == nil {
		return 0
	}
	return c.BattleType
}

// GetATBThreshold 返回行动条阈值。
func (c *BattleRuleCfg) GetATBThreshold() int64 {
	if c == nil {
		return 0
	}
	return c.ATBThreshold
}

// GetMaxActions 返回最大行动次数。
func (c *BattleRuleCfg) GetMaxActions() int32 {
	if c == nil {
		return 0
	}
	return c.MaxActions
}

// GetCooldown 返回技能冷却次数。
func (c *BattleSkillCfg) GetCooldown() int32 {
	if c == nil {
		return 0
	}
	return c.Cooldown
}

// GetTargetRule 返回技能目标规则。
func (c *BattleSkillCfg) GetTargetRule() int32 {
	if c == nil {
		return 0
	}
	return c.TargetRule
}

// GetEffects 返回技能顺序效果。
func (c *BattleSkillCfg) GetEffects() []BattleEffectCfg {
	if c == nil {
		return nil
	}
	return c.Effects
}

// GetKind 返回效果类型。
func (c *BattleEffectCfg) GetKind() int32 { return c.Kind }

// GetCoefficientPermille 返回效果倍率。
func (c *BattleEffectCfg) GetCoefficientPermille() int64 { return c.CoefficientPermille }

// GetFlat 返回效果固定值。
func (c *BattleEffectCfg) GetFlat() int64 { return c.Flat }

// GetStatusID 返回效果状态 ID。
func (c *BattleEffectCfg) GetStatusID() int32 { return c.StatusID }

// GetMaxHP 返回怪物基础最大生命。
func (c *BattleMonsterCfg) GetMaxHP() int64 {
	if c == nil {
		return 0
	}
	return c.MaxHP
}

// GetAttack 返回怪物基础攻击。
func (c *BattleMonsterCfg) GetAttack() int64 {
	if c == nil {
		return 0
	}
	return c.Attack
}

// GetDefense 返回怪物基础防御。
func (c *BattleMonsterCfg) GetDefense() int64 {
	if c == nil {
		return 0
	}
	return c.Defense
}

// GetSpeed 返回怪物基础速度。
func (c *BattleMonsterCfg) GetSpeed() int64 {
	if c == nil {
		return 0
	}
	return c.Speed
}

// GetBasicSkillID 返回怪物普攻技能 ID。
func (c *BattleMonsterCfg) GetBasicSkillID() int32 {
	if c == nil {
		return 0
	}
	return c.BasicSkillID
}

// GetActiveSkillIDs 返回怪物主动技能 ID。
func (c *BattleMonsterCfg) GetActiveSkillIDs() []int32 {
	if c == nil {
		return nil
	}
	return c.ActiveSkillIDs
}

// GetMaxHP 返回属性修正最大生命。
func (c *BattleAttributeModifierCfg) GetMaxHP() int64 { return c.MaxHP }

// GetAttack 返回属性修正攻击。
func (c *BattleAttributeModifierCfg) GetAttack() int64 { return c.Attack }

// GetDefense 返回属性修正防御。
func (c *BattleAttributeModifierCfg) GetDefense() int64 { return c.Defense }

// GetSpeed 返回属性修正速度。
func (c *BattleAttributeModifierCfg) GetSpeed() int64 { return c.Speed }

// GetDamageVariancePermille 返回伤害浮动。
func (c *BattleRuleCfg) GetDamageVariancePermille() int64 {
	if c == nil {
		return 0
	}
	return c.DamageVariancePermille
}

// GetCritChancePermille 返回暴击概率。
func (c *BattleRuleCfg) GetCritChancePermille() int64 {
	if c == nil {
		return 0
	}
	return c.CritChancePermille
}

// GetCritMultiplierPermille 返回暴击倍率。
func (c *BattleRuleCfg) GetCritMultiplierPermille() int64 {
	if c == nil {
		return 0
	}
	return c.CritMultiplierPermille
}

// GetInitialStatusIDs 返回角色初始状态 ID。
func (c *BattleRoleCfg) GetInitialStatusIDs() []int32 {
	if c == nil {
		return nil
	}
	return c.InitialStatusIDs
}

// GetInitialStatusIDs 返回怪物初始状态 ID。
func (c *BattleMonsterCfg) GetInitialStatusIDs() []int32 {
	if c == nil {
		return nil
	}
	return c.InitialStatusIDs
}

// GetCfgID 返回状态配置 ID。
func (c *BattleStatusCfg) GetCfgID() int32 {
	if c == nil {
		return 0
	}
	return c.CfgID
}

// GetKind 返回状态类型。
func (c *BattleStatusCfg) GetKind() int32 {
	if c == nil {
		return 0
	}
	return c.Kind
}

// GetDurationTurns 返回状态持续行动次数。
func (c *BattleStatusCfg) GetDurationTurns() int32 {
	if c == nil {
		return 0
	}
	return c.DurationTurns
}

// GetPotency 返回状态数值。
func (c *BattleStatusCfg) GetPotency() int64 {
	if c == nil {
		return 0
	}
	return c.Potency
}

// GetModifier 返回状态属性修正。
func (c *BattleStatusCfg) GetModifier() BattleAttributeModifierCfg {
	if c == nil {
		return BattleAttributeModifierCfg{}
	}
	return c.Modifier
}
