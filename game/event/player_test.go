package event

import (
	"github.com/gogu-x/gogs/pb/sspb"
	"google.golang.org/protobuf/proto"
	"testing"
)

// TestPlayerExtensionRoundTrip 验证通用扩展参数可完整保存 64 位值，克隆不会共享参数。
func TestPlayerExtensionRoundTrip(t *testing.T) {
	const uid = uint64(1<<63 + 12345)
	const generation = ^uint64(0)
	original := NewPlayer(NameUserLogin, uid, generation)
	raw, err := proto.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	restored := new(sspb.Event)
	if err := proto.Unmarshal(raw, restored); err != nil {
		t.Fatal(err)
	}
	if restored.GetEventTyp() != TypePlayerLifecycle || PlayerUID(restored) != uid || PlayerGeneration(restored) != generation {
		t.Fatal("扩展参数精度或事件类型错误")
	}
	if !PlayerEventTime(original).Equal(PlayerEventTime(restored)) {
		t.Fatal("时间参数未保留")
	}
	cloned := Clone(original)
	cloned.ExtArgs[ParamUID] = "1"
	if PlayerUID(original) != uid {
		t.Fatal("克隆共享了扩展参数")
	}
	fields := restored.ProtoReflect().Descriptor().Fields()
	for _, name := range []string{"uid", "generation", "occurred_at"} {
		for i := 0; i < fields.Len(); i++ {
			if string(fields.Get(i).Name()) == name {
				t.Fatalf("仍存在专用顶层字段 %s", name)
			}
		}
	}
}
