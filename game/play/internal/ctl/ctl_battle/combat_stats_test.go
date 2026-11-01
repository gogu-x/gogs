package ctl_battle

import (
	"testing"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/player"
	"github.com/gogu-x/gogs/game/play/internal/player/equipment"
	"github.com/gogu-x/gogs/game/play/internal/player/role"
	"github.com/gogu-x/gogs/ipb"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

type combatStatsGate struct {
	tree.Context
	ack   *pb_common.GetRoleCombatStatsAck
	uid   uint64
	sends int
}

func (gate *combatStatsGate) Lookup(name string) (tree.PID, bool) {
	return tree.PID{ID: 1}, name == def.GameGate
}

func (gate *combatStatsGate) Send(_ tree.PID, value interface{}) bool {
	push, ok := value.(*ipb.PushToMsg)
	if !ok {
		return false
	}
	ack, ok := push.Msg.(*pb_common.GetRoleCombatStatsAck)
	if !ok {
		return false
	}
	gate.ack = ack
	gate.uid = push.UID
	gate.sends++
	return true
}

func TestCombatStatsQueryMatchesBattleAndDoesNotModifyPlayer(t *testing.T) {
	tlog.NewLog("", 0)
	p := battleLoadoutPlayer(t)
	p.SetDirty(false)
	beforeRevision := p.Revision()
	gate := &combatStatsGate{}
	ctx := &core.Context{Actor: &core.PlayerActor{UID: 99, Player: p}, TreeCtx: gate}
	OnGetRoleCombatStats(ctx, &pb_common.GetRoleCombatStatsReq{RequestId: 901, UID: 123456})
	ack := gate.ack
	if ack == nil {
		t.Fatal("combat stat query did not return an ACK")
	}
	if ack.Code == nil {
		t.Fatal("combat stat query omitted explicit success")
	}
	if ack.GetCode() != pb_common.ErrCode_OK {
		t.Fatalf("combat stat query failed: %v", ack.GetCode())
	}
	if gate.uid != 99 {
		t.Fatal("combat stat query trusted client identity")
	}
	if gate.sends != 1 {
		t.Fatal("combat stat query did not reply exactly once")
	}
	if ack.GetRequestId() != 901 {
		t.Fatal("combat stat query lost request correlation")
	}
	data := ack.GetData()
	if data.GetRoleId() != "role-99" {
		t.Fatal("combat stat query used the wrong player role")
	}
	request, code := buildBattleRequest(p, &pb.StartBattleReq{})
	if code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	expected := compileLoadoutAttributes(t, request)
	if actual := [4]int64{data.GetMaxHp(), data.GetAttack(), data.GetDefense(), data.GetSpeed()}; actual != expected {
		t.Fatalf("query attributes %v differ from real battle attributes %v", actual, expected)
	}
	if p.IsDirty() {
		t.Fatal("read-only query marked the player dirty")
	}
	if p.Revision() != beforeRevision {
		t.Fatal("read-only query incremented the player revision")
	}
	if code := equipment.ChangeRoleEquipment(
		p.GetRoleMgr(), p.GetEquipmentMgr(), 17, "blade-b",
	); code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	OnGetRoleCombatStats(ctx, &pb_common.GetRoleCombatStatsReq{RequestId: 902})
	if gate.ack.GetData().GetMaxHp() != 1200 {
		t.Fatal("query retained stale attributes after an equipment change")
	}
	if data.GetMaxHp() != expected[0] {
		t.Fatal("a new query mutated the previous response")
	}
	if p.IsDirty() {
		t.Fatal("second query marked the player dirty")
	}
}

func TestCombatStatsQueryReturnsTypedFailuresWithoutChangingState(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*player.Player)
		code    pb_common.ErrCode
	}{
		{"missing role module", func(p *player.Player) { p.RoleMgr = nil }, pb_common.ErrCode_LOADOUT_UNAVAILABLE},
		{"missing character", func(p *player.Player) { p.GetRoleMgr().SetCharacter(nil) }, pb_common.ErrCode_LOADOUT_UNAVAILABLE},
		{"missing equipment module", func(p *player.Player) { p.EquipmentMgr = nil }, pb_common.ErrCode_EQUIPMENT_UNAVAILABLE},
		{"missing equipped item", func(p *player.Player) {
			p.GetRoleMgr().SetCharacter(&role.Role{ID: "role-99", ConfigID: 1, EquipmentIDs: []string{"missing"}})
		}, pb_common.ErrCode_EQUIPMENT_UNAVAILABLE},
		{"missing role config", func(p *player.Player) {
			p.GetRoleMgr().SetCharacter(&role.Role{ID: "role-99", ConfigID: 999})
		}, pb_common.ErrCode_CONFIG_UNAVAILABLE},
		{"missing equipment config", func(p *player.Player) {
			p.GetEquipmentMgr().AddItem(&equipment.Item{ID: "blade-a", ConfigID: 999})
		}, pb_common.ErrCode_CONFIG_UNAVAILABLE},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tlog.NewLog("", 0)
			p := battleLoadoutPlayer(t)
			test.prepare(p)
			p.SetDirty(false)
			beforeRevision := p.Revision()
			gate := &combatStatsGate{}
			ctx := &core.Context{Actor: &core.PlayerActor{UID: 99, Player: p}, TreeCtx: gate}
			OnGetRoleCombatStats(ctx, &pb_common.GetRoleCombatStatsReq{RequestId: 903})
			ack := gate.ack
			if ack == nil {
				t.Fatal("rejected query omitted its ACK")
			}
			if ack.Code == nil {
				t.Fatal("rejected query omitted explicit status")
			}
			if ack.GetCode() != test.code {
				t.Fatalf("code = %v, want %v", ack.GetCode(), test.code)
			}
			if ack.GetData() != nil {
				t.Fatal("rejected query published incomplete attributes")
			}
			if gate.sends != 1 {
				t.Fatal("rejected query did not reply exactly once")
			}
			if ack.GetRequestId() != 903 {
				t.Fatal("rejected query lost request correlation")
			}
			if p.IsDirty() {
				t.Fatal("rejected query marked the player dirty")
			}
			if p.Revision() != beforeRevision {
				t.Fatal("rejected query incremented the player revision")
			}
		})
	}
}
