package online

import (
	"os"

	"github.com/gogu-x/tree/tlog"
	"testing"

	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
)

func TestClientRequestJoinPreservesUpdateAndRejectsUnimplementedOperations(t *testing.T) {
	behavior := &Behavior{}
	behavior.Bind(nil, conf.ActivityConfig{ID: 1, Target: 80})
	behavior.OnInit()
	if err := behavior.member.Load(7, nil); err != nil {
		t.Fatal(err)
	}
	svc := service.NewActvService(1)
	actv := &service.Actv{Meta: &service.ActvMeta{ActvID: 12, OwnerID: 7, OwnerScope: service.ScopePlayer}, Ext: behavior}
	if !svc.AddActv(actv) {
		t.Fatal("activity setup failed")
	}
	if code := svc.HandleClientRequest(7, 12, &pb_activity.JoinActivityReq{ActivityId: 12}); code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	if got := behavior.member.Get(7).Target; got != 80 {
		t.Fatalf("target=%d", got)
	}
	if code := svc.HandleClientRequest(7, 12, &pb_activity.GetProgressReq{ActivityId: 12}); code != pb_common.ErrCode_ACTIVITY_UNAVAILABLE {
		t.Fatal(code)
	}
	if code := svc.HandleClientRequest(7, 12, &pb_activity.ClaimRewardReq{ActivityId: 12}); code != pb_common.ErrCode_ACTIVITY_UNAVAILABLE {
		t.Fatal(code)
	}
	if behavior.member.Get(7).Rewarded {
		t.Fatal("unimplemented claim changed rewards")
	}
	if code := svc.HandleClientRequest(8, 12, &pb_activity.JoinActivityReq{ActivityId: 12}); code != pb_common.ErrCode_ACTIVITY_NOT_FOUND {
		t.Fatal(code)
	}
	if code := svc.HandleClientRequest(7, 99, &pb_activity.JoinActivityReq{ActivityId: 99}); code != pb_common.ErrCode_ACTIVITY_NOT_FOUND {
		t.Fatal(code)
	}
	actv.Meta.State = service.StateEnded
	if code := svc.HandleClientRequest(7, 12, &pb_activity.JoinActivityReq{ActivityId: 12}); code != pb_common.ErrCode_ACTIVITY_NOT_OPEN {
		t.Fatal(code)
	}
}

func TestJoinWithoutLoadedMemberDoesNotSucceed(t *testing.T) {
	behavior := &Behavior{}
	behavior.OnInit()
	if code := behavior.HandleClientRequest(7, &pb_activity.JoinActivityReq{ActivityId: 1}); code != pb_common.ErrCode_ACTIVITY_UNAVAILABLE {
		t.Fatal(code)
	}
}

func TestMain(m *testing.M) {
	logger, err := tlog.New("error", "", tlog.NoFlags)
	if err != nil {
		panic(err)
	}
	tlog.Log = logger
	os.Exit(m.Run())
}
