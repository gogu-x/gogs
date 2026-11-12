package base_test

import (
	"testing"
	"time"

	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"google.golang.org/protobuf/proto"
)

// TestPackActvInfo 验证协议携带配置、归属及时间信息，且字段从一开始连续编号。
func TestPackActvInfo(t *testing.T) {
	now := time.Now().Unix()
	cases := []struct {
		start, end int64
		state      service.ActvState
		want       pb_actv.ActvStatus
	}{
		{now - 100, 0, service.StateRunning, pb_actv.ActvStatus_ONGOING},
		{now + 100, now + 200, service.StateRunning, pb_actv.ActvStatus_NOT_STARTED},
		{now - 200, now - 100, service.StateRunning, pb_actv.ActvStatus_ENDED},
		{now - 100, 0, service.StateEnded, pb_actv.ActvStatus_ENDED},
	}
	for _, tc := range cases {
		behavior := &base.BaseBehavior{}
		behavior.Bind(&service.Actv{Meta: &service.ActvMeta{
			ActvID: 100, CfgID: 70000001, TemplateType: 7001, OwnerID: 7, OwnerScope: service.ScopePlayer,
			StartTs: tc.start, EndTs: tc.end, State: tc.state,
		}}, conf.ActvConfig{})
		info := behavior.OnPack()
		if info.GetId() != 100 || info.GetCfgId() != 70000001 || info.GetTemplateType() != 7001 || info.GetOwnerId() != 7 || info.GetOwnerScope() != pb_actv.ActvOwnerScope_ACTV_OWNER_PLAYER {
			t.Fatal("活动元数据不完整")
		}
		if info.GetStatus() != tc.want || info.GetStartTime() != tc.start || info.GetEndTime() != tc.end {
			t.Fatal("状态或时间不正确")
		}
		descriptor := info.ProtoReflect().Descriptor()
		if descriptor.Fields().ByName("name") != nil || descriptor.ReservedRanges().Len() != 0 || descriptor.ReservedNames().Len() != 0 {
			t.Fatal("名称字段或兼容保留声明未移除")
		}
		for i := 0; i < descriptor.Fields().Len(); i++ {
			if int(descriptor.Fields().Get(i).Number()) != i+1 {
				t.Fatal("活动信息字段编号不连续")
			}
		}

		encoded, err := proto.Marshal(info)
		if err != nil {
			t.Fatal(err)
		}
		decoded := new(pb_actv.ActvInfo)
		if err := proto.Unmarshal(encoded, decoded); err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(info, decoded) {
			t.Fatal("活动信息序列化丢失字段")
		}
	}
}
