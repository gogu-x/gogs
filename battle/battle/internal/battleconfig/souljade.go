package battleconfig

import (
	"fmt"

	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// buildSoulJadeCharges 将 Game 权威提供的魂玉词条 ID 编译成当前角色的积攒效果。
func buildSoulJadeCharges(role *pb.Role) ([]engine.ChargeEffectConfig, error) {
	ids := role.GetSoulJadeEffectAffixIds()
	charges := make([]engine.ChargeEffectConfig, 0, len(ids))
	seen := make(map[pb.EffectType]bool, len(ids))
	for _, id := range ids {
		config := glconf.GetSoulJadeEffectAffixCfgNoErr(id)
		if !glconf.ValidSoulJadeEffectAffixCfg(config) {
			return nil, fmt.Errorf("role id=%s has invalid soul jade effect affix id=%d", role.GetRoleId(), id)
		}
		kind := config.GetEffectType()
		if seen[kind] {
			return nil, fmt.Errorf("role id=%s has duplicate soul jade effect type=%d", role.GetRoleId(), kind)
		}
		seen[kind] = true
		charges = append(charges, engine.ChargeEffectConfig{
			Kind: kind, Threshold: int64(config.GetTriggerThreshold()), Gain: int64(config.GetGainPerHit()),
			Trigger: true, TargetRule: config.GetTargetRule(),
			TriggerEffect: engine.EffectConfig{Kind: kind, CoefficientPermille: int64(config.GetDamageCoefficientPermille())},
		})
	}
	return charges, nil
}
