package glconf

import pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"

// GetCfgID 返回魂玉效果词条 ID。
func (cfg *SoulJadeEffectAffixCfg) GetCfgID() int32 {
	if cfg == nil {
		return 0
	}
	return cfg.CfgID
}

// GetEffectType 返回战斗效果协议类型。
func (cfg *SoulJadeEffectAffixCfg) GetEffectType() pb.EffectType {
	if cfg == nil {
		return pb.EffectType_EFFECT_TYPE_UNSPECIFIED
	}
	return pb.EffectType(cfg.EffectType)
}

// GetRollWeight 返回洗练抽取权重。
func (cfg *SoulJadeEffectAffixCfg) GetRollWeight() int32 {
	if cfg == nil {
		return 0
	}
	return cfg.RollWeight
}

// GetTriggerThreshold 返回触发所需积攒值。
func (cfg *SoulJadeEffectAffixCfg) GetTriggerThreshold() int32 {
	if cfg == nil {
		return 0
	}
	return cfg.TriggerThreshold
}

// GetGainPerHit 返回每次有效命中增加的积攒值。
func (cfg *SoulJadeEffectAffixCfg) GetGainPerHit() int32 {
	if cfg == nil {
		return 0
	}
	return cfg.GainPerHit
}

// GetDamageCoefficientPermille 返回触发伤害系数。
func (cfg *SoulJadeEffectAffixCfg) GetDamageCoefficientPermille() int32 {
	if cfg == nil {
		return 0
	}
	return cfg.DamageCoefficientPermille
}

// GetTargetRule 返回效果目标规则。
func (cfg *SoulJadeEffectAffixCfg) GetTargetRule() pb.TargetRule {
	if cfg == nil {
		return pb.TargetRule_TARGET_RULE_UNSPECIFIED
	}
	return pb.TargetRule(cfg.TargetRule)
}

// ValidSoulJadeEffectAffixCfg 检查洗练词条和战斗效果配置是否可用。
func ValidSoulJadeEffectAffixCfg(cfg *SoulJadeEffectAffixCfg) bool {
	if cfg == nil || cfg.GetCfgID() <= 4 || cfg.GetRollWeight() <= 0 || cfg.GetTriggerThreshold() <= 0 || cfg.GetGainPerHit() <= 0 || cfg.GetDamageCoefficientPermille() <= 0 {
		return false
	}
	switch cfg.GetEffectType() {
	case pb.EffectType_EFFECT_TYPE_ICE_BURST:
	case pb.EffectType_EFFECT_TYPE_THUNDER:
	default:
		return false
	}
	return cfg.GetTargetRule() == pb.TargetRule_TARGET_RULE_ENEMY_ALL || cfg.GetTargetRule() == pb.TargetRule_TARGET_RULE_ENEMY_SINGLE
}
