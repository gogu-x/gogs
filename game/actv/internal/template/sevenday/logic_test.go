package sevenday

import (
	"github.com/gogu-x/gogs/def"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
	"testing"
	"time"

	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/tree/tlog"
)

func newTestBehavior() *Behavior {
	tlog.NewLog("", 0)
	meta := &service.ActvMeta{ActvID: 100, OwnerID: 7, OwnerScope: service.ScopePlayer}
	b := (&Behavior{}).Create(&service.Actv{Meta: meta}, conf.ActvConfig{}).(*Behavior)
	b.OnInit()
	return b
}

// TestDailyLogin 验证重复登录、断登、在线切日和完成封顶。
func TestDailyLogin(t *testing.T) {
	b := newTestBehavior()
	now := time.Date(2026, 11, 12, 23, 59, 59, 0, time.FixedZone("CST", 8*3600))
	if !b.advance(now, true) || b.GetLoginData().GetLoginDays() != 1 {
		t.Fatal("首次登录应计一天")
	}
	if b.advance(now, true) {
		t.Fatal("同日重复登录不能改变进度")
	}
	b.advance(now.Add(time.Second), true)
	if b.GetLoginData().GetLoginDays() != 2 || b.GetLoginData().GetDailyCount() != 1 {
		t.Fatal("持续在线零点应计新一天")
	}
	b.advance(now.AddDate(0, 0, 4), false)
	if b.GetLoginData().GetLoginDays() != 2 || b.GetLoginData().GetDailyCount() != 0 {
		t.Fatal("断登不能补算或清零累计进度")
	}
	b.advance(now.AddDate(0, 0, 4), true)
	if b.GetLoginData().GetLoginDays() != 3 {
		t.Fatal("断登后上线只计一天")
	}
	for i := 5; i < 15; i++ {
		b.advance(now.AddDate(0, 0, i), true)
	}
	if b.GetLoginData().GetLoginDays() != 7 || !b.packState().GetCompleted() {
		t.Fatal("累计七天后必须保留完成状态")
	}
	if b.advance(now, true) {
		t.Fatal("时钟回拨不能重复累计")
	}
}

// TestRestoreAndCorruptData 验证持久化恢复去重及损坏数据的写保护。
func TestRestoreAndCorruptData(t *testing.T) {
	b := newTestBehavior()
	now := time.Now()
	b.advance(now, true)
	raw, err := b.login.Save()
	if err != nil {
		t.Fatal(err)
	}
	restored := newTestBehavior()
	if err := restored.login.Load(raw); err != nil {
		t.Fatal(err)
	}
	if restored.advance(now, true) || restored.GetLoginData().GetLoginDays() != 1 {
		t.Fatal("重启恢复后同日不能重复记数")
	}
	broken := newTestBehavior()
	if err := broken.login.Load([]byte{1, 2, 3}); err == nil {
		t.Fatal("错误数据应阻断组件")
	}
	if broken.advance(now, true) || broken.login.IsDirty() {
		t.Fatal("不能覆盖损坏存档")
	}
}

// TestCalendarMidnight 验证切日使用当地自然日，不固定加二十四小时。
func TestCalendarMidnight(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 3, 8, 0, 0, 0, 0, loc)
	b := newTestBehavior()
	b.advance(now, true)
	if b.packState().GetNextResetTime()-now.Unix() != 23*3600 {
		t.Fatal("夏令时切日应到下一自然日零点")
	}
}

// TestPushAndBusinessRouting 验证模板直接推送自定义通知，越权请求由自身 ACK 返回错误。
func TestPushAndBusinessRouting(t *testing.T) {
	b := newTestBehavior()
	gate := &sevenDayGate{messages: make(chan *ipb.PushToMsg, 8)}
	pid := tree.SpawnOne(gate)
	t.Cleanup(func() { tree.Stop(pid) })
	b.HandleEvent(gameevent.NewPlayer(gameevent.NameUserLogin, 7, 1))
	select {
	case push := <-gate.messages:
		ntf, ok := push.Msg.(*pb_actv.SevenDayLoginNtf)
		if !ok || push.GetUID() != 7 || ntf.GetData().GetLoginDays() != 1 {
			t.Fatal("首次登录没有推送模板状态")
		}
	case <-time.After(time.Second):
		t.Fatal("未收到主动推送")
	}
	b.HandleRequest(8, &pb_actv.GetSevenDayLoginReq{ActvId: 100})
	select {
	case push := <-gate.messages:
		ack, ok := push.Msg.(*pb_actv.GetSevenDayLoginAck)
		if !ok || push.GetUID() != 8 || ack.GetCode() != pb_common.ErrCode_ACTV_NOT_FOUND {
			t.Fatal("越权查询没有由业务 ACK 拒绝")
		}
	case <-time.After(time.Second):
		t.Fatal("未收到业务 ACK")
	}
}

type sevenDayGate struct{ messages chan *ipb.PushToMsg }

func (*sevenDayGate) Name() string        { return def.GameGate }
func (*sevenDayGate) OnInit(tree.Context) {}
func (*sevenDayGate) OnStop(tree.Context) {}
func (gate *sevenDayGate) HandleMessage(_ tree.Context, message interface{}) {
	if push, ok := message.(*ipb.PushToMsg); ok {
		gate.messages <- push
	}
}

// TestSessionEventsStayInTemplate 验证上下线事件直接进入模板，由模板过滤旧会话。
func TestSessionEventsStayInTemplate(t *testing.T) {
	b := newTestBehavior()
	b.HandleEvent(gameevent.NewPlayer(gameevent.NameUserLogin, 7, 2))
	if !b.online || b.GetLoginData().GetLoginDays() != 1 {
		t.Fatal("登录事件未由模板处理")
	}
	b.HandleEvent(gameevent.NewPlayer(gameevent.NameUserLogout, 7, 1))
	if !b.online {
		t.Fatal("旧会话退出影响了新会话")
	}
	b.HandleEvent(gameevent.NewPlayer(gameevent.NameUserLogout, 7, 2))
	if b.online {
		t.Fatal("匹配会话退出未生效")
	}
	b.HandleEvent(gameevent.NewPlayer(gameevent.NameUserLogin, 7, 1))
	if b.online {
		t.Fatal("过期登录事件恢复了在线状态")
	}
}

// TestCompletedActvInfo 验证七日完成状态通过通用活动信息下发并保留配置编号。
func TestCompletedActvInfo(t *testing.T) {
	b := newTestBehavior()
	b.Meta().CfgID = 70000001
	b.Meta().TemplateType = 7001
	b.GetLoginData().SetLoginDays(7)
	info := b.OnPack()
	if info.GetCfgId() != 70000001 || !info.GetCompleted() || info.GetStatus() != pb_actv.ActvStatus_ENDED {
		t.Fatal("已完成活动信息不完整")
	}
}
