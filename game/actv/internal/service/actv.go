package service

import (
	activityconf "github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree/tlog"
)

// ActvService 管理活动模板注册表、活动实例和归属索引。
type ActvService struct {
	Tmp           map[TemplateType]ActvTemplate      // 已注册的活动模板。
	actcMgr       map[ownerKey]map[int64]*Actv       // 每个归属对象持有的活动实例。
	actvOwnerMgr  map[int64]ownerKey                 // 每个活动实例对应的归属对象。
	ownerScopeMgr map[OwnerScope]map[OwnerID][]int64 // 每个归属对象的活动实例顺序。
	actvOrder     []int64                            // 活动实例的全局注册顺序。
	loadedPlayers map[uint64]bool                    // 活动数据已加载的玩家，新增实例时据此补标记。
}

// ownerKey 唯一标识一个归属范围和归属对象。
type ownerKey struct {
	scope OwnerScope
	id    OwnerID
}

// NewActvService 创建空的活动服务。
func NewActvService() *ActvService {
	return &ActvService{
		Tmp:           make(map[TemplateType]ActvTemplate),
		actcMgr:       make(map[ownerKey]map[int64]*Actv),
		actvOwnerMgr:  make(map[int64]ownerKey),
		ownerScopeMgr: make(map[OwnerScope]map[OwnerID][]int64),
		loadedPlayers: make(map[uint64]bool),
	}
}

// Actv 保存活动元数据和不参与序列化的模板运行时对象。
type Actv struct {
	Meta        *ActvMeta                 // 活动实例元数据。
	Ext         ActvBehavior              `bson:"-"` // 活动模板运行时对象。
	comps       map[int32]Component       // 模板注册的单条数据组件。
	playerComps map[int32]PlayerComponent // 模板注册的玩家列表组件。
}

// GetMeta 返回活动实例元数据。
func (actv *Actv) GetMeta() *ActvMeta {
	if actv == nil {
		return nil
	}
	return actv.Meta
}

// GetExt 返回活动模板运行时对象。
func (actv *Actv) GetExt() ActvBehavior {
	if actv == nil {
		return nil
	}
	return actv.Ext
}

// ActvMeta 保存活动实例的持久化元数据。
type ActvMeta struct {
	ActvID       int64        `bson:"_id"`           // 活动实例编号。
	StartTs      int64        `bson:"start_ts"`      // 活动开始时间戳。
	Started      bool         `bson:"started"`       // 活动是否已执行启动行为。
	CfgID        int32        `bson:"cfg_id"`        // 活动配置编号。
	OwnerID      OwnerID      `bson:"owner_id"`      // 活动归属对象编号。
	OwnerScope   OwnerScope   `bson:"owner_scope"`   // 活动归属对象范围。
	TemplateType TemplateType `bson:"template_type"` // 活动模板类型。
	Dirty        bool         `bson:"-"`             // 是否需要存盘。
	revision     uint64       `bson:"-"`             // 内存版本号，不写入活动文档。
}

// MarkDirty 标记活动元数据已修改并递增版本号。
func (meta *ActvMeta) MarkDirty() {
	if meta != nil {
		meta.SetDirty(true)
	}
}

// IsDirty 返回活动元数据是否需要存盘。
func (meta *ActvMeta) IsDirty() bool { return meta != nil && meta.Dirty }

// Revision 返回活动元数据的内存版本号。
func (meta *ActvMeta) Revision() uint64 {
	if meta == nil {
		return 0
	}
	return meta.revision
}

// SetDirty 更新活动元数据的存盘标记。
func (meta *ActvMeta) SetDirty(dirty bool) {
	if meta == nil {
		return
	}
	if dirty {
		meta.revision++
	}
	meta.Dirty = dirty
}

// GetActvID 返回活动实例编号。
func (meta *ActvMeta) GetActvID() int64 {
	if meta == nil {
		return 0
	}
	return meta.ActvID
}

// GetStartTs 返回活动开始时间戳。
func (meta *ActvMeta) GetStartTs() int64 {
	if meta == nil {
		return 0
	}
	return meta.StartTs
}

// GetCfgID 返回活动配置编号。
func (meta *ActvMeta) GetCfgID() int32 {
	if meta == nil {
		return 0
	}
	return meta.CfgID
}

// GetOwnerID 返回活动归属对象编号。
func (meta *ActvMeta) GetOwnerID() OwnerID {
	if meta == nil {
		return 0
	}
	return meta.OwnerID
}

