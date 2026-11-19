package souljade

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"slices"

	"github.com/gogu-x/gogs/glconf"
)

// rollRefineAffixes 按权重无放回抽取，基础属性值在配置区间内均匀随机。
func rollRefineAffixes(pool []glconf.SoulJadeRefineCandidate, level *glconf.SoulJadeLevelCfg, source io.Reader) ([]*Affix, error) {
	candidates := slices.Clone(pool)
	count := int(level.GetAffixCount())
	if count < 1 || count > len(candidates) {
		return nil, fmt.Errorf("invalid affix count")
	}
	result := make([]*Affix, 0, count)
	for len(result) < count {
		var total int64
		for _, candidate := range candidates {
			total += candidate.GetWeight()
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
			if choice < candidate.GetWeight() {
				selected = i
				break
			}
			choice -= candidate.GetWeight()
		}
		candidate := candidates[selected]
		value := int32(0)
		if !candidate.GetEffect() {
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
		result = append(result, &Affix{Attribute: candidate.GetID(), Permille: value})
		candidates = append(candidates[:selected], candidates[selected+1:]...)
	}
	return result, nil
}
