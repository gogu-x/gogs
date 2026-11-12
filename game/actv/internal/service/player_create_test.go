package service_test

import (
	"testing"
	"time"

	"github.com/gogu-x/gogs/game/actv/internal/service"
	templates "github.com/gogu-x/gogs/game/actv/internal/template"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/tree/tlog"
)

// TestPlayerCreationConfig 验证创建触发器、配置关闭、玩家隔离、完成保留和未加载保护。
func TestPlayerCreationConfig(t *testing.T) {
	tlog.NewLog("", 0)
	svc := service.NewActvService(1)
	templates.Register(svc)
	rows := []*glconf.ActvOnlineCfg{
		{ID: 1, Type: 7001, Available: true, Reopen: false},
		{ID: 2, Type: 7001, Available: true, Reopen: true},
		{ID: 3, Type: 7001, Available: false, Reopen: false},
		{ID: 4, Type: 7001, Available: true, Reopen: true},
	}
	create := func(uid uint64) []*service.Actv {
		return svc.CreatePlayerActvs(
			uid, rows, nil, time.Now().Unix(),
		)
	}
	if len(create(7)) != 0 {
		t.Fatal("不能在存档加载前创建")
	}
	svc.SetPlayerLoaded(7)
	acts := create(7)
	if len(acts) != 1 || acts[0].GetMeta().GetCfgID() != 1 {
		t.Fatal("只允许匹配且启用的配置")
	}
	acts[0].GetMeta().State = service.StateEnded
	if len(create(7)) != 0 {
		t.Fatal("完成实例不能重新创建")
	}
	svc.SetPlayerLoaded(9)
	if len(svc.CreatePlayerActvs(
		9, rows, map[int32]bool{1: true}, time.Now().Unix(),
	)) != 0 {
		t.Fatal("历史实例不得重复创建")
	}
	svc.SetPlayerLoaded(8)
	if other := create(8); len(other) != 1 || other[0].GetMeta().GetActvID() == acts[0].GetMeta().GetActvID() {
		t.Fatal("不同玩家必须有独立实例")
	}
}
