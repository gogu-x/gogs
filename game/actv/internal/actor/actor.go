package actor

import (
	"time"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	templates "github.com/gogu-x/gogs/game/actv/internal/template"
	"github.com/gogu-x/gogs/game/actv/internal/trigger"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/db/mongorpc"
	"github.com/gogu-x/tree/timer"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	// defaultActvServerID 表示未传入服务器编号时使用的默认编号。
	defaultActvServerID = uint64(1)
	// actvOwnerSaveInterval 表示活动记录的定时存盘间隔。
	actvOwnerSaveInterval = time.Minute
	// maxPendingActvRequests 表示活动 Actor 加载常驻归属数据期间暂存玩家事件与请求的总数量上限。
	maxPendingActvRequests = 100000
)

// pendingActvRequest 保存活动 Actor 加载期间暂存的单向消息。
type pendingActvRequest struct {
	message interface{}
}

// Actv 统一管理不同归属对象的活动状态及其存盘生命周期。
type Actv struct {
	creationTrigger *trigger.Engine // 统一配置触发创建引擎。
	serverID        uint64
	router          tree.Router
	wheel           *timer.TimeWheel
	selfPID         tree.PID
	store           *mongorpc.Store
	lifecycle       *mongorpc.Lifecycle
	model           *actvPersistenceModel
	dataModel       *actvDataPersistenceModel
	actvService     *service.ActvService
	ready           bool
	loadErr         error
	pending         []pendingActvRequest
	players         map[uint64]*playerLoadState // 玩家活动数据的加载/释放状态。
	serverOpened    bool
	serverClosing   bool
	triggerStarted  bool // 触发创建定时器是否已启动。
}

// NewActv 创建可选服务器编号对应的全局活动 Actor。
func NewActv() *Actv {
	serverID := uint64(conf.ServerID)
	if serverID == 0 {
		serverID = defaultActvServerID
	}
	return &Actv{
		serverID:    serverID,
		actvService: service.NewActvService(serverID),
		players:     make(map[uint64]*playerLoadState),
	}
}

// Name 返回全局活动 Actor 的名称。
func (actor *Actv) Name() string { return def.ActorActv }

func (actor *Actv) MailboxSize() int { return 1000000 }

// OnInit 注册消息路由、存盘生命周期、时间轮和活动模板。
// 启动时先创建索引，再加载服务器/联盟活动元数据，最后加载这些活动的单条组件数据。
func (actor *Actv) OnInit(ctx tree.Context) {
	actor.registerHandlers()
	actor.registerTimers(ctx)
	actor.store = mongorpc.NewStore(def.Mongo)
	actor.lifecycle = mongorpc.NewLifecycle(actor.store, actor.wheel, ctx)
	templates.Register(actor.actvService)
	actor.model = newActvPersistenceModel(actor.actvService)
	actor.dataModel = newActvDataPersistenceModel(actor.actvService)
	for _, model := range []mongorpc.Model{actor.model, actor.dataModel} {
		if err := actor.lifecycle.Register(model); err != nil {
			actor.failLoad(ctx, err)
			return
		}
	}
	actor.ensureIndexes(ctx)
	if !actor.lifecycle.LoadAll(ctx, (*service.ActvMeta)(nil), retainedActvFilter(bson.M{
		"owner_scope": bson.M{"$in": bson.A{int32(service.ScopeServer), int32(service.ScopeGuild)}},
	}, time.Now().Unix()), actor.onActvsLoaded) {
		actor.failLoad(ctx, nil)
		return
	}
	actor.scheduleLifecycleTick()
}

// HandleMessage 将全局活动消息分发给已注册的处理方法。
func (actor *Actv) HandleMessage(ctx tree.Context, message interface{}) {
	actor.router.Route(ctx, message)
}

// OnStop 停止活动存盘生命周期和时间轮。
func (actor *Actv) OnStop(_ tree.Context) {
	if actor.lifecycle != nil {
		actor.lifecycle.Stop()
	}
	if actor.wheel != nil {
		actor.wheel.Stop()
	}
}
