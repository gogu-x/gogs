package souljade

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"sort"

	"github.com/gogu-x/gogs/glconf"
)

type refineCandidate struct {
	id     int32
	weight int64
	effect bool
}

// refinePool 合并基础属性和配置有效的效果词条，基础属性权重为 1。
func refinePool(level *glconf.SoulJadeLevelCfg) ([]refineCandidate, bool) {
	if !validLevel(level) {
		return nil, false
	}
	pool := make([]refineCandidate, 0)
	for _, id := range level.GetAllowedAttributes() {
		pool = append(pool, refineCandidate{id: id, weight: 1})
	}
	effects := glconf.GetAllSoulJadeEffectAffixCfg()
	sort.Slice(effects, func(i, j int) bool { return effects[i].GetCfgID() < effects[j].GetCfgID() })
	for _, effect := range effects {
		if !glconf.ValidSoulJadeEffectAffixCfg(effect) {
			return nil, false
		}
		pool = append(pool, refineCandidate{id: effect.GetCfgID(), weight: int64(effect.GetRollWeight()), effect: true})
	}
	return pool, len(pool) >= int(level.GetAffixCount())
}

// rollRefineAffixes 按权重无放回抽取，基础属性值在配置区间内均匀随机。
func rollRefineAffixes(pool []refineCandidate, level *glconf.SoulJadeLevelCfg, source io.Reader) ([]*Affix, error) {
	candidates := append([]refineCandidate(nil), pool...)
	count := int(level.GetAffixCount())
	if count < 1 || count > len(candidates) {
		return nil, fmt.Errorf("invalid affix count")
	}
	result := make([]*Affix, 0, count)
	for len(result) < count {
		var total int64
		for _, candidate := range candidates {
			total += candidate.weight
		}
		if total <= 0 {
			return nil, fmt.Errorf("invalid affix weight")
		}
		draw, err := rand.Int(source, big.NewInt(total))
		if err != nil {
			return nil, err
		}
		choice := draw.Int64()
		selected := 0
		for i, candidate := range candidates {
			if choice < candidate.weight {
				selected = i
				break
			}
			choice -= candidate.weight
		}
		candidate := candidates[selected]
		value := int32(0)
		if !candidate.effect {
			minimum, maximum := level.GetRefineMinPermille(), level.GetRefineMaxPermille()
			if minimum <= 0 || maximum < minimum {
				return nil, fmt.Errorf("invalid affix range")
			}
			roll, err := rand.Int(source, big.NewInt(int64(maximum)-int64(minimum)+1))
			if err != nil {
				return nil, err
			}
			value = minimum + int32(roll.Int64())
		}
		result = append(result, &Affix{Attribute: candidate.id, Permille: value})
		candidates = append(candidates[:selected], candidates[selected+1:]...)
	}
	return result, nil
}
