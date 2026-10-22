package guild

import (
	"fmt"

	"github.com/gogu-x/gogs/def"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/game/guild/internal"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_guild"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/db/mongorpc"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	maxPendingGuildRequests = 1024
)

type pendingGuildRequest struct {
	message interface{}
	request *tree.Envelope
}

// Guild owns guild aggregates and their Mongo lifecycle.
type Guild struct {
	store         *internal.Store
	router        tree.Router
	messageRouter tree.Router
	model         *internal.PersistenceModel
	lifecycle     *mongorpc.Lifecycle
	wheel         *timer.TimeWheel
	ready         bool
	loadErr       error
	pending       []pendingGuildRequest
	serverOpened  bool
	serverClosing bool
}

// NewGuild creates the global guild Actor.
func NewGuild() *Guild {
	return &Guild{store: internal.NewStore()}
}

// Name returns the unique guild Actor name.
func (guild *Guild) Name() string { return def.Guild }

// OnInit registers guild routes and persistence, then loads guild aggregates.
func (guild *Guild) OnInit(ctx tree.Context) {
	internal.InitMessageHandlers(
		&guild.messageRouter,
		guild.onServerEvent,
		guild.onSaveAllGuilds,
		guild.onUnhandledMessage,
	)
	internal.InitRoutes(&guild.router, guild.store, guild.onCreateGuild)
	guild.wheel = timer.NewTimeWheel(32, ctx.Self(), ctx.System())
	guild.model = internal.NewPersistenceModel(guild.store)
	mongoStore := mongorpc.NewStore(def.Mongo)
	guild.lifecycle = mongorpc.NewLifecycle(mongoStore, guild.wheel, ctx)
	if err := guild.lifecycle.Register(guild.model); err != nil {
		guild.failLoad(ctx, err)
		return
	}
	if !guild.lifecycle.LoadAll(ctx, (*internal.Guild)(nil), bson.M{}, guild.onGuildsLoaded) {
		guild.failLoad(ctx, fmt.Errorf("guild load request could not be sent"))
		return
	}
}

// HandleMessage routes guild messages to their registered handlers.
func (guild *Guild) HandleMessage(ctx tree.Context, message interface{}) {
	guild.messageRouter.Route(ctx, message)
}

// onCreateGuild 创建联盟后通知活动服务加载该联盟的活动归属。
func (guild *Guild) onCreateGuild(ctx tree.Context, request *pb_guild.CreateGuildReq) {
	ack := guild.store.Create(request)
	if ack != nil && ack.GetCode() == pb_common.ErrCode_OK && ack.GetGuild() != nil {
		guild.notifyActivityOwnerCreated(ctx, ack.GetGuild().GetGuildId())
	}
	ctx.Response(ack, nil)
}

// notifyActivityOwnerCreated 通过单向事件通知活动服务加载联盟活动。
func (guild *Guild) notifyActivityOwnerCreated(ctx tree.Context, guildID uint64) {
	if guildID == 0 {
		return
	}
	activityPID, ok := ctx.Lookup(def.ActorActivity)
	if !ok || !ctx.Send(activityPID, &ipb.CreateGuildActivityOwner{GuildID: guildID}) {
		tlog.Log.Error("[guild/Guild.notifyActivityOwnerCreated] 联盟活动创建事件发送失败, guild=%v", guildID)
	}
}

// onServerEvent 处理 Guild Actor 的服务器开服和关服事件。
func (guild *Guild) onServerEvent(ctx tree.Context, message *sspb.Event) {
	if gameevent.IsServerOpened(message) {
		guild.serverOpened = true
		tlog.Log.Info("[guild/Guild.onServerEvent] 联盟 Actor 已处理开服事件")
		ctx.Response(nil, nil)
		return
	}
	if !gameevent.IsServerClosing(message) {
		eventName := message.GetName()
		err := fmt.Errorf("unsupported guild event %q", eventName)
		tlog.Log.Error("[guild/Guild.onServerEvent] 收到不支持的联盟事件, event=%v err=%v", eventName, err)
		ctx.Response(nil, err)
		return
	}
	guild.serverClosing = true
	tlog.Log.Info("[guild/Guild.onServerEvent] 联盟 Actor 已接收关服事件")
	if !guild.ready && guild.loadErr == nil {
		guild.queueRequest(ctx, message)
		return
	}
	guild.saveAll(ctx)
}

// onSaveAllGuilds 处理所有联盟数据的存盘请求。
func (guild *Guild) onSaveAllGuilds(ctx tree.Context, _ *ipb.SaveAllGuilds) {
	guild.saveAll(ctx)
}

// onUnhandledMessage 将联盟请求交给加载队列或业务路由处理。
func (guild *Guild) onUnhandledMessage(ctx tree.Context, message interface{}) {
	if guild.serverClosing {
		ctx.Response(nil, fmt.Errorf("guild actor is shutting down"))
		return
	}
	if guild.loadErr != nil {
		guild.respondUnavailable(ctx, message)
		return
	}
	if !guild.ready {
		guild.queueRequest(ctx, message)
		return
	}
	guild.router.Route(ctx, message)
}

// OnStop stops guild timers after the caller has completed its explicit flush.
func (guild *Guild) OnStop(_ tree.Context) {
	if guild.wheel != nil {
		guild.wheel.Stop()
	}
}

