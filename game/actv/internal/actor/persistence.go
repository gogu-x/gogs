package actor

import (
	"time"

	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
)

const actvSaveTickerType = timer.TimerType(1000012)

// actvPersistenceModel adapts actv metadata to Mongo lifecycle storage.
type actvPersistenceModel struct {
	service *service.ActvService
}

func newActvPersistenceModel(actvService *service.ActvService) *actvPersistenceModel {
	return &actvPersistenceModel{service: actvService}
}

// GetCollection 保留历史集合名，代码重命名不迁移已有存档。
func (*actvPersistenceModel) GetCollection() string { return "activity" }

func (*actvPersistenceModel) GetPrototype() interface{} { return (*service.ActvMeta)(nil) }

func (*actvPersistenceModel) GetIDField() string { return "_id" }

func (*actvPersistenceModel) GetKey(value interface{}) interface{} {
	meta, ok := value.(*service.ActvMeta)
	if !ok || meta == nil {
		return nil
	}
	return meta.GetActvID()
}

func (*actvPersistenceModel) Clone(value interface{}) interface{} {
	meta, ok := value.(*service.ActvMeta)
	if !ok || meta == nil {
		return nil
	}
	snapshot := *meta
	snapshot.SetDirty(false)
	return &snapshot
}

func (model *actvPersistenceModel) Range(callback func(interface{}) bool) {
	if model == nil || model.service == nil || callback == nil {
		return
	}
	for _, actv := range model.service.ListAllActvs() {
		if actv == nil || actv.GetMeta() == nil || !actv.GetMeta().IsDirty() {
			continue
		}
		if !callback(actv.GetMeta()) {
			return
		}
	}
}

func (*actvPersistenceModel) GetSaveInterval() time.Duration { return actvOwnerSaveInterval }

func (model *actvPersistenceModel) AfterSave(_ tree.Context, value interface{}) error {
	snapshot, ok := value.(*service.ActvMeta)
	if !ok || snapshot == nil || model == nil || model.service == nil {
		return nil
	}
	actv := model.service.GetActvByID(snapshot.GetActvID())
	if actv != nil && actv.GetMeta() != nil && actv.GetMeta().Revision() == snapshot.Revision() {
		actv.GetMeta().SetDirty(false)
	}
	return nil
}

func (*actvPersistenceModel) MackDirty(value interface{}) bool {
	meta, ok := value.(*service.ActvMeta)
	return ok && meta != nil && meta.IsDirty()
}

func (*actvPersistenceModel) GetTickerType() timer.TimerType { return actvSaveTickerType }

const actvDataSaveTickerType = timer.TimerType(1000013)

// actvDataPersistenceModel 将活动组件数据适配到历史组件集合，与元数据集合分开存放。
type actvDataPersistenceModel struct {
	service *service.ActvService
}

func newActvDataPersistenceModel(actvService *service.ActvService) *actvDataPersistenceModel {
	return &actvDataPersistenceModel{service: actvService}
}

// GetCollection 保留历史组件集合名，继续读取已有玩家进度。
func (*actvDataPersistenceModel) GetCollection() string { return "activity_data" }

func (*actvDataPersistenceModel) GetPrototype() interface{} { return (*service.ActvDataDoc)(nil) }

func (*actvDataPersistenceModel) GetIDField() string { return "_id" }

func (*actvDataPersistenceModel) GetKey(value interface{}) interface{} {
	doc, ok := value.(*service.ActvDataDoc)
	if !ok || doc == nil || doc.GetID() == "" {
		return nil
	}
	return doc.GetID()
}

// Clone 直接返回快照：Range 产出的文档已在 Actor 线程内完成编码，不再被修改。
func (*actvDataPersistenceModel) Clone(value interface{}) interface{} {
	doc, ok := value.(*service.ActvDataDoc)
	if !ok || doc == nil {
		return nil
	}
	return doc
}

func (model *actvDataPersistenceModel) Range(callback func(interface{}) bool) {
	if model == nil || model.service == nil || callback == nil {
		return
	}
	for _, doc := range model.service.DirtyData() {
		if !callback(doc) {
			return
		}
	}
}

func (*actvDataPersistenceModel) GetSaveInterval() time.Duration {
	return actvOwnerSaveInterval
}

func (model *actvDataPersistenceModel) AfterSave(_ tree.Context, value interface{}) error {
	doc, ok := value.(*service.ActvDataDoc)
	if !ok || doc == nil || model == nil || model.service == nil {
		return nil
	}
	model.service.ClearDataDirty(doc)
	return nil
}

func (*actvDataPersistenceModel) MackDirty(value interface{}) bool {
	doc, ok := value.(*service.ActvDataDoc)
	return ok && doc.IsDirty()
}

func (*actvDataPersistenceModel) GetTickerType() timer.TimerType {
	return actvDataSaveTickerType
}
