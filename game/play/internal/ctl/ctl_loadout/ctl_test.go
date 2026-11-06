package ctl_loadout

import (
	"testing"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/player"
	"github.com/gogu-x/gogs/game/play/internal/player/loadouttest"
	"github.com/gogu-x/gogs/game/play/internal/player/role"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_souljade"
	"github.com/gogu-x/gogs/pb/cspb/pb_talent"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type captureGate struct {
	tree.Context
	last  proto.Message
	uid   uint64
	sends int
}

func (gate *captureGate) Lookup(name string) (tree.PID, bool) {
	return tree.PID{ID: 1}, name == def.GameGate
}

func (gate *captureGate) Send(_ tree.PID, value interface{}) bool {
	push, ok := value.(*ipb.PushToMsg)
	if !ok {
		return false
	}
	message, ok := push.Msg.(proto.Message)
	if !ok {
		return false
	}
	gate.last, gate.uid = message, push.UID
	gate.sends++
	return true
}

func (gate *captureGate) reply(t *testing.T) proto.Message {
	t.Helper()
	if gate.last == nil || gate.uid != 42 {
		t.Fatal("handler did not send a reply to the authenticated player")
	}
	return gate.last
}

func loadoutContext(t *testing.T) (*core.Context, *player.Player, *captureGate) {
	t.Helper()
	loadouttest.Load(t)
	tlog.NewLog("", 0)
	p := player.NewPlayerData(42)
	p.GetRoleMgr().SetCharacter(&role.Role{ID: "role-42", ConfigID: 1, Level: 1})
	p.SetDirty(false)
	gate := &captureGate{}
	return &core.Context{Actor: &core.PlayerActor{UID: 42, Player: p}, TreeCtx: gate}, p, gate
}

func requireStatus(t *testing.T, message proto.Message, expected pb_common.ErrCode) {
	t.Helper()
	reflected := message.ProtoReflect()
	field := reflected.Descriptor().Fields().ByName("code")
	if field == nil {
		t.Fatal("response omitted its status field")
	}
	var actual pb_common.ErrCode
	switch field.Kind() {
	case protoreflect.EnumKind:
		actual = pb_common.ErrCode(reflected.Get(field).Enum())
	default:
		actual = pb_common.ErrCode(reflected.Get(field).Int())
	}
	if actual != expected {
		t.Fatalf("status = %v, want %v", actual, expected)
	}
	if reflected.Descriptor().Fields().ByName("error") != nil {
		t.Fatal("response still contains a free-form error field")
	}
}

func TestQueriesReturnExplicitSuccessAndCurrentSnapshots(t *testing.T) {
	ctx, p, gate := loadoutContext(t)
	GetSoulJade(ctx, &pb_souljade.GetRoleSoulJadeReq{RequestId: 10})
	jade := gate.reply(t).(*pb_souljade.GetRoleSoulJadeAck)
	requireStatus(t, jade, pb_common.ErrCode_OK)
	if jade.GetRequestId() != 10 || jade.GetData() == nil {
		t.Fatal("missing soul jade request ID or snapshot")
	}
	GetTalent(ctx, &pb_talent.GetRoleTalentReq{RequestId: 11})
	talents := gate.reply(t).(*pb_talent.GetRoleTalentAck)
	requireStatus(t, talents, pb_common.ErrCode_OK)
	if talents.GetRequestId() != 11 || talents.GetData() == nil || p.IsDirty() {
		t.Fatal("query changed state or omitted metadata")
	}
	p.GetRoleMgr().SetCharacter(nil)
	GetSoulJade(ctx, &pb_souljade.GetRoleSoulJadeReq{RequestId: 12})
	requireStatus(t, gate.reply(t), pb_common.ErrCode_LOADOUT_UNAVAILABLE)
	GetTalent(ctx, &pb_talent.GetRoleTalentReq{RequestId: 13})
	requireStatus(t, gate.reply(t), pb_common.ErrCode_LOADOUT_UNAVAILABLE)
}

func TestUpgradeReplayReturnsStaleRevisionWithoutDoubleCharge(t *testing.T) {
	ctx, p, gate := loadoutContext(t)
	mgr := p.GetSoulJadeMgr()
	id := mgr.GetSlots()[1]
	req := &pb_souljade.UpgradeRoleSoulJadeReq{RequestId: 100, ExpectedRevision: mgr.GetRevision(), InstanceId: id}
	UpgradeSoulJade(ctx, req)
	first := gate.reply(t).(*pb_souljade.UpgradeRoleSoulJadeAck)
	requireStatus(t, first, pb_common.ErrCode_OK)
	if first.GetRequestId() != 100 || !p.IsDirty() {
		t.Fatal("successful upgrade did not echo the request or mark dirty")
	}
	remaining, revision := mgr.GetEssence(), mgr.GetRevision()
	p.SetDirty(false)
	UpgradeSoulJade(ctx, req)
	second := gate.reply(t).(*pb_souljade.UpgradeRoleSoulJadeAck)
	requireStatus(t, second, pb_common.ErrCode_STALE_REVISION)
	if second.GetData() == nil || second.GetData().GetEssence() != remaining || mgr.GetRevision() != revision || p.IsDirty() {
		t.Fatal("replayed request charged again or lost the current snapshot")
	}
	req.RequestId = 101
	UpgradeSoulJade(ctx, req)
	third := gate.reply(t).(*pb_souljade.UpgradeRoleSoulJadeAck)
	requireStatus(t, third, pb_common.ErrCode_STALE_REVISION)
	if third.GetData().GetRevision() != revision || mgr.GetEssence() != remaining || p.IsDirty() {
		t.Fatal("concurrent stale request changed state")
	}
	req.ExpectedRevision = revision
	req.RequestId = 0
	UpgradeSoulJade(ctx, req)
	requireStatus(t, gate.reply(t), pb_common.ErrCode_STALE_REVISION)
	if mgr.GetEssence() != remaining || p.IsDirty() {
		t.Fatal("missing request ID changed state")
	}
}

func TestUpgradeBusinessFailurePreservesStateAndReturnsPreciseCode(t *testing.T) {
	ctx, p, gate := loadoutContext(t)
	mgr := p.GetSoulJadeMgr()
	mgr.SetEssence(0)
	revision := mgr.GetRevision()
	id := mgr.GetSlots()[1]
	level := mgr.GetItems()[id].GetLevel()
	UpgradeSoulJade(ctx, &pb_souljade.UpgradeRoleSoulJadeReq{RequestId: 102, ExpectedRevision: revision, InstanceId: id})
	ack := gate.reply(t).(*pb_souljade.UpgradeRoleSoulJadeAck)
	requireStatus(t, ack, pb_common.ErrCode_INSUFFICIENT_ESSENCE)
	if ack.GetData() == nil || ack.GetData().GetRevision() != revision || mgr.GetItems()[id].GetLevel() != level || p.IsDirty() {
		t.Fatal("failed upgrade partially changed state or omitted the snapshot")
	}
}

func TestRefineReplayDoesNotChargeTwice(t *testing.T) {
	ctx, p, gate := loadoutContext(t)
	mgr := p.GetSoulJadeMgr()
	req := &pb_souljade.RefineRoleSoulJadeReq{RequestId: 110, ExpectedRevision: mgr.GetRevision(), InstanceId: mgr.GetSlots()[1]}
	RefineSoulJade(ctx, req)
	requireStatus(t, gate.reply(t), pb_common.ErrCode_OK)
	if !p.IsDirty() {
		t.Fatal("successful reroll did not mark dirty")
	}
	remaining, revision := mgr.GetEssence(), mgr.GetRevision()
	p.SetDirty(false)
	RefineSoulJade(ctx, req)
	ack := gate.reply(t).(*pb_souljade.RefineRoleSoulJadeAck)
	requireStatus(t, ack, pb_common.ErrCode_STALE_REVISION)
	if ack.GetData().GetRevision() != revision || mgr.GetEssence() != remaining || p.IsDirty() {
		t.Fatal("replayed reroll changed state")
	}
}

func TestEquipReturnsSpecificErrorsAndFreshSnapshot(t *testing.T) {
	ctx, p, gate := loadoutContext(t)
	mgr := p.GetSoulJadeMgr()
	revision := mgr.GetRevision()
	req := &pb_souljade.ChangeRoleSoulJadeReq{RequestId: 120, ExpectedRevision: revision, Slot: 2, InstanceId: mgr.GetSlots()[1]}
	ChangeSoulJade(ctx, req)
	ack := gate.reply(t).(*pb_souljade.ChangeRoleSoulJadeAck)
	requireStatus(t, ack, pb_common.ErrCode_DUPLICATE_JADE)
	if ack.GetData() == nil || mgr.GetRevision() != revision || p.IsDirty() {
		t.Fatal("duplicate equipment request changed state")
	}
	req.InstanceId = ""
	ChangeSoulJade(ctx, req)
	requireStatus(t, gate.reply(t), pb_common.ErrCode_OK)
	if !p.IsDirty() || mgr.GetSlots()[2] != "" {
		t.Fatal("successful slot change was not persisted")
	}
	p.SetDirty(false)
	ChangeSoulJade(ctx, req)
	requireStatus(t, gate.reply(t), pb_common.ErrCode_STALE_REVISION)
	if p.IsDirty() {
		t.Fatal("stale slot change marked dirty")
	}
}

func TestTalentLearnAndResetRejectStaleRequests(t *testing.T) {
	ctx, p, gate := loadoutContext(t)
	mgr := p.GetTalentMgr()
	req := &pb_talent.LearnRoleTalentReq{RequestId: 200, ExpectedRevision: mgr.GetRevision(), NodeId: 1}
	LearnTalent(ctx, req)
	first := gate.reply(t).(*pb_talent.LearnRoleTalentAck)
	requireStatus(t, first, pb_common.ErrCode_OK)
	if first.GetData().GetAvailablePoints() != 5 || !p.IsDirty() {
		t.Fatal("successful learning did not spend points or mark dirty")
	}
	p.SetDirty(false)
	req.NodeId, req.RequestId = 2, 201
	LearnTalent(ctx, req)
	second := gate.reply(t).(*pb_talent.LearnRoleTalentAck)
	requireStatus(t, second, pb_common.ErrCode_STALE_REVISION)
	if second.GetData().GetSpentPoints() != 1 || mgr.GetLearned()[2] || p.IsDirty() {
		t.Fatal("stale talent request changed state")
	}
	reset := &pb_talent.ResetRoleTalentReq{RequestId: 202, ExpectedRevision: mgr.GetRevision(), Branch: 0}
	ResetTalent(ctx, reset)
	third := gate.reply(t).(*pb_talent.ResetRoleTalentAck)
	requireStatus(t, third, pb_common.ErrCode_OK)
	if third.GetData().GetAvailablePoints() != 6 || third.GetData().GetBonuses().GetAttackPermille() != 0 || !p.IsDirty() {
		t.Fatal("reset failed to refund points or persist state")
	}
	p.SetDirty(false)
	ResetTalent(ctx, reset)
	requireStatus(t, gate.reply(t), pb_common.ErrCode_STALE_REVISION)
	if p.IsDirty() {
		t.Fatal("replayed reset marked dirty")
	}
}

func TestWriteGuardsReplyOnceWithoutChangingState(t *testing.T) {
	actions := []struct {
		name   string
		invoke func(*core.Context, uint64, uint64)
	}{
		{"change", func(ctx *core.Context, requestID, revision uint64) {
			ChangeSoulJade(ctx, &pb_souljade.ChangeRoleSoulJadeReq{RequestId: requestID, ExpectedRevision: revision, Slot: 2})
		}},
		{"upgrade", func(ctx *core.Context, requestID, revision uint64) {
			UpgradeSoulJade(ctx, &pb_souljade.UpgradeRoleSoulJadeReq{RequestId: requestID, ExpectedRevision: revision})
		}},
		{"refine", func(ctx *core.Context, requestID, revision uint64) {
			RefineSoulJade(ctx, &pb_souljade.RefineRoleSoulJadeReq{RequestId: requestID, ExpectedRevision: revision})
		}},
		{"learn", func(ctx *core.Context, requestID, revision uint64) {
			LearnTalent(ctx, &pb_talent.LearnRoleTalentReq{RequestId: requestID, ExpectedRevision: revision, NodeId: 1})
		}},
		{"reset", func(ctx *core.Context, requestID, revision uint64) {
			ResetTalent(ctx, &pb_talent.ResetRoleTalentReq{RequestId: requestID, ExpectedRevision: revision, Branch: 0})
		}},
	}
	conditions := []struct {
		name      string
		prepare   func(*player.Player)
		requestID uint64
		revision  uint64
		code      pb_common.ErrCode
	}{
		{"missing role module", func(p *player.Player) { p.RoleMgr = nil }, 300, 1, pb_common.ErrCode_LOADOUT_UNAVAILABLE},
		{"missing character", func(p *player.Player) { p.GetRoleMgr().SetCharacter(nil) }, 300, 1, pb_common.ErrCode_LOADOUT_UNAVAILABLE},
		{"empty character ID", func(p *player.Player) { p.GetRoleMgr().SetCharacter(&role.Role{}) }, 300, 1, pb_common.ErrCode_LOADOUT_UNAVAILABLE},
		{"missing growth module", func(p *player.Player) {
			p.SoulJadeMgr = nil
			p.TalentMgr = nil
		}, 300, 1, pb_common.ErrCode_LOADOUT_UNAVAILABLE},
		{"uninitialized revision", func(p *player.Player) {
			p.GetSoulJadeMgr().SetRevision(0)
			p.GetTalentMgr().SetRevision(0)
		}, 300, 0, pb_common.ErrCode_LOADOUT_UNAVAILABLE},
		{"missing request ID", func(*player.Player) {}, 0, 1, pb_common.ErrCode_STALE_REVISION},
		{"stale revision", func(*player.Player) {}, 300, 0, pb_common.ErrCode_STALE_REVISION},
	}
	for _, action := range actions {
		for _, condition := range conditions {
			t.Run(action.name+"/"+condition.name, func(t *testing.T) {
				ctx, p, gate := loadoutContext(t)
				condition.prepare(p)
				beforeJade, beforeTalent := p.BuildSoulJadeData(), p.BuildTalentData()
				beforeRevision := p.Revision()
				action.invoke(ctx, condition.requestID, condition.revision)
				ack := gate.reply(t)
				requireStatus(t, ack, condition.code)
				if gate.sends != 1 {
					t.Fatalf("handler sent %d replies, want one", gate.sends)
				}
				reflected := ack.ProtoReflect()
				requestField := reflected.Descriptor().Fields().ByName("request_id")
				if reflected.Get(requestField).Uint() != condition.requestID {
					t.Fatal("guard response did not echo the request ID")
				}
				dataField := reflected.Descriptor().Fields().ByName("data")
				snapshot := reflected.Get(dataField).Message().Interface()
				if action.name == "learn" || action.name == "reset" {
					if !proto.Equal(snapshot, beforeTalent) {
						t.Fatal("guard response lost the initial talent snapshot")
					}
				} else if !proto.Equal(snapshot, beforeJade) {
					t.Fatal("guard response lost the initial soul jade snapshot")
				}
				if p.IsDirty() || p.Revision() != beforeRevision {
					t.Fatal("rejected request marked the player dirty")
				}
				if !proto.Equal(beforeJade, p.BuildSoulJadeData()) || !proto.Equal(beforeTalent, p.BuildTalentData()) {
					t.Fatal("rejected request changed growth state")
				}
			})
		}
	}
}

func TestQueriesWithMissingModulesReturnUnavailable(t *testing.T) {
	ctx, p, gate := loadoutContext(t)
	p.SoulJadeMgr = nil
	p.TalentMgr = nil
	GetSoulJade(ctx, &pb_souljade.GetRoleSoulJadeReq{RequestId: 310})
	requireStatus(t, gate.reply(t), pb_common.ErrCode_LOADOUT_UNAVAILABLE)
	GetTalent(ctx, &pb_talent.GetRoleTalentReq{RequestId: 311})
	requireStatus(t, gate.reply(t), pb_common.ErrCode_LOADOUT_UNAVAILABLE)
	if gate.sends != 2 || p.IsDirty() {
		t.Fatal("queries omitted a response or changed state")
	}
}
