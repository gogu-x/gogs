package actor

import (
	"fmt"
	"time"

	"github.com/gogu-x/gogs/game/actv/internal/service"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

const maxActvInstanceID = uint64(1<<63 - 1)

// registerHandlers 注册全局活动 Actor 的内部消息处理器。
func (actor *Actv) registerHandlers() {
	tree.RegisterFunc(&actor.router, &sspb.Event{}, actor.onEvent)
	tree.RegisterFunc(&actor.router, &ipb.PlayerActvMessage{}, actor.onPlayerActvMessage)
	tree.RegisterFunc(&actor.router, &ipb.SaveAllActvs{}, actor.onSaveAllActvs)
}

// onPlayerActvMessage 将玩家活动请求分发到实例绑定的模板行为；玩家数据未就绪时按玩家暂存。
func (actor *Actv) onPlayerActvMessage(ctx tree.Context, message *ipb.PlayerActvMessage) {
	if ctx == nil {
		return
	}
	if !actor.ready {
		if len(actor.pending) >= maxPendingActvRequests {
			logActvFailure(message, fmt.Errorf("actv data is still loading"))
			return
		}
		actor.pending = append(actor.pending, pendingActvRequest{message: message})
		return
	}
	if actor.loadErr != nil {
		logActvFailure(message, actor.loadErr)
		return
	}
	actor.dispatchPlayerRequest(ctx, message)
}

// handlePlayerActvMessage 只分发消息；列表和具体业务各自选择推送。
func (actor *Actv) handlePlayerActvMessage(ctx tree.Context, message *ipb.PlayerActvMessage) {
	uid := message.GetUID()
	request, ok := message.GetMessage().(proto.Message)
	if uid == 0 || !ok || request == nil || !request.ProtoReflect().IsValid() {
		logActvFailure(message, fmt.Errorf("invalid actv request"))
		return
	}
	if actor.actvService == nil {
		logActvFailure(message, fmt.Errorf("actv service unavailable"))
		return
	}
	if _, ok := request.(*pb_actv.GetActvListReq); ok {
		pushActv(ctx, message, actor.actvList(uid))
		return
	}
	actvID := message.GetActvID()
	if actvID > maxActvInstanceID {
		logActvFailure(message, fmt.Errorf("invalid actv id"))
		return
	}
	actor.actvService.HandleRequest(uid, int64(actvID), request)
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
		if err != nil {
			tlog.Log.Error("[actv/Actv.saveAll] 活动数据存盘失败, err=%v", err)
			return
		}
		tlog.Log.Info("[actv/Actv.saveAll] 活动数据存盘完成")
	})
}

// actvList 将玩家当前已加载活动实例打包为客户端活动列表。
func (actor *Actv) actvList(uid uint64) *pb_actv.GetActvListAck {
	ack := &pb_actv.GetActvListAck{Code: pb_common.ErrCode_OK}
	actvs := actor.actvService.ListActvs(service.ScopePlayer, service.OwnerID(uid))
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
			actor.actvService.StartDueActvs(time.Now().Unix())
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