// GetOwnerScope 返回活动归属对象范围。
func (meta *ActvMeta) GetOwnerScope() OwnerScope {
	if meta == nil {
		return ScopeUnspecified
	}
	return meta.OwnerScope
}

// GetTemplateType 返回活动模板类型。
func (meta *ActvMeta) GetTemplateType() TemplateType {
	if meta == nil {
		return 0
	}
	return meta.TemplateType
}

// NewActv 创建活动实例、绑定模板运行时对象并收集模板注册的数据组件。
// 组件数据由调用方随后通过 LoadData 恢复，恢复完成后调用 OnLoaded。
func (as *ActvService) NewActv(meta *ActvMeta, config activityconf.ActivityConfig) *Actv {
	if as == nil || meta == nil {
		return nil
	}
	template := as.Tmp[meta.GetTemplateType()]
	if template == nil {
		tlog.Log.Error("[activity/ActvService.NewActv] 活动模板未注册, activity=%v template=%v", meta.GetActvID(), meta.GetTemplateType())
		return nil
	}
	actv := &Actv{Meta: meta}
	behavior := template.Create(actv, config)
	if behavior == nil {
		tlog.Log.Error("[activity/ActvService.NewActv] 活动模板创建运行时对象失败, activity=%v template=%v", meta.GetActvID(), meta.GetTemplateType())
		return nil
	}
	actv.Ext = behavior
	behavior.OnInit()
	if err := actv.bindComponents(); err != nil {
		tlog.Log.Error("[activity/ActvService.NewActv] 活动模板组件注册无效, activity=%v template=%v err=%v",
			meta.GetActvID(), meta.GetTemplateType(), err)
		return nil
	}
	return actv
}

// RemoveActv 从内存索引中移除活动实例，不修改存盘数据。
func (as *ActvService) RemoveActv(activityID int64) {
	if as == nil {
		return
	}
	owner, found := as.actvOwnerMgr[activityID]
	if !found {
		return
	}
	delete(as.actvOwnerMgr, activityID)
	if activities := as.actcMgr[owner]; activities != nil {
		delete(activities, activityID)
		if len(activities) == 0 {
			delete(as.actcMgr, owner)
		}
	}
	if owners := as.ownerScopeMgr[owner.scope]; owners != nil {
		owners[owner.id] = removeID(owners[owner.id], activityID)
		if len(owners[owner.id]) == 0 {
			delete(owners, owner.id)
		}
	}
	as.actvOrder = removeID(as.actvOrder, activityID)
}

func removeID(ids []int64, target int64) []int64 {
	for index, id := range ids {
		if id == target {
			return append(ids[:index], ids[index+1:]...)
		}
	}
	return ids
}

// RegTpl 注册活动模板。
func (as *ActvService) RegTpl(typ TemplateType, activityTemplate ActvTemplate) {
	if as == nil || activityTemplate == nil {
		return
	}
	as.Tmp[typ] = activityTemplate
}

// AddActv 将已创建或加载的活动实例加入归属索引。
func (as *ActvService) AddActv(actv *Actv) bool {
	if as == nil || actv == nil || actv.GetExt() == nil {
		return false
	}
	meta := actv.GetMeta()
	if meta == nil {
		tlog.Log.Error("[activity/ActvService.AddActv] 活动实例缺少元数据, actv=%T", actv)
		return false
	}
	activityID := meta.GetActvID()
	ownerID := meta.GetOwnerID()
	ownerScope := meta.GetOwnerScope()
	if activityID <= 0 || ownerID <= 0 || ownerScope == ScopeUnspecified {
		tlog.Log.Error("[activity/ActvService.AddActv] 活动元数据无效, activity=%v scope=%v owner=%v",
			activityID, ownerScope, ownerID)
		return false
	}
	owner := ownerKey{scope: ownerScope, id: ownerID}
	if _, exists := as.actvOwnerMgr[activityID]; exists {
		tlog.Log.Error("[activity/ActvService.AddActv] 活动实例编号重复, activity=%v scope=%v owner=%v",
			activityID, owner.scope, owner.id)
		return false
	}
	ownerActivities := as.actcMgr[owner]
	if ownerActivities == nil {
		ownerActivities = make(map[int64]*Actv)
		as.actcMgr[owner] = ownerActivities
	}
	ownerActivities[activityID] = actv
	as.actvOwnerMgr[activityID] = owner
	as.addOwnerActvID(owner, activityID)
	as.actvOrder = append(as.actvOrder, activityID)
	// 玩家在线期间新增的服务器/联盟活动，为已加载玩家补标记，使玩家列表组件可为其新建条目。
	if ownerScope != ScopePlayer {
		for uid := range as.loadedPlayers {
			for _, comp := range actv.playerComps {
				comp.SetLoaded(uid)
			}
		}
	}
	meta.MarkDirty()
	tlog.Log.Info("[activity/ActvService.AddActv] 注册活动实例, activity=%v scope=%v owner=%v before=%v after=%v",
		activityID, owner.scope, owner.id, false, true)
	return true
}

