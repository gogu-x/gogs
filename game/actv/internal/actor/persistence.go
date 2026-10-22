package actor

import (
	"time"

	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
)

const activitySaveTickerType = timer.TimerType(1000012)

// activityPersistenceModel adapts activity metadata to Mongo lifecycle storage.
type activityPersistenceModel struct {
	service *service.ActvService
}

func newActivityPersistenceModel(actvService *service.ActvService) *activityPersistenceModel {
	return &activityPersistenceModel{service: actvService}
}

func (*activityPersistenceModel) GetCollection() string { return "activity" }

func (*activityPersistenceModel) GetPrototype() interface{} { return (*service.ActvMeta)(nil) }

func (*activityPersistenceModel) GetIDField() string { return "_id" }

func (*activityPersistenceModel) GetKey(value interface{}) interface{} {
	meta, ok := value.(*service.ActvMeta)
	if !ok || meta == nil {
		return nil
	}
	return meta.GetActvID()
}

func (*activityPersistenceModel) Clone(value interface{}) interface{} {
	meta, ok := value.(*service.ActvMeta)
	if !ok || meta == nil {
		return nil
	}
	snapshot := *meta
	snapshot.SetDirty(false)
	return &snapshot
}

func (model *activityPersistenceModel) Range(callback func(interface{}) bool) {
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

func (*activityPersistenceModel) GetSaveInterval() time.Duration { return activityOwnerSaveInterval }

func (model *activityPersistenceModel) AfterSave(_ tree.Context, value interface{}) error {
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

func (*activityPersistenceModel) MackDirty(value interface{}) bool {
	meta, ok := value.(*service.ActvMeta)
	return ok && meta != nil && meta.IsDirty()
}

func (*activityPersistenceModel) GetTickerType() timer.TimerType { return activitySaveTickerType }

const activityDataSaveTickerType = timer.TimerType(1000013)

// activityDataPersistenceModel 将活动组件数据适配到 activity_data 集合，与元数据集合分开存放。
type activityDataPersistenceModel struct {
	service *service.ActvService
}

func newActivityDataPersistenceModel(actvService *service.ActvService) *activityDataPersistenceModel {
	return &activityDataPersistenceModel{service: actvService}
}

func (*activityDataPersistenceModel) GetCollection() string { return "activity_data" }

func (*activityDataPersistenceModel) GetPrototype() interface{} { return (*service.ActvDataDoc)(nil) }

func (*activityDataPersistenceModel) GetIDField() string { return "_id" }

func (*activityDataPersistenceModel) GetKey(value interface{}) interface{} {
	doc, ok := value.(*service.ActvDataDoc)
	if !ok || doc == nil || doc.GetID() == "" {
		return nil
	}
	return doc.GetID()
}

// Clone 直接返回快照：Range 产出的文档已在 Actor 线程内完成编码，不再被修改。
func (*activityDataPersistenceModel) Clone(value interface{}) interface{} {
	doc, ok := value.(*service.ActvDataDoc)
	if !ok || doc == nil {
		return nil
	}
	return doc
}

func (model *activityDataPersistenceModel) Range(callback func(interface{}) bool) {
	if model == nil || model.service == nil || callback == nil {
		return
	}
	for _, doc := range model.service.DirtyData() {
		if !callback(doc) {
			return
		}
	}
}

func (*activityDataPersistenceModel) GetSaveInterval() time.Duration {
	return activityOwnerSaveInterval
}

func (model *activityDataPersistenceModel) AfterSave(_ tree.Context, value interface{}) error {
	doc, ok := value.(*service.ActvDataDoc)
	if !ok || doc == nil || model == nil || model.service == nil {
		return nil
	}
	model.service.ClearDataDirty(doc)
	return nil
}

func (*activityDataPersistenceModel) MackDirty(value interface{}) bool {
	doc, ok := value.(*service.ActvDataDoc)
	return ok && doc.IsDirty()
}

func (*activityDataPersistenceModel) GetTickerType() timer.TimerType {
	return activityDataSaveTickerType
}
