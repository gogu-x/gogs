// Package strategy 负责机器人施法条件判断和动作选择。
package strategy

import (
	"fmt"
	"math/bits"
	"strconv"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	common "github.com/gogu-x/gogs/pb/cspb/pb_common"
)

// Rule 是开战时编译的单条优先施法规则。
type Rule struct {
	condition string
	skill     string
	value     int64
}

// Compile 保留配置顺序，开战时校验条件范围和可用技能。
func Compile(source []*common.TypIDVal, skills map[string]bool) ([]Rule, error) {
	var rules []Rule
	for _, entry := range source {
		id := strconv.FormatInt(int64(entry.GetId()), 10)
		kind, value := entry.GetTyp(), entry.GetVal()
		if !skills[id] {
			return nil, fmt.Errorf("施法规则引用未携带技能 %s", id)
		}
		valid := false
		switch kind {
		case "always":
			valid = value == 0
		case "self_hp",
			"ally_hp",
			"enemy_hp":
			valid = value >= 0 && value <= 1000
		case "enemy_count":
			valid = value > 0
		}
		if !valid {
			return nil, fmt.Errorf("技能 %s 的施法条件无效：%s=%d", id, kind, value)
		}
		rules = append(rules, Rule{condition: kind, skill: id, value: value})
	}
	return rules, nil
}

// compareProducts 比较非负整数乘积，避免大血量的百分比交叉相乘溢出。
func compareProducts(a, b, c, d int64) int {
	ah, al := bits.Mul64(uint64(a), uint64(b))
	ch, cl := bits.Mul64(uint64(c), uint64(d))
	if ah < ch || (ah == ch && al < cl) {
		return -1
	}
	if ah == ch && al == cl {
		return 0
	}
	return 1
}

// lower 比较生命比例并选出生命比例较低的单位。
func lower(current, candidate *pb.BattleUnitResult) *pb.BattleUnitResult {
	if current == nil {
		return candidate
	}
	value := compareProducts(candidate.GetHp(), current.GetMaxHp(), current.GetHp(), candidate.GetMaxHp())
	if value < 0 || (value == 0 && candidate.GetInstanceId() < current.GetInstanceId()) {
		return candidate
	}
	return current
}

// below 判断单位当前生命比例是否低于或等于指定千分比。
func below(unit *pb.BattleUnitResult, threshold int64) bool {
	return unit != nil && compareProducts(unit.GetHp(), 1000, unit.GetMaxHp(), threshold) <= 0
}

// Select 按优先级选择首个满足条件且可以施放的技能。返回空目标时按技能配置选敌。
func Select(control *pb.BattleControl, rules []Rule, units []*pb.BattleUnitResult,
	available map[string]pb.TargetRule) (string, string) {
	var self, ally, enemy *pb.BattleUnitResult
	for _, unit := range units {
		if unit.GetInstanceId() == control.GetUnitId() {
			self = unit
			break
		}
	}
	if self == nil || !self.GetAlive() {
		return "", ""
	}
	var enemies int64
	for _, unit := range units {
		if !unit.GetAlive() || unit.GetMaxHp() <= 0 {
			continue
		}
		if unit.GetTeam() == self.GetTeam() {
			ally = lower(ally, unit)
		} else {
			enemy = lower(enemy, unit)
			enemies++
		}
	}
	for _, rule := range rules {
		target, ok := available[rule.skill]
		if !ok {
			continue
		}
		switch rule.condition {
		case "self_hp":
			if !below(self, rule.value) {
				continue
			}
		case "ally_hp":
			if !below(ally, rule.value) {
				continue
			}
		case "enemy_count":
			if enemies < rule.value {
				continue
			}
		case "enemy_hp":
			if !below(enemy, rule.value) {
				continue
			}
			if target == pb.TargetRule_TARGET_RULE_ENEMY_SINGLE {
				return rule.skill, enemy.GetInstanceId()
			}
		}
		return rule.skill, ""
	}
	return control.GetBasicSkill(), ""
}
