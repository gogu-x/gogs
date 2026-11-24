package actv

import (
	"fmt"

	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
)

const maxActvInstanceID = uint64(1<<63 - 1)

// registerHandlers 注册全局活动 Actor 的内部消息处理器。
func (actor *Actv) registerHandlers() {
	tree.RegisterFunc(&actor.router, &sspb.Event{}, actor.onEvent)
	tree.RegisterFunc(&actor.router, &ipb.ForwardedRequest{}, actor.onForwardedRequest)
	tree.RegisterFunc(&actor.router, &ipb.SaveAllActvs{}, actor.onSaveAllActvs)
}

// onForwardedRequest 处理玩家 Actor 转交的活动请求：全局或玩家活动数据加载期间按收件顺序暂存，
// 就绪后活动列表由本 Actor 直接回包，其余请求按请求中的活动编号交给模板处理。
func (actor *Actv) onForwardedRequest(ctx tree.Context, forwarded *ipb.ForwardedRequest) {
	if ctx == nil || forwarded == nil || forwarded.GetUID() == 0 || forwarded.GetRequest() == nil {
		tlog.Log.Error("[actv/Actv.onForwardedRequest] 活动请求无效")
		return
	}
	if !actor.acceptRequest(ctx, forwarded) || !actor.playerRequestReady(ctx, forwarded) {
		return
	}
	uid := forwarded.GetUID()
	if _, ok := forwarded.GetRequest().(*pb_actv.GetActvListReq); ok {
		tlog.Log.Info("[actv/Actv.onForwardedRequest] 获取活动列表, pid=%v", uid)
		pushActv(ctx, forwarded, actor.actvList(uid))
		return
	}
	target, ok := forwarded.GetRequest().(interface{ GetActvId() uint64 })
	if !ok || target.GetActvId() > maxActvInstanceID {
		actor.rejectRequest(ctx, forwarded, fmt.Errorf("invalid actv request %T", forwarded.GetRequest()))
		return
	}
	actor.actvService.HandleRequest(uid, int64(target.GetActvId()), forwarded.GetRequest(), forwarded.GetGeneration())
}

// acceptRequest 在全局活动数据加载期间暂存请求；加载失败或暂存已满时拒绝请求。
func (actor *Actv) acceptRequest(ctx tree.Context, forwarded *ipb.ForwardedRequest) bool {
	if actor.loadErr != nil {
		actor.rejectRequest(ctx, forwarded, actor.loadErr)
		return false
	}
	if actor.ready {
		return true
	}
	if len(actor.pending) >= maxPendingActvRequests {
		actor.rejectRequest(ctx, forwarded, fmt.Errorf("actv initialization queue full"))
		return false
	}
	actor.pending = append(actor.pending, pendingActvRequest{message: forwarded})
	return false
}

// rejectRequest 记录活动请求无法处理的原因，并向发起请求的会话推送 ErrorNtf(INTERNAL)。
func (actor *Actv) rejectRequest(ctx tree.Context, forwarded *ipb.ForwardedRequest, err error) {
	tlog.Log.Error("[actv/Actv.rejectRequest] 活动请求无法处理, pid=%v type=%T err=%v", forwarded.GetUID(), forwarded.GetRequest(), err)
	pushActv(ctx, forwarded, ipb.NewErrorNotification(forwarded.GetRequest(), pb_common.ErrCode_INTERNAL))
}

// onSaveAllActvs 保存所有活动实例的脏元数据。
func (actor *Actv) onSaveAllActvs(ctx tree.Context, _ *ipb.SaveAllActvs) {
	actor.saveAll(ctx)
}

func (actor *Actv) saveAll(ctx tree.Context) {
	if actor == nil || actor.lifecycle == nil {
		return
	}
	actor.lifecycle.SaveAll(ctx, func(_ tree.Context, err error) {
		ctx.Response(nil, err)
		if err != nil {
			tlog.Log.Error("[actv/Actv.saveAll] 活动数据存盘失败, err=%v", err)
			return
		}
		tlog.Log.Info("[actv/Actv.saveAll] 活动数据存盘完成")
	})
}

// actvList 将玩家本人、本服及所属联盟的已加载活动打包为客户端列表。
func (actor *Actv) actvList(uid uint64) *pb_actv.GetActvListAck {
	ack := &pb_actv.GetActvListAck{Code: pb_common.ErrCode_OK}
	actvs := actor.actvService.ListPlayerVisibleActvs(uid, actor.playerGuildID(uid))
	actvInfos := make([]*pb_actv.ActvInfo, 0, len(actvs))
	for _, actv := range actvs {
		if actv == nil {
			continue
		}
		behavior := actv.GetExt()
		if behavior == nil {
			continue
		}
		if info := behavior.OnPack(); info != nil {
			actvInfos = append(actvInfos, info)
		}
	}
	ack.Actvs = actvInfos
	return ack
}

// onEvent 统一等待服务就绪；除明确初始化入口外，事件最终都交给 HandleEvent。
func (actor *Actv) onEvent(ctx tree.Context, message *sspb.Event) {
	if actor == nil || message == nil {
		return
	}
	if actor.loadErr != nil {
		logEventFailure(message, actor.loadErr)
		return
	}
	if !actor.ready {
		if len(actor.pending) >= maxPendingActvRequests {
			logEventFailure(message, fmt.Errorf("actv initialization queue full"))
			return
		}
		actor.addPendingEvent(message)
		return
	}
	if gameevent.IsPlayerInitialized(message) {
		actor.initializePlayer(ctx, message)
		return
	}
	// 这里只等待已经开始的玩家初始化，不负责事件归属分发，也不启动加载。
	if message.GetEventTyp() == gameevent.TypePlayerLifecycle {
		if state := actor.players[gameevent.PlayerUID(message)]; state != nil && (state.loading || state.reload) {
			state.queuePending(ctx, actor, gameevent.Clone(message))
			return
		}
	}
	actor.dispatchEvent(ctx, message)
}

// addPendingEvent 将事件放入启动等待队列，与请求保持收件顺序。
func (actor *Actv) addPendingEvent(message *sspb.Event) {
	actor.pending = append(actor.pending, pendingActvRequest{message: gameevent.Clone(message)})
}

func (actor *Actv) dispatchEvent(ctx tree.Context, message *sspb.Event) {
	if actor == nil || message == nil {
		return
	}
	if gameevent.IsServerOpened(message) {
		actor.serverOpened = true
		actor.startTrigger() // 先按配置创建实例，再启动已到开始时间的活动。
		if actor.actvService != nil {
			actor.actvService.StartDueActvs(comm.NowUnix())
		}
	}
	if actor.actvService != nil {
		actor.actvService.HandleEvent(message)
	}
	if gameevent.IsPlayerReleased(message) {
		actor.onPlayerOwnerReleased(ctx, message)
	}
	if gameevent.IsServerOpened(message) {
		tlog.Log.Info("[actv/Actv.onServerEvent] 服务器活动 Actor 已处理开服事件")
		return
	}
	if !gameevent.IsServerClosing(message) {
		return
	}
	actor.serverClosing = true
	tlog.Log.Info("[actv/Actv.onServerEvent] 服务器活动 Actor 已接收关服事件")
	actor.saveAll(ctx)
}

func (actor *Actv) drainPending(ctx tree.Context) {
	if actor == nil {
		return
	}
	pending := actor.pending
	actor.pending = nil
	if actor.loadErr != nil {
		actor.rejectPending(ctx, pending, actor.loadErr)
		return
	}
	for _, item := range pending {
		actor.replayPending(ctx, item)
	}
}
