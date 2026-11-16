package ctl_loadout

import (
	"testing"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/player"
	"github.com/gogu-x/gogs/game/play/internal/player/loadouttest"
	"github.com/gogu-x/gogs/game/play/internal/player/role"
	"github.com/gogu-x/gogs/game/play/internal/player/souljade"
	"github.com/gogu-x/gogs/game/play/internal/player/talent"
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
	p.SoulJadeMgr = souljade.New(42)
	p.TalentMgr = talent.New()
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
}

func TestQueriesReturnCurrentSnapshots(t *testing.T) {
	ctx, p, gate := loadoutContext(t)
	GetSoulJade(ctx, &pb_souljade.GetRoleSoulJadeReq{RequestId: 10})
	jade := gate.reply(t).(*pb_souljade.GetRoleSoulJadeAck)
	requireStatus(t, jade, pb_common.ErrCode_OK)
	if jade.GetRequestId() != 10 || jade.GetData() == nil || jade.GetData().GetRoleId() != "role-42" {
		t.Fatal("missing soul jade request ID or snapshot")
	}
	GetTalent(ctx, &pb_talent.GetRoleTalentReq{RequestId: 11})
	talents := gate.reply(t).(*pb_talent.GetRoleTalentAck)
	requireStatus(t, talents, pb_common.ErrCode_OK)
	if talents.GetRequestId() != 11 || talents.GetData() == nil || p.IsDirty() {
		t.Fatal("query changed state or omitted the snapshot")
	}
}

func TestNewPlayerEmptyLoadoutSnapshotsAndTalentOperation(t *testing.T) {
	ctx, p, gate := loadoutContext(t)
	newPlayer := player.NewPlayerData(43)
	if newPlayer.GetSoulJadeMgr() == nil || newPlayer.GetTalentMgr() == nil || len(newPlayer.GetSoulJadeMgr().GetItems()) != 0 || len(newPlayer.GetEquipmentMgr().GetItems()) != 0 {
		t.Fatal("new player did not receive empty loadout modules")
	}
	p.SoulJadeMgr = newPlayer.GetSoulJadeMgr()
	p.TalentMgr = newPlayer.GetTalentMgr()
	GetSoulJade(ctx, &pb_souljade.GetRoleSoulJadeReq{RequestId: 20})
	jade := gate.reply(t).(*pb_souljade.GetRoleSoulJadeAck)
	if jade.GetCode() != pb_common.ErrCode_OK || jade.GetData() == nil || jade.GetData().GetRoleId() != "role-42" || len(jade.GetData().GetItems()) != 0 || len(jade.GetData().GetSlots()) == 0 {
		t.Fatalf("invalid empty soul jade snapshot: %v", jade)
	}
	if len(p.GetEquipmentMgr().GetItems()) != 0 {
		t.Fatal("initialization granted equipment")
	}
	ChangeSoulJade(ctx, &pb_souljade.ChangeRoleSoulJadeReq{RequestId: 23, Slot: 1, InstanceId: "unowned"})
	change := gate.reply(t).(*pb_souljade.ChangeRoleSoulJadeAck)
	requireStatus(t, change, pb_common.ErrCode_ITEM_NOT_OWNED)
	if change.GetData() == nil || len(change.GetData().GetItems()) != 0 {
		t.Fatal("invalid equip did not preserve the empty snapshot")
	}
	GetTalent(ctx, &pb_talent.GetRoleTalentReq{RequestId: 21})
	talents := gate.reply(t).(*pb_talent.GetRoleTalentAck)
	if talents.GetCode() != pb_common.ErrCode_OK || talents.GetData() == nil || talents.GetData().GetRoleId() != "role-42" || len(talents.GetData().GetNodes()) == 0 {
		t.Fatalf("invalid initial talent snapshot: %v", talents)
	}
	LearnTalent(ctx, &pb_talent.LearnRoleTalentReq{RequestId: 22, NodeId: 1})
	learn := gate.reply(t).(*pb_talent.LearnRoleTalentAck)
	requireStatus(t, learn, pb_common.ErrCode_OK)
	if learn.GetData() == nil || learn.GetData().GetSpentPoints() == 0 {
		t.Fatal("talent learning did not update the snapshot")
	}
}

