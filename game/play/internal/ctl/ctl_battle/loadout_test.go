package ctl_battle

import (
	"github.com/gogu-x/gogs/game/play/internal/player"
	"github.com/gogu-x/gogs/game/play/internal/player/loadouttest"
	"github.com/gogu-x/gogs/game/play/internal/player/role"
	"github.com/gogu-x/gogs/game/play/internal/player/talent"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"testing"
)

func TestBattleIgnoresClientGrowthAndUsesStoredLoadout(t *testing.T) {
	loadouttest.Load(t)
	p := player.NewPlayerData(99)
	p.GetRoleMgr().SetCharacter(&role.Role{ID: "role-99", ConfigID: 1, Level: 1})
	if code := talent.Learn(p.GetTalentMgr(), 1, 6); code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	incoming := &pb.StartBattleReq{AttackerRoles: []*pb.Role{{RoleId: "forged", GrowthBonus: &pb_common.GrowthBonuses{AttackPermille: 999999}}}}
	request, code := buildBattleRequest(p, incoming)
	if code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	roles := request.GetAttackerRoles()
	if len(roles) != 1 || roles[0].GetRoleId() != "role-99" || roles[0].GetGrowthBonus().GetAttackPermille() != 44 {
		t.Fatalf("battle did not use authoritative growth: %v", roles)
	}
	if incoming.GetAttackerRoles()[0].GetGrowthBonus().GetAttackPermille() != 999999 {
		t.Fatal("original request was modified")
	}
}
