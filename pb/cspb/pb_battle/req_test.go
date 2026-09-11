package pb_battle

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestStartBattleReqRoundTrip(t *testing.T) {
	tests := []*StartBattleReq{
		{UID: 1, SourceNodeId: 2, BattleType: BattleType_BATTLE_TYPE_PVE, BusinessId: "stage-1", AttackerRoles: []*Role{{RoleId: "a", RoleConfigId: 1, Level: 1}}, MonsterGroupConfigId: 100},
		{UID: 2, SourceNodeId: 3, BattleType: BattleType_BATTLE_TYPE_PVP, BusinessId: "match-1", AttackerRoles: []*Role{{RoleId: "a", RoleConfigId: 1}}, DefenderRoles: []*Role{{RoleId: "d", RoleConfigId: 2}}},
	}
	for _, request := range tests {
		data, err := proto.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		decoded := &StartBattleReq{}
		if err := proto.Unmarshal(data, decoded); err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(request, decoded) {
			t.Fatalf("request mismatch: %v != %v", request, decoded)
		}
	}
}
