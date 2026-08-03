package internal

import (
	"testing"

	"github.com/gogu-x/gogs/game/play/internal/base"
	"github.com/gogu-x/gogs/pb/protoActivity"
	"github.com/gogu-x/gogs/pb/protoChat"
	"github.com/gogu-x/gogs/pb/protoGateway"
)

// TestInitRoutes 路由表快照：确认每条消息注册到位，且登录白名单只包含预期消息。
func TestInitRoutes(t *testing.T) {
	app := base.NewApp()
	InitRoutes(app)

	cases := []struct {
		msg  interface{}
		anon bool
	}{
		{&protoGateway.LoginReq{}, true},
		{&protoGateway.RegisterReq{}, true},
		{&protoChat.ChatReq{}, false},
		{&protoActivity.GetActivityListReq{}, false},
		{&protoActivity.JoinActivityReq{}, false},
		{&protoActivity.GetProgressReq{}, false},
		{&protoActivity.ClaimRewardReq{}, false},
	}

	for _, c := range cases {
		anon, ok := app.Dispatcher().IsAnonymous(c.msg)
		if !ok {
			t.Errorf("%T not registered", c.msg)
			continue
		}
		if anon != c.anon {
			t.Errorf("%T anonymous = %v, want %v", c.msg, anon, c.anon)
		}
	}

	if got := app.Dispatcher().PlayerRouteCount(); got != len(cases) {
		t.Errorf("PlayerRouteCount = %d, want %d", got, len(cases))
	}
}
