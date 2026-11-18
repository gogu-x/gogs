package actv

import (
	"testing"

	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	"github.com/gogu-x/tree/tlog"
)

// TestVisibleActvList 验证归属合并、跨玩家隔离及实际下发的本服活动。
func TestVisibleActvList(t *testing.T) {
	tlog.NewLog("", 0)
	svc := service.NewActvService(1)
	owners := []struct {
		scope service.OwnerScope
		id    service.OwnerID
	}{
		{service.ScopePlayer, 7}, {service.ScopePlayer, 8},
		{service.ScopeServer, 1}, {service.ScopeServer, 2},
		{service.ScopeGuild, 9}, {service.ScopeGuild, 10},
	}
	for i, owner := range owners {
		behavior := &base.BaseBehavior{}
		a := &service.Actv{Meta: &service.ActvMeta{ActvID: int64(i + 1), OwnerScope: owner.scope, OwnerID: owner.id}, Ext: behavior}
		behavior.Bind(a, conf.ActvConfig{})
		if !svc.AddActv(a) {
			t.Fatal("添加活动失败")
		}
	}
	visible := svc.ListPlayerVisibleActvs(7, 9)
	if len(visible) != 3 {
		t.Fatalf("可见活动数量错误: %d", len(visible))
	}
	for i, id := range []int64{1, 3, 5} {
		if visible[i].GetMeta().GetActvID() != id {
			t.Fatal("包含其他归属活动或顺序错误")
		}
	}
	visible[0] = nil
	if svc.ListActvs(service.ScopePlayer, 7)[0] == nil {
		t.Fatal("修改返回列表破坏归属索引")
	}
	actor := &Actv{actvService: svc}
	infos := actor.actvList(7).GetActvs()
	if len(infos) != 2 || infos[0].GetId() != 1 || infos[1].GetId() != 3 {
		t.Fatal("个人或本服活动未正确下发")
	}
	if len(svc.ListPlayerVisibleActvs(0, 9)) != 0 {
		t.Fatal("无效玩家仍取得活动")
	}
}
