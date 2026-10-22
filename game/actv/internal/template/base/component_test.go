package base_test

import (
	"os"
	"testing"

	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	"github.com/gogu-x/tree/tlog"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMain(m *testing.M) {
	logger, err := tlog.New("error", "", tlog.NoFlags)
	if err != nil {
		panic(err)
	}
	tlog.Log = logger
	os.Exit(m.Run())
}

const testTemplate service.TemplateType = 99

type instData struct {
	Round int32 `bson:"round"`
}

type memberData struct {
	Score int32 `bson:"score"`
}

type testBehavior struct {
	base.BaseBehavior
	inst   *base.Comp[instData]
	member *base.PlayerComp[memberData]
	loaded bool
	// 钩子调用记录。
	events    []string
	seenScore int32
}

func (*testBehavior) Create(actv *service.Actv, cfg conf.ActivityConfig) service.ActvBehavior {
	b := &testBehavior{}
	b.Bind(actv, cfg)
	return b
}

func (b *testBehavior) OnInit() {
	b.inst = base.NewComp[instData](1)
	b.AddComp(b.inst)
	b.member = base.NewPlayerComp[memberData](2)
	b.AddPlayerComp(b.member)
}

func (b *testBehavior) OnLoaded() { b.loaded = true }

func (b *testBehavior) OnPlayerLoaded(uid uint64) {
	b.events = append(b.events, "loaded")
	// 钩子内可直接使用已恢复的玩家条目。
	if p := b.member.Get(uid); p != nil {
		b.seenScore = p.Score
	}
}

func (b *testBehavior) OnPlayerUnload(uid uint64) {
	b.events = append(b.events, "unload")
	if p := b.member.Get(uid); p != nil {
		p.Score += 100
		b.member.MarkDirty(uid)
	}
}

func newTestActv(t *testing.T, svc *service.ActvService, id int64, scope service.OwnerScope, owner service.OwnerID) (*service.Actv, *testBehavior) {
	t.Helper()
	meta := &service.ActvMeta{ActvID: id, CfgID: 1, OwnerID: owner, OwnerScope: scope, TemplateType: testTemplate}
	actv := svc.NewActv(meta, conf.ActivityConfig{})
	if actv == nil || !svc.AddActv(actv) {
		t.Fatalf("create activity %d failed", id)
	}
	return actv, actv.GetExt().(*testBehavior)
}

func mustRaw(t *testing.T, v any) bson.Raw {
	t.Helper()
	raw, err := bson.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCompRoundTrip(t *testing.T) {
	comp := base.NewComp[instData](1)
	if comp.IsDirty() {
		t.Fatal("new comp should be clean")
	}
	comp.Data.Round = 3
	comp.MarkDirty()
	raw, err := comp.Save()
	if err != nil {
		t.Fatal(err)
	}
	other := base.NewComp[instData](1)
	if err := other.Load(raw); err != nil || other.Data.Round != 3 {
		t.Fatalf("load got %+v err=%v", other.Data, err)
	}
	rev := comp.Revision()
	comp.MarkDirty()
	comp.ClearDirty(rev)
	if !comp.IsDirty() {
		t.Fatal("stale revision must not clear dirty")
	}
	comp.ClearDirty(comp.Revision())
	if comp.IsDirty() {
		t.Fatal("current revision should clear dirty")
	}
}

func TestPlayerCompGetRequiresLoaded(t *testing.T) {
	comp := base.NewPlayerComp[memberData](2)
	if comp.Get(7) != nil {
		t.Fatal("unloaded player must return nil")
	}
	comp.MarkDirty(7)
	if len(comp.DirtyPlayers()) != 0 {
		t.Fatal("unloaded player must not become dirty")
	}
	comp.SetLoaded(7)
	comp.Get(7).Score = 5
	comp.MarkDirty(7)
	if !comp.IsDirty(7) {
		t.Fatal("loaded player should be dirty")
	}
	comp.Unload(7)
	if comp.Get(7) != nil || comp.IsDirty(7) {
		t.Fatal("unload should clear player")
	}
}

func TestServiceLoadSaveUnload(t *testing.T) {
	svc := service.NewActvService()
	svc.RegTpl(testTemplate, &testBehavior{})
	serverActv, server := newTestActv(t, svc, 10, service.ScopeServer, 1)
	playerActv, player := newTestActv(t, svc, 20, service.ScopePlayer, 7)

	// 单条数据归属者与活动一致；服务器活动的玩家列表条目归属者为玩家。
	docs := []*service.ActvDataDoc{
		{ActvID: 10, Comp: 1, OwnerScope: service.ScopeServer, OwnerID: 1, Data: mustRaw(t, instData{Round: 4})},
		{ActvID: 20, Comp: 1, OwnerScope: service.ScopePlayer, OwnerID: 7, Data: mustRaw(t, instData{Round: 9})},
		{ActvID: 10, Comp: 2, OwnerScope: service.ScopePlayer, OwnerID: 7, Data: mustRaw(t, memberData{Score: 11})},
	}
	for _, doc := range docs {
		if err := svc.GetActvByID(doc.ActvID).LoadData(doc); err != nil {
			t.Fatalf("load %+v: %v", doc, err)
		}
	}
	if err := serverActv.LoadData(&service.ActvDataDoc{ActvID: 10, Comp: 1, OwnerScope: service.ScopePlayer, OwnerID: 7}); err == nil {
		t.Fatal("server single comp with player owner should be rejected")
	}
	if err := serverActv.LoadData(&service.ActvDataDoc{ActvID: 10, Comp: 2, OwnerScope: service.ScopeGuild, OwnerID: 7}); err == nil {
		t.Fatal("player comp with non-player owner should be rejected")
	}
	if err := playerActv.LoadData(&service.ActvDataDoc{ActvID: 20, Comp: 3, OwnerScope: service.ScopePlayer, OwnerID: 7}); err == nil {
		t.Fatal("unknown comp should be rejected")
	}
	if server.inst.Data.Round != 4 || player.inst.Data.Round != 9 || server.member.Get(7).Score != 11 {
		t.Fatal("component data not restored")
	}
	if len(svc.DirtyData()) != 0 {
		t.Fatal("restored data should be clean")
	}

	server.inst.Data.Round++
	server.inst.MarkDirty()
	server.member.Get(7).Score++
	server.member.MarkDirty(7)
	player.inst.MarkDirty()
	if got := len(svc.DirtyData()); got != 3 {
		t.Fatalf("dirty data count = %d, want 3", got)
	}
	playerDocs := svc.PlayerDirtyData(7)
	if len(playerDocs) != 2 {
		t.Fatalf("player dirty data count = %d, want 2", len(playerDocs))
	}
	for _, doc := range playerDocs {
		if doc.OwnerScope != service.ScopePlayer || doc.OwnerID != 7 || doc.GetID() != service.DataDocID(doc.ActvID, doc.Comp, 7) {
			t.Fatalf("unexpected player doc %+v", doc)
		}
		svc.ClearDataDirty(doc)
	}
	serverDocs := svc.DirtyData()
	if len(serverDocs) != 1 {
		t.Fatalf("after clear dirty count = %d, want 1 (server instance)", len(serverDocs))
	}
	if doc := serverDocs[0]; doc.OwnerScope != service.ScopeServer || doc.OwnerID != 1 || doc.GetID() != "10_1_1" {
		t.Fatalf("unexpected server doc %+v", doc)
	}

	svc.UnloadPlayer(7)
	if svc.GetActvByID(20) != nil {
		t.Fatal("player activity should be removed")
	}
	if svc.GetActvByID(10) == nil || server.member.Get(7) != nil {
		t.Fatal("server activity kept, player entry unloaded")
	}
	if len(svc.ListActvs(service.ScopePlayer, 7)) != 0 || len(svc.ListAllActvs()) != 1 {
		t.Fatal("indexes not cleaned")
	}
}

func TestDuplicateCompIDRejected(t *testing.T) {
	svc := service.NewActvService()
	svc.RegTpl(testTemplate, &dupBehavior{})
	meta := &service.ActvMeta{ActvID: 1, CfgID: 1, OwnerID: 1, OwnerScope: service.ScopeServer, TemplateType: testTemplate}
	if svc.NewActv(meta, conf.ActivityConfig{}) != nil {
		t.Fatal("duplicate component id should fail")
	}
}

type dupBehavior struct{ base.BaseBehavior }

func (*dupBehavior) Create(actv *service.Actv, cfg conf.ActivityConfig) service.ActvBehavior {
	b := &dupBehavior{}
	b.Bind(actv, cfg)
	return b
}

func (b *dupBehavior) OnInit() {
	b.AddComp(base.NewComp[instData](1))
	b.AddPlayerComp(base.NewPlayerComp[memberData](1))
}

// TestDataDocBSON 校验存盘文档字段：私有字段不落库，归属者字段与 ActvMeta 同名同类型，可按 int32/int64 过滤。
func TestDataDocBSON(t *testing.T) {
	doc := &service.ActvDataDoc{
		ID: service.DataDocID(10, 2, 7), ActvID: 10, Comp: 2,
		OwnerScope: service.ScopePlayer, OwnerID: 7, Data: mustRaw(t, memberData{Score: 3}),
	}
	raw := mustRaw(t, doc)
	var plain bson.M
	if err := bson.Unmarshal(raw, &plain); err != nil {
		t.Fatal(err)
	}
	if len(plain) != 6 {
		t.Fatalf("unexpected fields %v", plain)
	}
	if scope, ok := plain["owner_scope"].(int32); !ok || scope != int32(service.ScopePlayer) {
		t.Fatalf("owner_scope encoded as %T %v", plain["owner_scope"], plain["owner_scope"])
	}
	if owner, ok := plain["owner_id"].(int64); !ok || owner != 7 {
		t.Fatalf("owner_id encoded as %T %v", plain["owner_id"], plain["owner_id"])
	}
	var decoded service.ActvDataDoc
	if err := bson.Unmarshal(raw, &decoded); err != nil || decoded.OwnerID != 7 || decoded.GetID() != "10_2_7" {
		t.Fatalf("decoded %+v err=%v", decoded, err)
	}
}

// TestPlayerHooks 校验上线/卸载钩子只通知服务器/联盟活动，卸载钩子中的修改进入存盘快照，
// 以及玩家在线期间新增的服务器活动会自动补加载标记。
func TestPlayerHooks(t *testing.T) {
	svc := service.NewActvService()
	svc.RegTpl(testTemplate, &testBehavior{})
	serverActv, server := newTestActv(t, svc, 10, service.ScopeServer, 1)
	_, player := newTestActv(t, svc, 20, service.ScopePlayer, 7)

	svc.SetPlayerLoaded(7)
	if err := serverActv.LoadData(&service.ActvDataDoc{
		ActvID: 10, Comp: 2, OwnerScope: service.ScopePlayer, OwnerID: 7, Data: mustRaw(t, memberData{Score: 5}),
	}); err != nil {
		t.Fatal(err)
	}
	svc.NotifyPlayerLoaded(7)
	if len(server.events) != 1 || server.events[0] != "loaded" || server.seenScore != 5 {
		t.Fatalf("server hook events=%v score=%d", server.events, server.seenScore)
	}
	if len(player.events) != 0 {
		t.Fatalf("player-owned activity must not receive hooks, got %v", player.events)
	}

	// 玩家在线期间新增服务器活动：玩家列表组件可直接为在线玩家新建条目。
	_, late := newTestActv(t, svc, 30, service.ScopeServer, 1)
	if late.member.Get(7) == nil {
		t.Fatal("new shared activity should mark loaded players")
	}
	if late.member.Get(8) != nil {
		t.Fatal("offline player must stay unloaded")
	}

	// 卸载钩子在取快照前调用，修改随释放存盘。
	svc.NotifyPlayerUnload(7)
	var found bool
	for _, doc := range svc.PlayerDirtyData(7) {
		if doc.ActvID != 10 || doc.Comp != 2 {
			continue
		}
		var saved memberData
		if err := bson.Unmarshal(doc.Data, &saved); err != nil {
			t.Fatal(err)
		}
		found = saved.Score == 105
	}
	if !found {
		t.Fatal("unload hook change should be included in release snapshot")
	}
	svc.UnloadPlayer(7)
	if svc.IsPlayerLoaded(7) {
		t.Fatal("player should be unloaded")
	}
	if _, again := newTestActv(t, svc, 40, service.ScopeServer, 1); again.member.Get(7) != nil {
		t.Fatal("unloaded player must not be marked on new activity")
	}
}
