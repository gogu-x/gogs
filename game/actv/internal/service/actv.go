package service

import (
	"fmt"

	activityconf "github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// ActvService 管理活动模板注册表、活动实例和归属索引。
// 三个索引直接保存实例指针，遍历和查找不再经过编号二次查表；
// 返回给调用方的切片为内部只读视图，删除实例时生成新切片，不影响进行中的遍历。
type ActvService struct {
	Tmp           map[TemplateType]ActvTemplate // 已注册的活动模板。
	tplScopes     map[TemplateType]OwnerScope   // 模板注册时声明的实例归属范围。
	serverID      OwnerID                       // 本服编号，服务器活动的归属者。
	actvs         map[int64]*Actv               // 全部活动实例，键为实例编号。
	ownerActvs    map[ownerKey][]*Actv          // 每个归属者的活动实例，保持注册顺序。
	orderedActvs  []*Actv                       // 全部活动实例，保持全局注册顺序，供 tick、事件和存盘遍历。
	loadedPlayers map[uint64]bool               // 活动数据已加载的玩家，“玩家是否已加载”的唯一数据来源。
}

// ownerKey 唯一标识一个归属范围和归属对象。
type ownerKey struct {
	scope OwnerScope
	id    OwnerID
}

// NewActvService 创建本服的空活动服务。
func NewActvService(serverID uint64) *ActvService {
	return &ActvService{
		Tmp:           make(map[TemplateType]ActvTemplate),
		tplScopes:     make(map[TemplateType]OwnerScope),
		serverID:      OwnerID(serverID),
		actvs:         make(map[int64]*Actv),
		ownerActvs:    make(map[ownerKey][]*Actv),
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
	EndTs        int64        `bson:"end_ts"`        // 活动结束时间戳；0 表示不自动结束。
	Started      bool         `bson:"started"`       // 活动是否已执行启动行为。
	CfgID        int32        `bson:"cfg_id"`        // 活动配置编号。
	OwnerID      OwnerID      `bson:"owner_id"`      // 活动归属对象编号。
	OwnerScope   OwnerScope   `bson:"owner_scope"`   // 活动归属对象范围。
	TemplateType TemplateType `bson:"template_type"` // 活动模板类型。
	State        ActvState    `bson:"state"`         // 活动运行状态。
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

// GetEndTs 返回活动结束时间戳。
func (meta *ActvMeta) GetEndTs() int64 {
	if meta == nil {
		return 0
	}
	return meta.EndTs
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
	if err := actv.bindComponents(as.canCreatePlayerEntry(actv)); err != nil {
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
	actv, found := as.actvs[activityID]
	if !found {
		return
	}
	delete(as.actvs, activityID)
	owner := actv.ownerKey()
	if remaining := withoutActv(as.ownerActvs[owner], actv); len(remaining) > 0 {
		as.ownerActvs[owner] = remaining
	} else {
		delete(as.ownerActvs, owner)
	}
	as.orderedActvs = withoutActv(as.orderedActvs, actv)
}

// withoutActv 返回去掉指定实例后的新切片，不修改原切片，保证进行中的遍历安全。
func withoutActv(list []*Actv, target *Actv) []*Actv {
	result := make([]*Actv, 0, len(list))
	for _, actv := range list {
		if actv != target {
			result = append(result, actv)
		}
	}
	return result
}

// ownerKey 返回活动实例的归属键。
func (actv *Actv) ownerKey() ownerKey {
	meta := actv.GetMeta()
	return ownerKey{scope: meta.GetOwnerScope(), id: meta.GetOwnerID()}
}

// RegTpl 注册活动模板，并声明该模板创建的实例归属范围（服务器 / 联盟 / 玩家）。
func (as *ActvService) RegTpl(typ TemplateType, activityTemplate ActvTemplate, scope OwnerScope) {
	if as == nil || activityTemplate == nil {
		return
	}
	if scope != ScopePlayer && scope != ScopeServer && scope != ScopeGuild {
		tlog.Log.Error("[activity/ActvService.RegTpl] 活动模板归属范围无效，拒绝注册, template=%v scope=%v", typ, scope)
		return
	}
	as.Tmp[typ] = activityTemplate
	as.tplScopes[typ] = scope
}

// TplScope 返回模板声明的归属范围；模板未注册时返回 false。
func (as *ActvService) TplScope(typ TemplateType) (OwnerScope, bool) {
	scope, ok := as.tplScopes[typ]
	return scope, ok
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
	if _, exists := as.actvs[activityID]; exists {
		tlog.Log.Error("[activity/ActvService.AddActv] 活动实例编号重复, activity=%v scope=%v owner=%v",
			activityID, owner.scope, owner.id)
		return false
	}
	as.actvs[activityID] = actv
	as.ownerActvs[owner] = append(as.ownerActvs[owner], actv)
	as.orderedActvs = append(as.orderedActvs, actv)
	meta.MarkDirty()
	tlog.Log.Info("[activity/ActvService.AddActv] 注册活动实例, activity=%v scope=%v owner=%v",
		activityID, owner.scope, owner.id)
	return true
}

// GetActvByID 按实例编号查找活动。
func (as *ActvService) GetActvByID(activityID int64) *Actv {
	if as == nil || activityID <= 0 {
		return nil
	}
	return as.actvs[activityID]
}

// ListActvs 返回指定归属者的活动实例（按注册顺序）。返回内部只读视图，调用方不得修改。
func (as *ActvService) ListActvs(scope OwnerScope, ownerID OwnerID) []*Actv {
	if as == nil || scope == ScopeUnspecified || ownerID <= 0 {
		return nil
	}
	return as.ownerActvs[ownerKey{scope: scope, id: ownerID}]
}

// ListServerActvs 返回本服的服务器活动实例。返回内部只读视图，调用方不得修改。
func (as *ActvService) ListServerActvs() []*Actv {
	return as.ListActvs(ScopeServer, as.serverID)
}

// ListAllActvs 按全局注册顺序返回全部活动实例。返回内部只读视图，调用方不得修改。
func (as *ActvService) ListAllActvs() []*Actv {
	if as == nil {
		return nil
	}
	return as.orderedActvs
}

// HandleRequest 将玩家请求转发给活动模板，uid 透传给模板。归属校验规则：
//   - 玩家独有活动：只接受归属玩家本人的请求；
//   - 服务器活动：本服已加载的服务器活动接受任意玩家请求；
//   - 联盟活动：活动 Actor 尚无联盟成员信息，暂不接受玩家请求；
//   - 已结束活动：不再接受请求，结算由 Settler 处理。
func (as *ActvService) HandleRequest(uid uint64, activityID int64, request proto.Message) error {
	if as == nil || uid == 0 || request == nil {
		return fmt.Errorf("activity request requires a player and a message")
	}
	actv := as.GetActvByID(activityID)
	if actv == nil || actv.GetExt() == nil {
		return fmt.Errorf("activity %d not found", activityID)
	}
	if actv.IsEnded() {
		return fmt.Errorf("activity %d has ended", activityID)
	}
	actv.GetExt().HandleRequest(uid, request)
	return nil
}

// HandleEvent 将事件按注册顺序分发给所有进行中的活动实例。
func (as *ActvService) HandleEvent(event *sspb.Event) {
	if as == nil || event == nil {
		return
	}
	for _, actv := range as.orderedActvs {
		if actv.GetExt() == nil || actv.IsEnded() {
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
		if actv == nil || actv.GetMeta() == nil || actv.GetExt() == nil || actv.IsEnded() {
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
	for _, actv := range as.ListAllActvs() {
		if actv == nil || actv.GetMeta() == nil || !actv.GetMeta().Started || actv.GetExt() == nil || actv.IsEnded() {
			continue
		}
		actv.GetExt().OnTicker()
	}
}

// IsEnded 返回活动实例是否已结束。
func (actv *Actv) IsEnded() bool {
	return actv != nil && actv.GetMeta() != nil && actv.GetMeta().State == StateEnded
}

// EndActv 结束活动实例：调用模板 OnClosed（模板在此固定全服结果、结算在线玩家），
// 然后标记为已结束。实例继续常驻内存，供离线玩家上线时补结算。
func (as *ActvService) EndActv(activityID int64) bool {
	actv := as.GetActvByID(activityID)
	if actv == nil || actv.GetExt() == nil || actv.IsEnded() {
		return false
	}
	actv.GetExt().OnClosed()
	meta := actv.GetMeta()
	meta.State = StateEnded
	meta.MarkDirty()
	tlog.Log.Info("[activity/ActvService.EndActv] 活动结束, activity=%v scope=%v owner=%v",
		meta.GetActvID(), meta.GetOwnerScope(), meta.GetOwnerID())
	return true
}
