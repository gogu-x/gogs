package engine

// DefaultSkillExecutor 按配置顺序解析技能对单个目标的普通效果。
type DefaultSkillExecutor struct{}

// Execute 依次解析效果；战斗引擎负责应用结果和生成事件。
func (DefaultSkillExecutor) Execute(context SkillExecutionContext) ([]EffectResolution, error) {
	if context.Effects == nil {
		return nil, nil
	}
	results := make([]EffectResolution, 0, len(context.Skill.Effects))
	for _, effect := range context.Skill.Effects {
		result, err := context.Effects.Resolve(EffectContext{
			Actor: context.Actor, Target: context.Target, Skill: context.Skill,
			Effect: effect, Rules: context.Rules, Damage: context.Damage, RNG: context.RNG,
		})
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}
