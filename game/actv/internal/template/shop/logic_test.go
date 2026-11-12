package shop

import (
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/testfixture"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/tree/tlog"
	"testing"
	"time"
)

// TestIndependentPersonalShop 验证个人隔离、投递失败、等待回调和每日限购。
func TestIndependentPersonalShop(t *testing.T) {
	tlog.NewLog("", 0)
	testfixture.Load(t)
	svc := service.NewActvService(1)
	svc.RegTpl(7003, &Shop{}, service.ScopePlayer)
	svc.SetPlayerLoaded(7)
	now := time.Now()
	meta := &service.ActvMeta{ActvID: 101, OwnerID: 7, OwnerScope: service.ScopePlayer, CfgID: 1000004, TemplateType: 7003, StartTs: now.Unix() - 1, EndTs: now.Unix() + 86400, ParentActvID: 100}
	a := svc.NewActv(meta, service.NewActvConfig(glconf.GetActvOnlineCfgNoErr(1000004), meta))
	if !svc.AddActv(a) {
		t.Fatal("create")
	}
	b := a.GetExt().(*Shop)
	req := &pb_actv.BuyActvShopReq{ActvId: 101, GoodsId: 10005003, Count: 2, Sequence: 1}
	b.HandleRequest(8, req)
	if b.buying {
		t.Fatal("跨玩家请求创建交易")
	}
	b.HandleRequest(7, req)
	if b.buying || b.data.Get(7).GetSequence() != 0 {
		t.Fatal("投递失败仍保留请求状态")
	}
	d := b.data.Get(7)
	d.GetBought()[10005003] = 2
	b.buying = true
	b.HandleRequest(7, req)
	if d.GetSequence() != 0 || d.GetBought()[10005003] != 2 {
		t.Fatal("等待回调期间修改购买状态")
	}
	b.buying = false
	b.refresh(7, now.AddDate(0, 0, 1))
	if d.GetBought()[10005003] != 0 {
		t.Fatal("每日限购未重置")
	}
	raw, err := b.data.Save()
	if err != nil {
		t.Fatal(err)
	}
	if err := b.data.Load(raw); err != nil {
		t.Fatal(err)
	}
	if b.data.Get(7).GetSequence() != 0 {
		t.Fatal("序号未恢复")
	}
}
