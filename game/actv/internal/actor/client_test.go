package actor

import (
	"os"

	"github.com/gogu-x/tree/tlog"
	"testing"

	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
)

func TestActivityListExplicitSuccessAndJoinNotFound(t *testing.T) {
	actor := &Activity{actvService: service.NewActvService(1)}
	value, err := actor.handlePlayerActivityMessage(&ipb.PlayerActivityMessage{UID: 7, Message: &pb_activity.GetActivityListReq{}})
	ack, valid := value.(*pb_activity.GetActivityListAck)
	if err != nil || !valid || ack.GetCode() != pb_common.ErrCode_OK {
		t.Fatalf("list=%v err=%v", value, err)
	}
	value, err = actor.handlePlayerActivityMessage(&ipb.PlayerActivityMessage{UID: 7, ActvID: 99, Message: &pb_activity.JoinActivityReq{ActivityId: 99}})
	join, valid := value.(*pb_activity.JoinActivityAck)
	if err != nil || !valid || join.GetCode() != pb_common.ErrCode_ACTIVITY_NOT_FOUND {
		t.Fatalf("join=%v err=%v", value, err)
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
