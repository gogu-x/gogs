//go:generate actv-router-gen

// Package sevenday 实现累计七日登录的个人活动。
package sevenday

import (
	"time"

	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree/tlog"
)

const (
	compLogin  = int32(1) // 登录进度组件编号，发布后不可更改。
	targetDays = int32(7) // 累计登录目标天数。
)

// Behavior 每个玩家、每份配置独立保存七日登录进度。
type Behavior struct {
	generation        uint64 // 模板已接收的会话代次，过滤过期上下线事件。
	base.BaseBehavior        // 元数据、组件注册和推送能力。
	login             *base.Comp[LoginData]
	online            bool
	nextReset         int64
}

// Create 创建个人活动行为，不在模板内决定是否创建活动实例。
func (*Behavior) Create(actv *service.Actv, cfg conf.ActvConfig) service.ActvBehavior {
	b := &Behavior{}
	b.Bind(actv, cfg)
	return b
}

// OnInit 注册随玩家活动实例存盘的单条进度组件。
func (b *Behavior) OnInit() {
	b.login = base.NewComp[LoginData](compLogin)
	b.AddComp(b.login)
}

// OnTicker 复用活动 Actor 的秒级心跳，在服务器本地零点切日并推送在线玩家。
func (b *Behavior) OnTicker() {
	now := time.Now()
	if now.Unix() < b.nextReset {
		return
	}
	if b.advance(now, b.online) && b.online {
		b.pushState()
	}
}

// GetLoginData 返回模板内存数据，修改后由调用方标记组件脏状态。
func (b *Behavior) GetLoginData() *LoginData { return b.login.GetData() }

// updateOnline 更新模板自己的在线状态，事件处理时调用。
func (b *Behavior) updateOnline(online bool, now time.Time) {
	// 离线前推进日期，确保跨零点后先收到断线消息也不会漏算。
	b.advance(now, b.online || online)
	b.online = online
	if online {
		b.pushState()
	}
}

// advance 按服务器自然日去重；离线间隔无论多少天，只计本次登录的一天。
func (b *Behavior) advance(now time.Time, online bool) bool {
	if b.login.Blocked() {
		return false
	}
	data := b.GetLoginData()
	day := time.Date(
		now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location(),
	)
	start := day.Unix()
	b.nextReset = day.AddDate(0, 0, 1).Unix()
	// 时钟回拨不得回到旧日期重复累计。
	if start < data.GetDayStart() {
		return false
	}
	changed := false
	if start > data.GetDayStart() {
		data.resetDay(start)
		changed = true
	}
	if online && data.GetDailyCount() == 0 {
		data.SetDailyCount(1)
		before := data.GetLoginDays()
		if before < targetDays {
			data.SetLoginDays(before + 1)
		}
		changed = true
		tlog.Log.Info(
			"[sevenday/advance] 记录当日登录, pid=%v before=%v after=%v day=%v",
			b.Meta().GetOwnerID(), before, data.GetLoginDays(), start,
		)
	}
	if changed {
		b.login.MarkDirty()
	}
	return changed
}

// resetDay 清空新自然日的有效登录次数，保留累计进度。
func (data *LoginData) resetDay(start int64) {
	data.SetDayStart(start)
	data.SetDailyCount(0)
}

// packState 生成独立消息快照，避免网关异步发送期间数据变化。
func (b *Behavior) packState() *pb_actv.SevenDayLoginData {
	data := b.GetLoginData()
	return &pb_actv.SevenDayLoginData{
		ActvId: uint64(b.Meta().GetActvID()), LoginDays: data.GetLoginDays(),
		TodayLoginCount: data.GetDailyCount(), NextResetTime: b.nextReset,
		Completed: data.GetLoginDays() >= targetDays,
	}
}

// pushState 主动向当前会话推送，不依赖玩家拉取或请求回调。
func (b *Behavior) pushState() {
	if b.login.Blocked() {
		return
	}
	b.PushToPlayer(uint64(b.Meta().GetOwnerID()), &pb_actv.SevenDayLoginNtf{Data: b.packState()})
}

// handleGetSevenDayLogin 提供可选补拉接口，所有业务校验及响应由模板决定。
// @actv.request
func (b *Behavior) handleGetSevenDayLogin(uid uint64, request *pb_actv.GetSevenDayLoginReq) {
	meta := b.Meta()
	ack := &pb_actv.GetSevenDayLoginAck{Code: pb_common.ErrCode_OK}
	switch {
	case uid == 0 || uint64(meta.GetOwnerID()) != uid || request.GetActvId() != uint64(meta.GetActvID()):
		ack.Code = pb_common.ErrCode_ACTV_NOT_FOUND
	case b.login.Blocked():
		ack.Code = pb_common.ErrCode_ACTV_UNAVAILABLE
	case meta.GetStartTs() > time.Now().Unix():
		ack.Code = pb_common.ErrCode_ACTV_NOT_OPEN
	default:
		b.advance(time.Now(), b.online)
		ack.Data = b.packState()
	}
	b.PushToPlayer(uid, ack)
}

// OnPack 返回列表信息；完成状态保留，但不结束或删除实例。
func (b *Behavior) OnPack() *pb_actv.ActvInfo {
	info := b.BaseBehavior.OnPack()
	if info == nil {
		return nil
	}
	info.Completed = b.GetLoginData().GetLoginDays() >= targetDays
	if info.GetCompleted() {
		info.Status = pb_actv.ActvStatus_ENDED
	}
	return info
}

// handleLogin 消费玩家登录事件，直接使用事件发生时间计数。
// @actv.event 2 "userLogin"
func (b *Behavior) handleLogin(event *sspb.Event) {
	if gameevent.PlayerUID(event) != uint64(b.Meta().GetOwnerID()) {
		return
	}
	generation := gameevent.PlayerGeneration(event)
	if generation == 0 || generation <= b.generation {
		return
	}
	b.generation = generation
	b.updateOnline(true, gameevent.PlayerEventTime(event))
}

// handleLogout 消费玩家离线事件，跨零点时先结算再离线。
// @actv.event 2 "userLogout"
func (b *Behavior) handleLogout(event *sspb.Event) {
	if gameevent.PlayerUID(event) != uint64(b.Meta().GetOwnerID()) {
		return
	}
	if generation := gameevent.PlayerGeneration(event); generation != b.generation {
		return
	}
	b.updateOnline(false, gameevent.PlayerEventTime(event))
}

// handleRelease 在玩家释放前结束在线状态，不另建会话回调接口。
// @actv.event 2 "playerReleased"
func (b *Behavior) handleRelease(event *sspb.Event) { b.handleLogout(event) }
