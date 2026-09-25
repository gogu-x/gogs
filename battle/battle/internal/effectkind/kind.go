// Package effectkind 定义稳定的战斗效果类型标识。
package effectkind

// Kind 表示技能效果配置所指定的操作类型。
type Kind uint8

const (
	// Damage 表示扣除目标生命值的伤害效果。
	Damage Kind = iota + 1
	// Heal 表示恢复目标生命值且不超过生命上限的治疗效果。
	Heal
	// ApplyStatus 表示向目标施加已解析状态的效果。
	ApplyStatus
)
