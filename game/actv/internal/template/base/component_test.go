package base_test

import (
	"os"
	"testing"
	"time"

	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	"github.com/gogu-x/gogs/game/actv/internal/trigger"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/tree/tlog"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
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
	loaded := map[uint64]bool{}
	comp.BindCreateChecker(func(uid uint64) bool { return loaded[uid] })
	if comp.Get(7) != nil {
		t.Fatal("unloaded player must return nil")
	}
	comp.MarkDirty(7)
	if len(comp.DirtyPlayers()) != 0 {
		t.Fatal("unloaded player must not become dirty")
	}
	loaded[7] = true
	comp.Get(7).Score = 5
	comp.MarkDirty(7)
	if !comp.IsDirty(7) {
		t.Fatal("loaded player should be dirty")
	}
	delete(loaded, 7)
	comp.Unload(7)
	if comp.Get(7) != nil || comp.IsDirty(7) {
		t.Fatal("unload should clear player")
	}
}

// TestDecodeFailureBlocks 校验存盘数据解码失败后不会用默认值覆盖库中数据：
// 玩家条目阻断新建和存盘，直到卸载；单条组件阻断存盘。
func TestDecodeFailureBlocks(t *testing.T) {
	bad := bson.Raw{0x01, 0x02, 0x03}

	comp := base.NewPlayerComp[memberData](2)
	comp.BindCreateChecker(func(uint64) bool { return true })
	if err := comp.Load(7, bad); err == nil {
		t.Fatal("bad data should fail to decode")
	}
	if comp.Get(7) != nil {
		t.Fatal("blocked player must not get a default entry")
	}
	comp.MarkDirty(7)
	if comp.IsDirty(7) || len(comp.DirtyPlayers()) != 0 {
		t.Fatal("blocked player must not be saved")
	}
	if comp.Get(8) == nil {
		t.Fatal("other players unaffected")
	}
	comp.Unload(7)
	if comp.Get(7) == nil {
		t.Fatal("unload should clear block")
	}

	single := base.NewComp[instData](1)
	single.Data.Round = 9
	if err := single.Load(bad); err == nil || !single.Blocked() {
		t.Fatal("single comp should be blocked on decode failure")
	}
	single.MarkDirty()
	if single.IsDirty() {
		t.Fatal("blocked single comp must not be saved")
	}

	// 服务层：玩家已加载，但其条目解码失败，模板拿不到默认条目，也不会产生存盘快照。
	svc := service.NewActvService(1)
	svc.RegTpl(testTemplate, &testBehavior{}, service.ScopeServer)
	_, server := newTestActv(t, svc, 10, service.ScopeServer, 1)
	svc.SetPlayerLoaded(7)
	svc.ApplyDataDocs([]service.ActvDataDoc{{ActvID: 10, Comp: 2, OwnerScope: service.ScopePlayer, OwnerID: 7, Data: bad}}, 0)
	if server.member.Get(7) != nil || len(svc.PlayerDirtyData(7, 0)) != 0 {
		t.Fatal("decode failure must not create or save default entry")
	}
}

