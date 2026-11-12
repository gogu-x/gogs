package actor

import (
	"github.com/gogu-x/gogs/game/actv/internal/service"
	templates "github.com/gogu-x/gogs/game/actv/internal/template"
	"github.com/gogu-x/gogs/game/actv/internal/trigger"
	"github.com/gogu-x/gogs/glconf"
	"testing"
	"time"
)

// TestInitializeOnceWithoutCreationEvent 验证初始化不依赖创建事件，历史记录与完成状态均阻止重建。
func TestInitializeOnceWithoutCreationEvent(t *testing.T) {
	actor := NewActv()
	templates.Register(actor.actvService)
	actor.creationTrigger = trigger.NewEngine(
		[]*glconf.ActvOnlineCfg{{ID: 1, Type: 7001, Available: true}}, actor.actvTemplateScope, 0, time.Local,
	)
	actor.actvService.SetPlayerLoaded(7)
	actor.initializePlayerActvs(7, &playerLoadState{})
	acts := actor.actvService.ListActvs(service.ScopePlayer, 7)
	if len(acts) != 1 {
		t.Fatal("无角色创建事件也应创建缺少的活动")
	}
	acts[0].GetMeta().State = service.StateEnded
	actor.initializePlayerActvs(7, &playerLoadState{})
	if len(actor.actvService.ListActvs(service.ScopePlayer, 7)) != 1 {
		t.Fatal("已完成活动被重建")
	}
	actor.actvService.SetPlayerLoaded(8)
	metas := []service.ActvMeta{{CfgID: 1, OwnerID: 8, OwnerScope: service.ScopePlayer}}
	actor.initializePlayerActvs(8, &playerLoadState{metas: &metas})
	if len(actor.actvService.ListActvs(service.ScopePlayer, 8)) != 0 {
		t.Fatal("未恢复行为的历史记录被重复创建")
	}
}