func TestQueriesWithoutRoleStillReturnModuleData(t *testing.T) {
	ctx, _, gate := loadoutContext(t)
	ctx.GetPlayer().GetRoleMgr().SetCharacter(nil)
	GetSoulJade(ctx, &pb_souljade.GetRoleSoulJadeReq{RequestId: 12})
	jade := gate.reply(t).(*pb_souljade.GetRoleSoulJadeAck)
	requireStatus(t, jade, pb_common.ErrCode_OK)
	if jade.GetData() == nil || jade.GetData().GetRoleId() != "" {
		t.Fatal("soul jade query depends on the role")
	}
	GetTalent(ctx, &pb_talent.GetRoleTalentReq{RequestId: 13})
	talents := gate.reply(t).(*pb_talent.GetRoleTalentAck)
	requireStatus(t, talents, pb_common.ErrCode_OK)
	if talents.GetData() == nil {
		t.Fatal("talent query depends on the role")
	}
}

func TestQueriesWithMissingModulesReturnEmpty(t *testing.T) {
	ctx, p, gate := loadoutContext(t)
	p.SoulJadeMgr = nil
	p.TalentMgr = nil
	GetSoulJade(ctx, &pb_souljade.GetRoleSoulJadeReq{RequestId: 310})
	jade := gate.reply(t).(*pb_souljade.GetRoleSoulJadeAck)
	requireStatus(t, jade, pb_common.ErrCode_OK)
	if jade.GetRequestId() != 310 || jade.GetData() != nil {
		t.Fatal("missing soul jade data should return an empty snapshot")
	}
	GetTalent(ctx, &pb_talent.GetRoleTalentReq{RequestId: 311})
	talents := gate.reply(t).(*pb_talent.GetRoleTalentAck)
	requireStatus(t, talents, pb_common.ErrCode_OK)
	if talents.GetRequestId() != 311 || talents.GetData() != nil {
		t.Fatal("missing talent data should return an empty snapshot")
	}
	if gate.sends != 2 || p.IsDirty() {
		t.Fatal("queries omitted a response or changed state")
	}
}

func TestUpgradeIgnoresRevisionAndSpendsEssence(t *testing.T) {
	ctx, p, gate := loadoutContext(t)
	mgr := p.GetSoulJadeMgr()
	id := mgr.GetSlots()[1]
	essence := mgr.GetEssence()
	UpgradeSoulJade(ctx, &pb_souljade.UpgradeRoleSoulJadeReq{RequestId: 100, ExpectedRevision: 999, InstanceId: id})
	ack := gate.reply(t).(*pb_souljade.UpgradeRoleSoulJadeAck)
	requireStatus(t, ack, pb_common.ErrCode_OK)
	if ack.GetRequestId() != 100 || ack.GetData() == nil || !p.IsDirty() || mgr.GetEssence() >= essence {
		t.Fatal("upgrade did not apply, echo the request or mark dirty")
	}
}

func TestUpgradeBusinessFailurePreservesStateAndReturnsPreciseCode(t *testing.T) {
	ctx, p, gate := loadoutContext(t)
	mgr := p.GetSoulJadeMgr()
	mgr.SetEssence(0)
	id := mgr.GetSlots()[1]
	level := mgr.GetItems()[id].GetLevel()
	UpgradeSoulJade(ctx, &pb_souljade.UpgradeRoleSoulJadeReq{RequestId: 102, InstanceId: id})
	ack := gate.reply(t).(*pb_souljade.UpgradeRoleSoulJadeAck)
	requireStatus(t, ack, pb_common.ErrCode_INSUFFICIENT_ESSENCE)
	if ack.GetData() == nil || mgr.GetItems()[id].GetLevel() != level || p.IsDirty() {
		t.Fatal("failed upgrade partially changed state or omitted the snapshot")
	}
}

func TestRefineAppliesWithoutRevision(t *testing.T) {
	ctx, p, gate := loadoutContext(t)
	mgr := p.GetSoulJadeMgr()
	RefineSoulJade(ctx, &pb_souljade.RefineRoleSoulJadeReq{RequestId: 110, InstanceId: mgr.GetSlots()[1]})
	requireStatus(t, gate.reply(t), pb_common.ErrCode_OK)
	if !p.IsDirty() {
		t.Fatal("successful reroll did not mark dirty")
	}
}

