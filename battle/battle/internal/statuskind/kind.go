// Package statuskind 定义稳定的战斗状态类型标识。
package statuskind

// Kind 表示已施加状态所代表的行为类型。
type Kind uint8

const (
	// Stun 表示阻止目标执行行动的眩晕状态。
	Stun Kind = iota + 1
	// Silence 表示阻止目标选择主动技能的沉默状态。
	Silence
	// DOT 表示持续期间周期性造成伤害的状态。
	DOT
	// HOT 表示持续期间周期性恢复生命值的状态。
	HOT
	// AttributeModifier 表示持续期间修正一个或多个单位属性的状态。
	AttributeModifier
)
