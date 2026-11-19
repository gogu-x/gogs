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
	"google.golang.org/protobuf/proto"
)

const maxActvInstanceID = uint64(1<<63 - 1)

// registerHandlers 注册全局活动 Actor 的内部消息处理器。
func (actor *Actv) registerHandlers() {
	tree.RegisterFunc(&actor.router, &sspb.Event{}, actor.onEvent)
	tree.RegisterFunc(&actor.router, &ipb.ActvMessage{}, actor.onPlayerActvMessage)
	tree.RegisterFunc(&actor.router, &ipb.SaveAllActvs{}, actor.onSaveAllActvs)
	tree.RegisterFunc(&actor.router, &ipb.ActvList{}, actor.handleGetActvList)
}

// onPlayerActvMessage 将玩家活动请求分发到实例绑定的模板行为；玩家数据未就绪时按玩家暂存。
func (actor *Actv) onPlayerActvMessage(ctx tree.Context, message *ipb.ActvMessage) {
	if ctx == nil || !actor.acceptActvRequest(message) {
		return
	}
	actor.dispatchPlayerRequest(ctx, message)
}

// actvPlayerRequest 提供请求排队所需的玩家身份。
type actvPlayerRequest interface{ GetUID() uint64 }

// acceptActvRequest 在全局活动数据加载期间保留原始消息类型。
func (actor *Actv) acceptActvRequest(message actvPlayerRequest) bool {
	if !actor.ready {
		if len(actor.pending) >= maxPendingActvRequests {
			tlog.Log.Error("[actv/Actv.acceptActvRequest] 活动等待队列已满, pid=%v", message.GetUID())
			return false
		}
		actor.pending = append(actor.pending, pendingActvRequest{message: message})
		return false
	}
	if actor.loadErr != nil {
		tlog.Log.Error("[actv/Actv.acceptActvRequest] 活动数据加载失败, pid=%v err=%v", message.GetUID(), actor.loadErr)
		return false
	}
	return true
}

// handlePlayerActvMessage 只分发消息；列表和具体业务各自选择推送。
func (actor *Actv) handlePlayerActvMessage(ctx tree.Context, message *ipb.ActvMessage) {
	uid := message.GetUID()
	request, ok := message.GetMessage().(proto.Message)
	if uid == 0 || !ok || request == nil || !request.ProtoReflect().IsValid() {
		tlog.Log.Error("[actv/Actv.handlePlayerActvMessage] 活动请求无效, pid=%v", uid)
		return
	}
	if actor.actvService == nil {
		tlog.Log.Error("[actv/Actv.handlePlayerActvMessage] 活动服务不可用, pid=%v", uid)
		return
	}
	actvID := message.GetActvID()
	if actvID > maxActvInstanceID {
		tlog.Log.Error("[actv/Actv.handlePlayerActvMessage] 活动编号无效, pid=%v actv=%v", uid, actvID)
		return
	}
	actor.actvService.HandleRequest(uid, int64(actvID), request)
}

// handleGetActvList 处理活动列表请求并推送当前已加载的活动信息。
func (actor *Actv) handleGetActvList(ctx tree.Context, message *ipb.ActvList) {
	if ctx == nil || message == nil || message.GetUID() == 0 || !actor.acceptActvRequest(message) || !actor.playerRequestReady(message) {
		return
	}
	uid := message.GetUID()
	tlog.Log.Info("[actv/Actv.handleGetActvList] 获取活动列表, pid=%v", uid)
	pushActv(ctx, message, actor.actvList(uid))
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
		logPlayerInputFailure(message, actor.loadErr)
		return
	}
	if !actor.ready {
		if len(actor.pending) >= maxPendingActvRequests {
			logPlayerInputFailure(message, fmt.Errorf("actv initialization queue full"))
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
			state.queuePending(gameevent.Clone(message))
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
		respondPending(ctx, pending, actor.loadErr)
		return
	}
	for _, item := range pending {
		actor.replayPending(ctx, item)
	}
}
