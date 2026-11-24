package internal

import (
	"fmt"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/guild/internal/store"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/db/mongorpc"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// maxPendingGuildRequests 是联盟加载期间等待处理的消息上限。
const maxPendingGuildRequests = 1024

// GuildActor 管理联盟聚合及其加载、存盘和会话回包。
type GuildActor struct {
	store         *store.Store
	messageRouter tree.Router
	lifecycle     *mongorpc.Lifecycle
	wheel         *timer.TimeWheel
	ready         bool
	loadErr       error
	pending       []func(tree.Context)
	serverOpened  bool
	serverClosing bool
}

// NewGuildActor 创建全局联盟 Actor。
func NewGuildActor() *GuildActor {
	return &GuildActor{store: store.NewStore()}
}

// Name 返回联盟 Actor 的注册名称。
func (guild *GuildActor) Name() string { return def.Guild }

// MailboxSize 为全服联盟查询、登录同步和存盘期间的突发请求预留缓冲。
func (guild *GuildActor) MailboxSize() int { return 32 * 1024 }

// OnInit 注册处理方法并加载联盟数据。
func (guild *GuildActor) OnInit(ctx tree.Context) {
	guild.registerHandlers()
	guild.initTimer(ctx)
	model := store.NewPersistenceModel(guild.store, guildSaveTickerType)
	mongoStore := mongorpc.NewStore(def.Mongo)
	guild.lifecycle = mongorpc.NewLifecycle(mongoStore, guild.wheel, ctx)
	if err := guild.lifecycle.Register(model); err != nil {
		guild.failLoad(ctx, err)
		return
	}
	if !guild.lifecycle.LoadAll(
		ctx,
		model.GetPrototype(),
		bson.M{},
		guild.onGuildsLoaded,
	) {
		guild.failLoad(ctx, fmt.Errorf("guild load request could not be sent"))
	}
}

// HandleMessage 按消息类型分发给已注册的处理方法。
func (guild *GuildActor) HandleMessage(ctx tree.Context, message interface{}) {
	guild.messageRouter.Route(ctx, message)
}

// OnStop 停止联盟 Actor 的存盘定时器。
func (guild *GuildActor) OnStop(_ tree.Context) {
	guild.stopTimer()
}

// notifyActvOwnerCreated 通过单向事件通知活动服务加载联盟活动。
func (guild *GuildActor) notifyActvOwnerCreated(ctx tree.Context, guildID uint64) {
	if guildID == 0 {
		return
	}
	actvPID, ok := ctx.Lookup(def.ActorActv)
	if !ok || !ctx.Send(actvPID, &ipb.CreateGuildActvOwner{GuildID: guildID}) {
		tlog.Log.Error("[guild/GuildActor.notifyActvOwnerCreated] 联盟活动创建事件发送失败, guild=%v", guildID)
	}
}

// sendPlayerGuildMembership 将联盟成员索引中的归属交给玩家 Actor 更新存档。
func (guild *GuildActor) sendPlayerGuildMembership(ctx tree.Context, uid uint64) {
	guildID := guild.store.GetPlayerGuildID(uid)
	delivery := &ipb.DeliverToPlayer{
		UID:     uid,
		Payload: &ipb.PlayerGuildMembership{UID: uid, GuildID: guildID},
	}
	if gatePID, ok := ctx.Lookup(def.GameGate); !ok || !ctx.TrySend(gatePID, delivery) {
		tlog.Log.Error("[guild/GuildActor.sendPlayerGuildMembership] 玩家联盟归属投递失败, pid=%v guild=%v", uid, guildID)
	}
}

// onGuildsLoaded 恢复联盟聚合并处理加载期间排队的请求。
func (guild *GuildActor) onGuildsLoaded(ctx tree.Context, value interface{}, err error) {
	if err != nil {
		guild.failLoad(ctx, err)
		return
	}
	loaded, ok := value.(*[]store.Guild)
	if !ok || loaded == nil {
		guild.failLoad(ctx, fmt.Errorf("invalid guild load result %T", value))
		return
	}
	guilds := make([]*store.Guild, 0, len(*loaded))
	for index := range *loaded {
		guilds = append(guilds, &(*loaded)[index])
	}
	guildStore := guild.store
	guildStore.ReplaceAll(guilds)
	guildStore.Range(func(owner *store.Guild) bool {
		if owner != nil && !owner.IsDeleted() {
			guild.notifyActvOwnerCreated(ctx, owner.GetID())
		}
		return true
	})
	guild.ready = true
	tlog.Log.Info("[guild/GuildActor.onGuildsLoaded] 联盟数据加载完成, guilds=%v", guildStore.Count())
	guild.drainPending(ctx)
}

// failLoad 记录加载错误，并回复正在等待的请求。
func (guild *GuildActor) failLoad(ctx tree.Context, err error) {
	guild.loadErr = err
	guild.ready = true
	tlog.Log.Error("[guild/GuildActor.failLoad] 联盟数据加载失败，停止处理联盟请求, err=%v", err)
	guild.drainPending(ctx)
}

// addPending 将普通请求或关服请求按收件顺序加入加载队列。
func (guild *GuildActor) addPending(handle func(tree.Context)) bool {
	if len(guild.pending) >= maxPendingGuildRequests {
		return false
	}
	guild.pending = append(guild.pending, handle)
	return true
}

// drainPending 按收到的顺序处理加载期间排队的请求。
func (guild *GuildActor) drainPending(ctx tree.Context) {
	pending := guild.pending
	guild.pending = nil
	for _, handle := range pending {
		handle(ctx)
	}
}

// saveAllRequest 保存联盟数据并向系统请求方返回结果。
func (guild *GuildActor) saveAllRequest(ctx tree.Context, request *tree.Envelope) {
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
			tlog.Log.Error("[guild/GuildActor.saveAllRequest] 关服前联盟存盘失败, err=%v", err)
		}
		if request != nil {
			request.Respond(nil, err)
		}
	})
}
