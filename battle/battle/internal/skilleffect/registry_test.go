package skilleffect

import (
	"reflect"
	"testing"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

func TestEffectsHaveSeparatePackages(t *testing.T) {
	registry := Default()
	prefix := "github.com/gogu-x/gogs/battle/battle/internal/skilleffect/"
	for kind, suffix := range map[pb.EffectType]string{
		pb.EffectType_EFFECT_TYPE_DAMAGE: "damage", pb.EffectType_EFFECT_TYPE_HEAL: "heal", pb.EffectType_EFFECT_TYPE_APPLY_STATUS: "status",
		pb.EffectType_EFFECT_TYPE_ICE_BURST: "iceburst", pb.EffectType_EFFECT_TYPE_THUNDER: "thunder",
	} {
		if handler := registry[kind]; handler == nil {
			t.Fatalf("效果 %d 未注册处理器", kind)
		} else if got := reflect.TypeOf(handler).PkgPath(); got != prefix+suffix {
			t.Errorf("效果 %d 应由 %s 包处理，实际为 %s", kind, suffix, got)
		}
	}
}
