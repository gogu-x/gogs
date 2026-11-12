package actor

import (
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	"github.com/gogu-x/gogs/game/actv/internal/trigger"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestMain 为活动测试初始化日志。
func TestMain(m *testing.M) { tlog.NewLog("", 0); os.Exit(m.Run()) }

type queueObserver struct {
	base.BaseBehavior
	received []string
	times    []int64
}

func (b *queueObserver) HandleEvent(event *sspb.Event) {
	b.received = append(b.received, event.GetName())
	b.times = append(b.times, gameevent.PlayerEventTime(event).UnixNano())
}
func (b *queueObserver) HandleRequest(uint64, proto.Message) {
	b.received = append(b.received, "request")
}

func queueActor() *Actv {
	actor := NewActv()
	actor.ready = true
	actor.creationTrigger = trigger.NewEngine(
		nil, actor.actvTemplateScope, 0, time.Local,
	)
	return actor
}

// TestUnifiedPlayerQueue 验证原始事件和请求按同一个 FIFO 回放，事件和请求各自进入模板对应接口。
func TestUnifiedPlayerQueue(t *testing.T) {
	actor := queueActor()
	state := &playerLoadState{loading: true}
	actor.players[7] = state
	observer := &queueObserver{}
	actor.actvService.AddActv(&service.Actv{Meta: &service.ActvMeta{ActvID: 100, OwnerID: 7, OwnerScope: service.ScopePlayer}, Ext: observer})
	other := &queueObserver{}
	actor.actvService.AddActv(&service.Actv{Meta: &service.ActvMeta{ActvID: 101, OwnerID: 8, OwnerScope: service.ScopePlayer}, Ext: other})
	login := gameevent.NewPlayer(gameevent.NameUserLogin, 7, 2)
	login.ExtArgs[gameevent.ParamOccurredAt] = strconv.FormatInt(time.Now().Add(-time.Hour).UnixNano(), 10)
	actor.onEvent(nil, login)
	actor.dispatchPlayerRequest(nil, &ipb.PlayerActvMessage{UID: 7, ActvID: 100, Message: &pb_actv.GetProgressReq{}})
	actor.onEvent(nil, gameevent.NewPlayer(gameevent.NameUserLogout, 7, 2))
	if len(state.pending) != 3 || len(observer.received) != 0 {
		t.Fatal("加载期间没有统一暂存")
	}
	actor.onPlayerLoadFinished(&directContext{}, 7, state, nil)
	want := []string{gameevent.NameUserLogin, "request", gameevent.NameUserLogout}
	if len(observer.received) != len(want) {
		t.Fatalf("事件顺序错误: %v", observer.received)
	}
	for i, v := range want {
		if observer.received[i] != v {
			t.Fatalf("事件顺序错误: %v", observer.received)
		}
	}
	if observer.times[0] != gameevent.PlayerEventTime(login).UnixNano() || len(state.pending) != 0 || len(other.received) != 0 {
		t.Fatal("时间、在线状态、玩家隔离或队列清理错误")
	}
}

// TestReleaseDuringLoad 验证释放事件等待加载完成并走同一队列，不丢弃初始化。
func TestReleaseDuringLoad(t *testing.T) {
	actor := queueActor()
	state := &playerLoadState{loading: true}
	actor.players[7] = state
	actor.onEvent(&directContext{}, gameevent.NewPlayer(gameevent.NamePlayerReleased, 7, 0))
	if actor.players[7] != state || len(state.pending) != 1 {
		t.Fatal("释放事件未暂存")
	}
	actor.onPlayerLoadFinished(&directContext{}, 7, state, nil)
	if actor.actvService.IsPlayerLoaded(7) || actor.players[7] != nil {
		t.Fatal("加载完成后未释放")
	}
}

// TestStartupPlayerQueue 验证全局加载期间也保留事件和请求之间的顺序。
func TestStartupPlayerQueue(t *testing.T) {
	actor := queueActor()
	actor.ready = false
	actor.onEvent(nil, gameevent.NewPlayer(gameevent.NameUserLogin, 7, 1))
	actor.onPlayerActvMessage(&directContext{}, &ipb.PlayerActvMessage{UID: 7, Message: &pb_actv.GetActvListReq{}})
	actor.onEvent(nil, gameevent.NewPlayer(gameevent.NameUserLogout, 7, 1))
	state := &playerLoadState{loading: true}
	actor.players[7] = state
	actor.ready = true
	actor.drainPending(&directContext{})
	if len(state.pending) != 3 {
		t.Fatal("启动暂存消息未进入玩家队列")
	}
	if _, ok := state.pending[1].message.(*ipb.PlayerActvMessage); !ok {
		t.Fatal("请求与事件顺序被打乱")
	}
}

// TestBusinessPathsNeverLoad 验证普通事件、请求都不能隐式初始化玩家活动。
func TestBusinessPathsNeverLoad(t *testing.T) {
	actor := queueActor()
	actor.onEvent(&directContext{}, gameevent.NewPlayer(gameevent.NameUserLogin, 7, 1))
	actor.onPlayerActvMessage(&directContext{}, &ipb.PlayerActvMessage{UID: 7, Message: &pb_actv.GetActvListReq{}})
	if len(actor.players) != 0 || actor.actvService.IsPlayerLoaded(7) {
		t.Fatal("业务消息隐式加载了玩家")
	}
	// 存盘释放期间只有明确初始化事件能够要求重载。
	state := &playerLoadState{releasing: true}
	actor.players[7] = state
	actor.onEvent(&directContext{}, gameevent.NewPlayer(gameevent.NamePlayerInitialized, 7, 2))
	if !state.reload {
		t.Fatal("初始化事件没有安排重载")
	}
	actor.onEvent(&directContext{}, gameevent.NewPlayer(gameevent.NameUserLogin, 7, 2))
	actor.onPlayerActvMessage(&directContext{}, &ipb.PlayerActvMessage{UID: 7, Message: &pb_actv.GetActvListReq{}})
	if len(state.pending) != 2 {
		t.Fatal("明确初始化后的事件与请求未暂存")
	}
}
