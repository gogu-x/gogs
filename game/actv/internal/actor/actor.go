package actor

import (
	"time"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	templates "github.com/gogu-x/gogs/game/actv/internal/template"
	"github.com/gogu-x/gogs/game/actv/internal/trigger"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/db/mongorpc"
	"github.com/gogu-x/tree/timer"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	// defaultActivityServerID 表示未传入服务器编号时使用的默认编号。
	defaultActivityServerID = uint64(1)
	// activityOwnerSaveInterval 表示活动记录的定时存盘间隔。
	activityOwnerSaveInterval = time.Minute
	// maxPendingActivityRequests 表示活动 Actor 加载常驻归属数据期间暂存请求的数量上限。
	maxPendingActivityRequests = 1024
)

// pendingActivityRequest 保存活动 Actor 加载期间暂存的消息和请求信封。
type pendingActivityRequest struct {
	message interface{}
	request *tree.Envelope
}

// Activity 统一管理不同归属对象的活动状态及其存盘生命周期。
type Activity struct {
	serverID       uint64
	router         tree.Router
	wheel          *timer.TimeWheel
	system         *tree.Tree
	selfPID        tree.PID
	store          *mongorpc.Store
	lifecycle      *mongorpc.Lifecycle
	model          *activityPersistenceModel
	dataModel      *activityDataPersistenceModel
	actvService    *service.ActvService
	ready          bool
	loadErr        error
	pending        []pendingActivityRequest
	pendingEvents  []*sspb.Event
	pendingLogins  map[uint64]bool             // 启动加载期间上线、待 ready 后加载的玩家。
	players        map[uint64]*playerLoadState // 玩家活动数据的加载/释放状态。
	serverOpened   bool
	serverClosing  bool
	triggerStarted bool            // 触发创建定时器是否已启动。
	triggerPlans   []*trigger.Plan // 服务器归属活动的时间规则，触发启动时解析一次。
}

// NewActivity 创建可选服务器编号对应的全局活动 Actor。
func NewActivity() *Activity {
	serverID := uint64(conf.ServerID)
	if serverID == 0 {
		serverID = defaultActivityServerID
	}
	return &Activity{
		serverID:      serverID,
		actvService:   service.NewActvService(serverID),
		pendingLogins: make(map[uint64]bool),
		players:       make(map[uint64]*playerLoadState),
	}
}

// Name 返回全局活动 Actor 的名称。
func (actor *Activity) Name() string { return def.ActorActivity }

// OnInit 注册消息路由、存盘生命周期、时间轮和活动模板。
// 启动时先创建索引，再加载服务器/联盟活动元数据，最后加载这些活动的单条组件数据。
func (actor *Activity) OnInit(ctx tree.Context) {
	actor.registerHandlers()
	actor.registerTimers(ctx)
	actor.store = mongorpc.NewStore(def.Mongo)
	actor.lifecycle = mongorpc.NewLifecycle(actor.store, actor.wheel, ctx)
	templates.Register(actor.actvService)
	actor.model = newActivityPersistenceModel(actor.actvService)
	actor.dataModel = newActivityDataPersistenceModel(actor.actvService)
	for _, model := range []mongorpc.Model{actor.model, actor.dataModel} {
		if err := actor.lifecycle.Register(model); err != nil {
			actor.failLoad(ctx, err)
			return
		}
	}
	actor.ensureIndexes(ctx)
	if !actor.lifecycle.LoadAll(ctx, (*service.ActvMeta)(nil), bson.M{
		"owner_scope": bson.M{"$in": bson.A{int32(service.ScopeServer), int32(service.ScopeGuild)}},
	}, actor.onActivitiesLoaded) {
		actor.failLoad(ctx, nil)
		return
	}
	actor.scheduleLifecycleTick()
}

// HandleMessage 将全局活动消息分发给已注册的处理方法。
func (actor *Activity) HandleMessage(ctx tree.Context, message interface{}) {
	actor.router.Route(ctx, message)
}

// OnStop 停止活动存盘生命周期和时间轮。
func (actor *Activity) OnStop(_ tree.Context) {
	if actor.lifecycle != nil {
		actor.lifecycle.Stop()
	}
	if actor.wheel != nil {
		actor.wheel.Stop()
	}
}