func TestServiceLoadSaveUnload(t *testing.T) {
	svc := service.NewActvService(1)
	svc.RegTpl(testTemplate, &testBehavior{}, service.ScopeServer)
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
	playerDocs := svc.PlayerDirtyData(7, 0)
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

	svc.UnloadPlayer(7, 0)
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
	svc := service.NewActvService(1)
	svc.RegTpl(testTemplate, &dupBehavior{}, service.ScopeServer)
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
	svc := service.NewActvService(1)
	svc.RegTpl(testTemplate, &testBehavior{}, service.ScopeServer)
	serverActv, server := newTestActv(t, svc, 10, service.ScopeServer, 1)
	_, player := newTestActv(t, svc, 20, service.ScopePlayer, 7)

	svc.SetPlayerLoaded(7)
	if err := serverActv.LoadData(&service.ActvDataDoc{
		ActvID: 10, Comp: 2, OwnerScope: service.ScopePlayer, OwnerID: 7, Data: mustRaw(t, memberData{Score: 5}),
	}); err != nil {
		t.Fatal(err)
	}
	svc.NotifyPlayerLoaded(7, 0)
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
	svc.NotifyPlayerUnload(7, 0)
	var found bool
	for _, doc := range svc.PlayerDirtyData(7, 0) {
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
	svc.UnloadPlayer(7, 0)
	if svc.IsPlayerLoaded(7) {
		t.Fatal("player should be unloaded")
	}
	if _, again := newTestActv(t, svc, 40, service.ScopeServer, 1); again.member.Get(7) != nil {
		t.Fatal("unloaded player must not be marked on new activity")
	}
}

const settleTemplate service.TemplateType = 98

type settleData struct {
	Score   int32 `bson:"score"`
	Settled bool  `bson:"settled,omitempty"`
}

// settleBehavior 实现 Settler 的测试模块，记录 tick、上线通知和结算调用。
type settleBehavior struct {
	base.BaseBehavior
	member  *base.PlayerComp[settleData]
	ticks   int
	logins  int
	settled []uint64
}

func (*settleBehavior) Create(actv *service.Actv, cfg conf.ActivityConfig) service.ActvBehavior {
	b := &settleBehavior{}
	b.Bind(actv, cfg)
	return b
}

func (b *settleBehavior) OnInit() {
	b.member = base.NewPlayerComp[settleData](2)
	b.AddPlayerComp(b.member)
}

func (b *settleBehavior) NeedSettle(uid uint64) bool {
	p := b.member.Get(uid)
	return p != nil && !p.Settled && p.Score > 0
}

func (b *settleBehavior) Settle(uid uint64) {
	p := b.member.Get(uid)
	b.settled = append(b.settled, uid)
	p.Settled = true
	b.member.MarkDirty(uid)
}

func (b *settleBehavior) OnClosed() {
	b.member.Range(func(uid uint64, _ *settleData) bool {
		if b.NeedSettle(uid) {
			b.Settle(uid)
		}
		return true
	})
}

func (b *settleBehavior) OnTicker()             { b.ticks++ }
func (b *settleBehavior) OnPlayerLoaded(uint64) { b.logins++ }

func memberDoc(t *testing.T, actvID int64, uid uint64, data any) service.ActvDataDoc {
	t.Helper()
	return service.ActvDataDoc{ActvID: actvID, Comp: 2, OwnerScope: service.ScopePlayer, OwnerID: service.OwnerID(uid), Data: mustRaw(t, data)}
}

func hasEntry(b *settleBehavior, uid uint64) bool {
	found := false
	b.member.Range(func(id uint64, _ *settleData) bool {
		found = found || id == uid
		return true
	})
	return found
}

// TestEndedSettle 校验已结束活动：跳过 tick/上线通知、不新建条目；
// 玩家上线后需要结算的执行 Settle、存盘后卸载，不需要的立即卸载。
func TestEndedSettle(t *testing.T) {
	svc := service.NewActvService(1)
	svc.RegTpl(settleTemplate, &settleBehavior{}, service.ScopeServer)
	meta := &service.ActvMeta{ActvID: 50, CfgID: 1, OwnerID: 1, OwnerScope: service.ScopeServer, TemplateType: settleTemplate, Started: true}
	actv := svc.NewActv(meta, conf.ActivityConfig{})
	if actv == nil || !svc.AddActv(actv) {
		t.Fatal("create activity failed")
	}
	b := actv.GetExt().(*settleBehavior)

	// 在线玩家 1 在活动结束时由 OnClosed 结算。
	svc.SetPlayerLoaded(1)
	svc.ApplyDataDocs([]service.ActvDataDoc{memberDoc(t, 50, 1, settleData{Score: 3})}, 0)
	if !svc.EndActv(50) || !actv.IsEnded() || svc.EndActv(50) {
		t.Fatal("EndActv should end once")
	}
	if len(b.settled) != 1 || b.settled[0] != 1 || !b.member.IsDirty(1) {
		t.Fatalf("online player should be settled on close, got %v", b.settled)
	}

	svc.HandleTicker()
	svc.SetPlayerLoaded(9)
	svc.NotifyPlayerLoaded(9, 0)
	if b.ticks != 0 || b.logins != 0 {
		t.Fatalf("ended activity must skip tick/hooks, ticks=%d logins=%d", b.ticks, b.logins)
	}
	if b.member.Get(9) != nil {
		t.Fatal("ended activity must not create entries")
	}

	// 玩家 7 未结算：结算后存盘再卸载。
	svc.ApplyDataDocs([]service.ActvDataDoc{memberDoc(t, 50, 7, settleData{Score: 5})}, 0)
	docs, settled := svc.SettleEndedPlayer(7, 0)
	if len(docs) != 1 || docs[0].OwnerID != 7 || len(settled) != 1 || settled[0] != 50 {
		t.Fatalf("settle docs=%+v settled=%v", docs, settled)
	}
	var saved settleData
	if err := bson.Unmarshal(docs[0].Data, &saved); err != nil || !saved.Settled {
		t.Fatalf("snapshot should contain settled flag, got %+v err=%v", saved, err)
	}
	svc.UnloadEndedPlayer(7, settled)
	if !hasEntry(b, 7) {
		t.Fatal("dirty entry must stay until saved")
	}
	svc.ClearDataDirty(docs[0])
	svc.UnloadEndedPlayer(7, settled)
	if hasEntry(b, 7) {
		t.Fatal("saved entry should be unloaded")
	}

	// 玩家 8 已结算：不调用 Settle，立即卸载。
	svc.ApplyDataDocs([]service.ActvDataDoc{memberDoc(t, 50, 8, settleData{Score: 5, Settled: true})}, 0)
	if docs, _ := svc.SettleEndedPlayer(8, 0); len(docs) != 0 || hasEntry(b, 8) {
		t.Fatal("settled player should be unloaded without saving")
	}
	if len(b.settled) != 2 {
		t.Fatalf("settle calls = %v, want [1 7]", b.settled)
	}
}

// TestEndedWithoutSettler 未实现 Settler 的已结束活动，玩家条目加载后立即卸载。
func TestEndedWithoutSettler(t *testing.T) {
	svc := service.NewActvService(1)
	svc.RegTpl(testTemplate, &testBehavior{}, service.ScopeServer)
	_, b := newTestActv(t, svc, 60, service.ScopeServer, 1)
	svc.EndActv(60)
	svc.ApplyDataDocs([]service.ActvDataDoc{memberDoc(t, 60, 7, memberData{Score: 1})}, 0)
	if docs, settled := svc.SettleEndedPlayer(7, 0); len(docs) != 0 || len(settled) != 0 {
		t.Fatal("no settle expected")
	}
	b.member.Range(func(uint64, *memberData) bool {
		t.Fatal("entry should be unloaded")
		return false
	})
}

// reqBehavior 记录收到请求的玩家，并按 uid 写入玩家列表组件。
type reqBehavior struct {
	base.BaseBehavior
	member *base.PlayerComp[memberData]
	uids   []uint64
}

const reqTemplate service.TemplateType = 97

func (*reqBehavior) Create(actv *service.Actv, cfg conf.ActivityConfig) service.ActvBehavior {
	b := &reqBehavior{}
	b.Bind(actv, cfg)
	return b
}

func (b *reqBehavior) OnInit() {
	b.member = base.NewPlayerComp[memberData](2)
	b.AddPlayerComp(b.member)
}

func (b *reqBehavior) HandleRequest(uid uint64, _ proto.Message) {
	b.uids = append(b.uids, uid)
	if p := b.member.Get(uid); p != nil {
		p.Score++
		b.member.MarkDirty(uid)
	}
}

// TestHandleRequestRouting 校验请求路由：服务器活动接受任意玩家并透传 uid；
// 玩家独有活动只接受本人；联盟活动和已结束活动拒绝。
func TestHandleRequestRouting(t *testing.T) {
	svc := service.NewActvService(1)
	svc.RegTpl(reqTemplate, &reqBehavior{}, service.ScopeServer)
	newReq := func(id int64, scope service.OwnerScope, owner service.OwnerID) *reqBehavior {
		meta := &service.ActvMeta{ActvID: id, CfgID: 1, OwnerID: owner, OwnerScope: scope, TemplateType: reqTemplate}
		actv := svc.NewActv(meta, conf.ActivityConfig{})
		if actv == nil || !svc.AddActv(actv) {
			t.Fatalf("create activity %d failed", id)
		}
		return actv.GetExt().(*reqBehavior)
	}
	server := newReq(10, service.ScopeServer, 1)
	player := newReq(20, service.ScopePlayer, 7)
	newReq(30, service.ScopeGuild, 5)
	svc.SetPlayerLoaded(7)
	svc.SetPlayerLoaded(8)
	req := &pb_activity.JoinActivityReq{}

	for _, uid := range []uint64{7, 8} {
		if err := svc.HandleRequest(uid, 10, req); err != nil {
			t.Fatalf("server activity should accept player %d: %v", uid, err)
		}
	}
	if len(server.uids) != 2 || server.uids[0] != 7 || server.uids[1] != 8 {
		t.Fatalf("uid not passed through, got %v", server.uids)
	}
	if server.member.Get(8).Score != 1 || !server.member.IsDirty(8) {
		t.Fatal("server activity should write player entry by uid")
	}

	if err := svc.HandleRequest(7, 20, req); err != nil || len(player.uids) != 1 {
		t.Fatalf("owner request should pass: %v", err)
	}
	if err := svc.HandleRequest(8, 20, req); err == nil || len(player.uids) != 1 {
		t.Fatal("other player's request to player activity must be rejected")
	}
	if err := svc.HandleRequest(7, 30, req); err == nil {
		t.Fatal("guild activity must reject player requests for now")
	}
	if err := svc.HandleRequest(7, 99, req); err == nil {
		t.Fatal("unknown activity must be rejected")
	}
	if err := svc.HandleRequest(0, 10, req); err == nil {
		t.Fatal("request without uid must be rejected")
	}
	svc.EndActv(10)
	if err := svc.HandleRequest(7, 10, req); err == nil || len(server.uids) != 2 {
		t.Fatal("ended activity must reject requests")
	}
}

// initData 实现 Initializer 的测试数据结构。
type initData struct {
	Level int32           `bson:"level"`
	Items map[int32]int32 `bson:"items,omitempty"`
	Tags  map[string]bool `bson:"tags,omitempty"`
}

func (d *initData) Init() {
	if d.Items == nil {
		d.Items = make(map[int32]int32)
	}
	if d.Tags == nil {
		d.Tags = make(map[string]bool)
	}
}

// TestDataInit 校验 Init 在新建、加载、字段缺失时都会补齐容器，且不覆盖已加载的值。
func TestDataInit(t *testing.T) {
	// 单条组件：新建即初始化。
	single := base.NewComp[initData](1)
	if single.Data.Items == nil || single.Data.Tags == nil {
		t.Fatal("NewComp should init containers")
	}
	single.Data.Items[1] = 1 // 不会 panic

	// 单条组件：加载已有数据保留值，缺失字段（tags）被补齐。
	raw := mustRaw(t, bson.M{"level": 3, "items": bson.M{"5": 9}})
	if err := single.Load(raw); err != nil {
		t.Fatal(err)
	}
	if single.Data.Level != 3 || single.Data.Items[5] != 9 || single.Data.Tags == nil {
		t.Fatalf("load should keep values and fill missing containers, got %+v", single.Data)
	}

	// 玩家条目：Get 新建即初始化。
	comp := base.NewPlayerComp[initData](2)
	comp.BindCreateChecker(func(uint64) bool { return true })
	if p := comp.Get(7); p == nil || p.Items == nil || p.Tags == nil {
		t.Fatal("new player entry should be initialized")
	}

	// 玩家条目：加载缺失字段的数据后补齐，已有值保留。
	if err := comp.Load(8, mustRaw(t, bson.M{"level": 2, "tags": bson.M{"vip": true}})); err != nil {
		t.Fatal(err)
	}
	p := comp.Get(8)
	if p.Level != 2 || !p.Tags["vip"] || p.Items == nil {
		t.Fatalf("loaded entry should keep values and fill missing containers, got %+v", p)
	}
}

// TestOwnerIndex 校验归属索引：增删后 actvs / ownerActvs / orderedActvs 一致；
// 上线相关遍历只触及本服活动和玩家所属联盟活动，不触及其他联盟和其他玩家的活动。
func TestOwnerIndex(t *testing.T) {
	svc := service.NewActvService(1)
	svc.RegTpl(testTemplate, &testBehavior{}, service.ScopeServer)
	_, server := newTestActv(t, svc, 10, service.ScopeServer, 1)
	_, otherServer := newTestActv(t, svc, 11, service.ScopeServer, 2) // 非本服
	_, guildA := newTestActv(t, svc, 20, service.ScopeGuild, 100)
	_, guildB := newTestActv(t, svc, 21, service.ScopeGuild, 200)
	newTestActv(t, svc, 30, service.ScopePlayer, 7)
	newTestActv(t, svc, 31, service.ScopePlayer, 7)
	newTestActv(t, svc, 40, service.ScopePlayer, 8)

	if got := len(svc.ListAllActvs()); got != 7 {
		t.Fatalf("all = %d, want 7", got)
	}
	if got := svc.ListServerActvs(); len(got) != 1 || got[0].GetMeta().GetActvID() != 10 {
		t.Fatalf("server actvs = %v", got)
	}
	if got := svc.ListActvs(service.ScopePlayer, 7); len(got) != 2 || got[0].GetMeta().GetActvID() != 30 {
		t.Fatal("player index should keep order")
	}

	// 上线通知：玩家 7 属于联盟 100。
	svc.NotifyPlayerLoaded(7, 100)
	if len(server.events) != 1 || len(guildA.events) != 1 || len(guildB.events) != 0 || len(otherServer.events) != 0 {
		t.Fatalf("notify scope wrong: server=%v guildA=%v guildB=%v other=%v",
			server.events, guildA.events, guildB.events, otherServer.events)
	}

	// 其他联盟活动的玩家条目不挂入。
	svc.SetPlayerLoaded(7)
	svc.ApplyDataDocs([]service.ActvDataDoc{
		{ActvID: 20, Comp: 2, OwnerScope: service.ScopePlayer, OwnerID: 7, Data: mustRaw(t, memberData{Score: 1})},
		{ActvID: 21, Comp: 2, OwnerScope: service.ScopePlayer, OwnerID: 7, Data: mustRaw(t, memberData{Score: 1})},
	}, 100)
	if guildA.member.Get(7) == nil || guildA.member.Get(7).Score != 1 {
		t.Fatal("own guild entry should load")
	}
	hasB := false
	guildB.member.Range(func(uint64, *memberData) bool { hasB = true; return false })
	if hasB {
		t.Fatal("other guild entry must be skipped")
	}

	// 释放：移除玩家 7 的独有活动，卸载相关共享活动中的条目，索引同步。
	svc.UnloadPlayer(7, 100)
	if len(svc.ListActvs(service.ScopePlayer, 7)) != 0 || svc.GetActvByID(30) != nil || svc.GetActvByID(31) != nil {
		t.Fatal("player activities should be removed")
	}
	if got := len(svc.ListAllActvs()); got != 5 {
		t.Fatalf("all after unload = %d, want 5", got)
	}
	hasA := false
	guildA.member.Range(func(uint64, *memberData) bool { hasA = true; return false })
	if hasA {
		t.Fatal("guild entry should be unloaded")
	}
	if len(svc.ListActvs(service.ScopePlayer, 8)) != 1 {
		t.Fatal("other player unaffected")
	}

	// 遍历过程中删除不影响当前遍历。
	all := svc.ListAllActvs()
	svc.RemoveActv(40)
	if len(all) != 5 || len(svc.ListAllActvs()) != 4 || svc.ListActvs(service.ScopePlayer, 8) != nil {
		t.Fatal("remove should produce new slices and clean empty owner")
	}
}

// TestCreateDueServerActvs 校验按时间规则创建服务器活动：只创建有效期内的轮次，重复检查不重复创建，
// 到结束时间后结束；模板未注册或声明非服务器归属的配置被过滤。
func TestCreateDueServerActvs(t *testing.T) {
	svc := service.NewActvService(1)
	svc.RegTpl(testTemplate, &testBehavior{}, service.ScopeServer)
	svc.RegTpl(reqTemplate, &reqBehavior{}, service.ScopePlayer)
	if scope, ok := svc.TplScope(testTemplate); !ok || scope != service.ScopeServer {
		t.Fatal("template scope should be recorded")
	}
	svc.RegTpl(settleTemplate, &settleBehavior{}, service.ScopeUnspecified)
	if _, ok := svc.TplScope(settleTemplate); ok {
		t.Fatal("invalid scope must be rejected")
	}

	loc := time.Local
	newPlan := func(id, tpl int32) *trigger.Plan {
		plan, err := trigger.NewPlan(&glconf.ActvOnlineCfg{ID: id, Available: true, Type: tpl, TriggerType: 1,
			TriggerVal: "2026-10-01 00:00:00", Reopen: true, TriggerRept: "7d", Duration: "3d"}, 0, loc)
		if err != nil {
			t.Fatal(err)
		}
		return plan
	}
	plans := svc.FilterServerPlans([]*trigger.Plan{
		newPlan(100, int32(testTemplate)), // 服务器归属
		newPlan(200, int32(reqTemplate)),  // 玩家归属，过滤
		newPlan(300, 999),                 // 未注册，过滤
	})
	if len(plans) != 1 || plans[0].CfgID != 100 {
		t.Fatalf("filtered plans = %d", len(plans))
	}

	start, _ := time.ParseInLocation("2006-01-02 15:04:05", "2026-10-15 00:00:00", loc) // 第 3 轮开始
	now := start.Unix() + 3600
	created := svc.CreateDueServerActs(plans, now)
	if len(created) != 3 {
		t.Fatalf("created = %d, want 3 (including settlement rounds)", len(created))
	}
	meta := created[2].GetMeta()
	if meta.GetActvID() != plans[0].InstanceID(3) || meta.GetOwnerScope() != service.ScopeServer || meta.GetOwnerID() != 1 ||
		meta.GetStartTs() != start.Unix() || meta.GetEndTs() != start.Unix()+3*86400 || !meta.IsDirty() {
		t.Fatalf("meta = %+v", meta)
	}
	if b := created[2].GetExt().(*testBehavior); !b.loaded {
		t.Fatal("OnLoaded should be called on new instance")
	}
	if len(svc.CreateDueServerActs(plans, now+60)) != 0 {
		t.Fatal("existing instance must not be recreated")
	}

	svc.EndDueActivities(meta.GetEndTs() - 1)
	if created[2].IsEnded() {
		t.Fatal("should not end before EndTs")
	}
	svc.EndDueActivities(meta.GetEndTs())
	if !created[2].IsEnded() {
		t.Fatal("should end at EndTs")
	}
	// 十四天结算期内不重复创建同一轮；下一轮到期时创建新实例。
	next := svc.CreateDueServerActs(plans, start.Unix()+7*86400)
	if len(next) != 1 || next[0].GetMeta().GetActvID() != plans[0].InstanceID(4) {
		t.Fatal("next round should be created")
	}
}