func TestEquipReturnsSpecificErrorsAndFreshSnapshot(t *testing.T) {
	ctx, p, gate := loadoutContext(t)
	mgr := p.GetSoulJadeMgr()
	req := &pb_souljade.ChangeRoleSoulJadeReq{RequestId: 120, Slot: 2, InstanceId: mgr.GetSlots()[1]}
	ChangeSoulJade(ctx, req)
	ack := gate.reply(t).(*pb_souljade.ChangeRoleSoulJadeAck)
	requireStatus(t, ack, pb_common.ErrCode_DUPLICATE_JADE)
	if ack.GetData() == nil || p.IsDirty() {
		t.Fatal("duplicate equipment request changed state")
	}
	req.InstanceId = ""
	ChangeSoulJade(ctx, req)
	ack = gate.reply(t).(*pb_souljade.ChangeRoleSoulJadeAck)
	requireStatus(t, ack, pb_common.ErrCode_OK)
	if !p.IsDirty() || mgr.GetSlots()[2] != "" || ack.GetData() == nil {
		t.Fatal("successful slot change was not applied")
	}
}

func TestTalentLearnAndReset(t *testing.T) {
	ctx, p, gate := loadoutContext(t)
	LearnTalent(ctx, &pb_talent.LearnRoleTalentReq{RequestId: 200, NodeId: 1})
	first := gate.reply(t).(*pb_talent.LearnRoleTalentAck)
	requireStatus(t, first, pb_common.ErrCode_OK)
	if first.GetData().GetAvailablePoints() != 5 || !p.IsDirty() {
		t.Fatal("successful learning did not spend points or mark dirty")
	}
	p.SetDirty(false)
	LearnTalent(ctx, &pb_talent.LearnRoleTalentReq{RequestId: 201, NodeId: 1})
	requireStatus(t, gate.reply(t), pb_common.ErrCode_TALENT_ALREADY_LEARNED)
	if p.IsDirty() {
		t.Fatal("rejected learning marked dirty")
	}
	ResetTalent(ctx, &pb_talent.ResetRoleTalentReq{RequestId: 202, Branch: 0})
	third := gate.reply(t).(*pb_talent.ResetRoleTalentAck)
	requireStatus(t, third, pb_common.ErrCode_OK)
	if third.GetData().GetAvailablePoints() != 6 || third.GetData().GetBonuses().GetAttackPermille() != 0 || !p.IsDirty() {
		t.Fatal("reset failed to refund points or persist state")
	}
}

func TestWritesWithMissingModulesReturnUnavailable(t *testing.T) {
	actions := []struct {
		name   string
		invoke func(*core.Context)
	}{
		{"change", func(ctx *core.Context) {
			ChangeSoulJade(ctx, &pb_souljade.ChangeRoleSoulJadeReq{RequestId: 300, Slot: 2})
		}},
		{"upgrade", func(ctx *core.Context) {
			UpgradeSoulJade(ctx, &pb_souljade.UpgradeRoleSoulJadeReq{RequestId: 300})
		}},
		{"refine", func(ctx *core.Context) {
			RefineSoulJade(ctx, &pb_souljade.RefineRoleSoulJadeReq{RequestId: 300})
		}},
		{"learn", func(ctx *core.Context) {
			LearnTalent(ctx, &pb_talent.LearnRoleTalentReq{RequestId: 300, NodeId: 1})
		}},
		{"reset", func(ctx *core.Context) {
			ResetTalent(ctx, &pb_talent.ResetRoleTalentReq{RequestId: 300, Branch: 0})
		}},
	}
	for _, action := range actions {
		t.Run(action.name, func(t *testing.T) {
			ctx, p, gate := loadoutContext(t)
			p.SoulJadeMgr = nil
			p.TalentMgr = nil
			action.invoke(ctx)
			ack := gate.reply(t)
			requireStatus(t, ack, pb_common.ErrCode_LOADOUT_UNAVAILABLE)
			if gate.sends != 1 {
				t.Fatalf("handler sent %d replies, want one", gate.sends)
			}
			reflected := ack.ProtoReflect()
			if reflected.Get(reflected.Descriptor().Fields().ByName("request_id")).Uint() != 300 {
				t.Fatal("response did not echo the request ID")
			}
			if reflected.Has(reflected.Descriptor().Fields().ByName("data")) {
				t.Fatal("missing module should return an empty snapshot")
			}
			if p.IsDirty() {
				t.Fatal("rejected request marked the player dirty")
			}
		})
	}
}
