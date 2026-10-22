package actor

import (
	"fmt"

	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func (actor *Activity) failLoad(ctx tree.Context, err error) {
	actor.loadErr = err
	actor.ready = true
	tlog.Log.Error("[activity/Activity.failLoad] 活动数据加载失败, err=%v", err)
	actor.drainPending(ctx)
}

// onActivitiesLoaded 恢复服务器/联盟活动实例，并继续加载它们的单条组件数据。
func (actor *Activity) onActivitiesLoaded(ctx tree.Context, value interface{}, err error) {
	if err != nil {
		actor.failLoad(ctx, err)
		return
	}
	loaded, ok := value.(*[]service.ActvMeta)
	if !ok || loaded == nil {
		actor.failLoad(ctx, fmt.Errorf("invalid activity load result %T", value))
		return
	}
	activityIDs := bson.A{}
	for index := range *loaded {
		meta := &(*loaded)[index]
		scope := meta.GetOwnerScope()
		if scope != service.ScopeServer && scope != service.ScopeGuild {
			continue
		}
		if meta.GetOwnerID() <= 0 || (scope == service.ScopeServer && uint64(meta.GetOwnerID()) != actor.serverID) {
			continue
		}
		if actor.restoreActivity(meta) {
			activityIDs = append(activityIDs, meta.GetActvID())
		}
	}
	tlog.Log.Info("[activity/Activity.onActivitiesLoaded] 服务器和联盟活动元数据加载完成, count=%v restored=%v", len(*loaded), len(activityIDs))
	if len(activityIDs) == 0 {
		actor.finishStartupLoad(ctx)
		return
	}
	// 只加载归属于服务器/联盟的单条数据；玩家列表条目归属玩家，随玩家上线加载。
	filter := bson.M{
		"actv_id":     bson.M{"$in": activityIDs},
		"owner_scope": bson.M{"$in": bson.A{int32(service.ScopeServer), int32(service.ScopeGuild)}},
	}
	if !actor.lifecycle.LoadAll(ctx, (*service.ActvDataDoc)(nil), filter, actor.onActivityDataLoaded) {
		actor.failLoad(ctx, fmt.Errorf("activity data load request could not be sent"))
	}
}

// onActivityDataLoaded 将服务器/联盟活动的单条组件数据交给对应组件恢复。
func (actor *Activity) onActivityDataLoaded(ctx tree.Context, value interface{}, err error) {
	if err != nil {
		actor.failLoad(ctx, err)
		return
	}
	docs, ok := value.(*[]service.ActvDataDoc)
	if !ok || docs == nil {
		actor.failLoad(ctx, fmt.Errorf("invalid activity data load result %T", value))
		return
	}
	// 启动只加载服务器/联盟自己的单条数据，不含玩家条目，因此不会产生待结算条目。
	actor.actvService.ApplyDataDocs(*docs, 0)
	tlog.Log.Info("[activity/Activity.onActivityDataLoaded] 服务器和联盟活动组件数据加载完成, count=%v", len(*docs))
	actor.finishStartupLoad(ctx)
}

func (actor *Activity) finishStartupLoad(ctx tree.Context) {
	for _, actv := range actor.actvService.ListAllActvs() {
		actv.GetExt().OnLoaded()
	}
	actor.ready = true
	actor.drainPending(ctx)
}

func (actor *Activity) restoreActivity(meta *service.ActvMeta) bool {
	if actor == nil || actor.actvService == nil || meta == nil || meta.GetActvID() <= 0 || meta.GetCfgID() <= 0 || meta.GetOwnerID() <= 0 {
		return false
	}
	configRow := glconf.GetActvOnlineCfgNoErr(meta.GetCfgID())
	if configRow == nil {
		tlog.Log.Error("[activity/Activity.restoreActivity] 活动配置不存在，跳过实例恢复, activity=%v config=%v", meta.GetActvID(), meta.GetCfgID())
		return false
	}
	templateType := meta.GetTemplateType()
	if templateType == 0 {
		templateType = service.TemplateType(configRow.GetType())
		meta.TemplateType = templateType
	}
	if templateType != service.TemplateType(configRow.GetType()) {
		tlog.Log.Error("[activity/Activity.restoreActivity] 活动模板类型与配置不一致，跳过实例恢复, activity=%v config=%v template=%v cfgTemplate=%v",
			meta.GetActvID(), meta.GetCfgID(), templateType, configRow.GetType())
		return false
	}
	actv := actor.actvService.NewActv(meta, service.NewActivityConfig(configRow, meta))
	if actv == nil || !actor.actvService.AddActv(actv) {
		return false
	}
	meta.SetDirty(false)
	return true
}