func (guild *Guild) onGuildsLoaded(ctx tree.Context, value interface{}, err error) {
	if err != nil {
		guild.failLoad(ctx, err)
		return
	}
	loaded, ok := value.(*[]internal.Guild)
	if !ok || loaded == nil {
		guild.failLoad(ctx, fmt.Errorf("invalid guild load result %T", value))
		return
	}
	guilds := make([]*internal.Guild, 0, len(*loaded))
	for index := range *loaded {
		guilds = append(guilds, &(*loaded)[index])
	}
	guild.store.ReplaceAll(guilds)
	guild.store.Range(func(owner *internal.Guild) bool {
		if owner != nil && !owner.IsDeleted() {
			guild.notifyActivityOwnerCreated(ctx, owner.GetID())
		}
		return true
	})
	guild.ready = true
	tlog.Log.Info("[guild/Guild.onGuildsLoaded] 联盟数据加载完成, guilds=%v", guild.store.Count())
	guild.drainPending(ctx)
}

func (guild *Guild) failLoad(ctx tree.Context, err error) {
	guild.loadErr = err
	guild.ready = true
	tlog.Log.Error("[guild/Guild.failLoad] 联盟数据加载失败，停止处理联盟请求, err=%v", err)
	guild.drainPending(ctx)
}

func (guild *Guild) queueRequest(ctx tree.Context, message interface{}) {
	request := ctx.RequestEnvelope()
	if request == nil {
		tlog.Log.Error("[guild/Guild.queueRequest] 联盟数据加载期间收到无请求信封消息, type=%T", message)
		return
	}
	if len(guild.pending) >= maxPendingGuildRequests {
		if gameevent.IsServerClosing(message) {
			ctx.Response(nil, fmt.Errorf("guild shutdown queue is full"))
			return
		}
		guild.respondUnavailable(ctx, message)
		tlog.Log.Error("[guild/Guild.queueRequest] 联盟加载期间请求队列已满, count=%v", len(guild.pending))
		return
	}
	guild.addPending(pendingGuildRequest{message: message, request: request})
}

func (guild *Guild) addPending(request pendingGuildRequest) {
	if guild == nil || len(guild.pending) >= maxPendingGuildRequests {
		return
	}
	guild.pending = append(guild.pending, request)
}

func (guild *Guild) drainPending(ctx tree.Context) {
	pending := guild.pending
	guild.pending = nil
	for _, item := range pending {
		if item.request == nil {
			continue
		}
		if guild.loadErr != nil {
			if gameevent.IsServerClosing(item.message) {
				item.request.Respond(nil, guild.loadErr)
				continue
			}
			item.request.Respond(guild.unavailableResponse(item.message), nil)
			continue
		}
		if gameevent.IsServerClosing(item.message) {
			guild.serverClosing = true
			guild.saveAllRequest(ctx, item.request)
			continue
		}
		response, handled := guild.store.HandlePending(item.message)
		if !handled {
			item.request.Respond(nil, fmt.Errorf("unsupported guild request %T", item.message))
			continue
		}
		if createAck, ok := response.(*pb_guild.CreateGuildAck); ok && createAck.GetCode() == pb_common.ErrCode_OK && createAck.GetGuild() != nil {
			guild.notifyActivityOwnerCreated(ctx, createAck.GetGuild().GetGuildId())
		}
		item.request.Respond(response, nil)
	}
}

func (guild *Guild) respondUnavailable(ctx tree.Context, message interface{}) {
	ctx.Response(guild.unavailableResponse(message), nil)
}

func (guild *Guild) unavailableResponse(message interface{}) interface{} {
	const reason = "guild data is unavailable"
	switch message.(type) {
	case *pb_guild.CreateGuildReq:
		return &pb_guild.CreateGuildAck{Code: pb_common.ErrCode_INTERNAL, Msg: reason}
	case *pb_guild.JoinGuildReq:
		return &pb_guild.JoinGuildAck{Code: pb_common.ErrCode_INTERNAL, Msg: reason}
	case *pb_guild.LeaveGuildReq:
		return &pb_guild.LeaveGuildAck{Code: pb_common.ErrCode_INTERNAL, Msg: reason}
	case *pb_guild.GetGuildReq:
		return &pb_guild.GetGuildAck{Code: pb_common.ErrCode_INTERNAL}
	default:
		return nil
	}
}

func (guild *Guild) saveAll(ctx tree.Context) {
	guild.saveAllRequest(ctx, ctx.RequestEnvelope())
}

func (guild *Guild) saveAllRequest(ctx tree.Context, request *tree.Envelope) {
	if guild.lifecycle == nil {
		if request != nil {
			request.Respond(nil, fmt.Errorf("guild persistence lifecycle is unavailable"))
		}
		return
	}
	if !guild.ready || guild.loadErr != nil {
		if request != nil {
			request.Respond(nil, fmt.Errorf("guild data is not ready"))
		}
		return
	}
	guild.lifecycle.SaveAll(ctx, func(_ tree.Context, err error) {
		if err != nil {
			tlog.Log.Error("[guild/Guild.saveAll] 关服前联盟存盘失败, err=%v", err)
		}
		if request != nil {
			request.Respond(nil, err)
		}
	})
}