// GetActvByID 按全局活动实例编号查找活动。
func (as *ActvService) GetActvByID(activityID int64) *Actv {
	if as == nil || activityID <= 0 {
		return nil
	}
	owner, found := as.actvOwnerMgr[activityID]
	if !found {
		return nil
	}
	return as.actcMgr[owner][activityID]
}

// ListActvs 返回指定归属对象当前已加载的活动实例。
func (as *ActvService) ListActvs(scope OwnerScope, ownerID OwnerID) []*Actv {
	if as == nil || scope == ScopeUnspecified || ownerID <= 0 {
		return nil
	}
	activities := as.actcMgr[ownerKey{scope: scope, id: ownerID}]
	activityIDs := as.ownerScopeMgr[scope][ownerID]
	result := make([]*Actv, 0, len(activityIDs))
	for _, activityID := range activityIDs {
		if actv := activities[activityID]; actv != nil {
			result = append(result, actv)
		}
	}
	return result
}

// ListAllActvs 按注册顺序返回全部活动实例。
func (as *ActvService) ListAllActvs() []*Actv {
	if as == nil {
		return nil
	}
	result := make([]*Actv, 0, len(as.actvOrder))
	for _, activityID := range as.actvOrder {
		if actv := as.GetActvByID(activityID); actv != nil {
			result = append(result, actv)
		}
	}
	return result
}

// HandleRequest 将请求转发给指定归属对象下的活动模板。
func (as *ActvService) HandleRequest(
	scope OwnerScope,
	ownerID OwnerID,
	activityID int64,
	request interface{},
) {
	if as == nil || request == nil {
		return
	}
	actv := as.GetActvByID(activityID)
	if actv == nil {
		return
	}
	meta := actv.GetMeta()
	if meta == nil || meta.GetOwnerScope() != scope || meta.GetOwnerID() != ownerID {
		return
	}
	behavior := actv.GetExt()
	if behavior == nil {
		return
	}
	behavior.HandleRequest(request)
}

// HandleEvent 将事件按注册顺序分发给所有已加载活动实例。
func (as *ActvService) HandleEvent(event *sspb.Event) {
	if as == nil || event == nil {
		return
	}
	for _, activityID := range as.actvOrder {
		actv := as.GetActvByID(activityID)
		if actv == nil || actv.GetExt() == nil {
			continue
		}
		actv.GetExt().HandleEvent(event)
	}
}

// StartDueActivities 对达到开始时间且尚未启动的实例调用模板启动行为。
func (as *ActvService) StartDueActivities(now int64) {
	if as == nil {
		return
	}
	for _, actv := range as.ListAllActvs() {
		if actv == nil || actv.GetMeta() == nil || actv.GetExt() == nil {
			continue
		}
		meta := actv.GetMeta()
		if meta.Started || (meta.GetStartTs() > 0 && now < meta.GetStartTs()) {
			continue
		}
		actv.GetExt().OnStart()
		meta.Started = true
		meta.MarkDirty()
		tlog.Log.Info("[activity/ActvService.StartDueActivities] 活动启动, activity=%v scope=%v owner=%v",
			meta.GetActvID(), meta.GetOwnerScope(), meta.GetOwnerID())
	}
}

// HandleTicker 将生命周期定时回调分发给已启动的活动模板。
func (as *ActvService) HandleTicker() {
	if as == nil {
		return
	}
	for _, actv := range as.ListAllActvs() {
		if actv == nil || actv.GetMeta() == nil || !actv.GetMeta().Started || actv.GetExt() == nil {
			continue
		}
		actv.GetExt().OnTicker()
	}
}

// addOwnerActvID 按创建顺序记录归属对象持有的活动编号。
func (as *ActvService) addOwnerActvID(owner ownerKey, activityID int64) {
	owners := as.ownerScopeMgr[owner.scope]
	if owners == nil {
		owners = make(map[OwnerID][]int64)
		as.ownerScopeMgr[owner.scope] = owners
	}
	activityIDs := owners[owner.id]
	for _, existingID := range activityIDs {
		if existingID == activityID {
			return
		}
	}
	owners[owner.id] = append(activityIDs, activityID)
}
