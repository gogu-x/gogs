package service_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree/tlog"
)

type eventObserver struct {
	base.BaseBehavior
	count int
}

func (b *eventObserver) HandleEvent(*sspb.Event) { b.count++ }

// TestEventOwnerRouting 验证唯一分发入口按类型和归属隔离，过滤结束、过期及未知类型。
func TestEventOwnerRouting(t *testing.T) {
	tlog.NewLog("", 0)
	svc := service.NewActvService(1)
	scopes := []service.OwnerScope{service.ScopeServer, service.ScopeServer, service.ScopePlayer, service.ScopePlayer, service.ScopeGuild, service.ScopeGuild, service.ScopePlayer, service.ScopePlayer}
	owners := []service.OwnerID{1, 2, 7, 8, 10, 11, 7, 7}
	observers := make([]*eventObserver, len(scopes))
	for i, scope := range scopes {
		observer := &eventObserver{}
		observers[i] = observer
		meta := &service.ActvMeta{ActvID: int64(i + 1), OwnerScope: scope, OwnerID: owners[i]}
		if i == 6 {
			meta.State = service.StateEnded
		}
		if i == 7 {
			meta.EndTs = time.Now().Add(-15 * 24 * time.Hour).Unix()
		}
		if !svc.AddActv(&service.Actv{Meta: meta, Ext: observer}) {
			t.Fatal("实例创建失败")
		}
	}
	svc.SetPlayerLoaded(7)
	svc.SetPlayerLoaded(8)
	svc.HandleEvent(gameevent.NewServerOpened())
	svc.HandleEvent(gameevent.NewPlayer(gameevent.NameUserLogin, 7, 1))
	svc.HandleEvent(&sspb.Event{EventTyp: gameevent.TypeGuild, ExtArgs: map[string]string{gameevent.ParamGuildID: strconv.Itoa(10)}})
	svc.HandleEvent(&sspb.Event{EventTyp: 99})
	svc.HandleEvent(gameevent.NewPlayer(gameevent.NameUserLogin, 0, 1))
	svc.HandleEvent(&sspb.Event{EventTyp: gameevent.TypeGuild})
	want := []int{1, 0, 1, 0, 1, 0, 0, 0}
	for i, observer := range observers {
		if observer.count != want[i] {
			t.Fatalf("归属分发错误 index=%d got=%d want=%d", i, observer.count, want[i])
		}
	}
}
