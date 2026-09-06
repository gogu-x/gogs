package internal

type Db interface {
	Loader()
	Save()
	TableName() string
	OnTimer() int64
}

// DbMgr 注册到db管理器中，负责统一调度，启动时加载，关服全部存盘，每个实现的按子时间定时存盘
type DbMgr struct {
	dbs []Db
}

func NewDbMgr() *DbMgr {
	return &DbMgr{dbs: make([]Db, 0)}
}

// OnLoader 统一加载 将play 上面的数据统一调度加载到play上面只限注册到db里面的，这里应该要按类型注册，查找然后绑定数据
func (mgr *DbMgr) OnLoader(play *Play) {

}

func (mgr *DbMgr) Register(d Db) {
	mgr.dbs = append(mgr.dbs, d)
}

type DbLocal struct {
}

func NewDbLocal() *DbLocal {
	return &DbLocal{}
}

// Loader 加载数据
func (dl *DbLocal) Loader() {

}

// Save 负责保存数据
func (dl *DbLocal) Save() {

}

func (dl *DbLocal) TableName() string {
	return ""
}

func (dl *DbLocal) OnTimer() int64 {
	return 1000
}
