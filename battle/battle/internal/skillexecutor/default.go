package skillexecutor

import "github.com/gogu-x/gogs/battle/battle/internal/engine"

type Default struct{}

func (Default) Execute(context engine.SkillExecutionContext) ([]engine.EffectResolution, error) {
	if context.Effects == nil {
		return nil, nil
	}
	results := make([]engine.EffectResolution, 0, len(context.Skill.Effects))
	for _, effect := range context.Skill.Effects {
		result, err := context.Effects.Resolve(engine.EffectContext{
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
