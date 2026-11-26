package mode

import (
	"fmt"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

type Scenario struct {
	Attackers            []*pb.Role
	Defenders            []*pb.Role
	MonsterGroupConfigID int32
}

// Rule validates the raw create request and reduces it to the participants
// and encounter reference that the config compiler must resolve.
type Rule interface {
	Resolve(*pb.StartBattleReq) (Scenario, error)
}

type Registry map[pb.BattleType]Rule

// DefaultRegistry 构造内置战斗模式规则表。
func DefaultRegistry() Registry {
	pve := pveRule{}
	return Registry{
		pb.BattleType_BATTLE_TYPE_TOWER: pve,
		pb.BattleType_BATTLE_TYPE_BOSS:  pve,
		pb.BattleType_BATTLE_TYPE_STORY: pve,
		pb.BattleType_BATTLE_TYPE_TEST:  pve,
		pb.BattleType_BATTLE_TYPE_PVP:   pvpRule{},
	}
}

type pveRule struct{}

// Resolve 校验进攻方与怪物组并解析挑战场景。
func (pveRule) Resolve(request *pb.StartBattleReq) (Scenario, error) {
	if len(request.GetAttackerRoles()) == 0 {
		return Scenario{}, fmt.Errorf("battle type=%d requires attacker roles", request.GetBattleType())
	}
	if len(request.GetDefenderRoles()) != 0 {
		return Scenario{}, fmt.Errorf("battle type=%d forbids defender roles", request.GetBattleType())
	}
	if request.GetMonsterGroupConfigId() == 0 {
		return Scenario{}, fmt.Errorf("battle type=%d requires monster group config id", request.GetBattleType())
	}
	return Scenario{Attackers: request.GetAttackerRoles(), MonsterGroupConfigID: request.GetMonsterGroupConfigId()}, nil
}

type pvpRule struct{}

// Resolve 校验对战双方的角色并解析玩家对战场景。
func (pvpRule) Resolve(request *pb.StartBattleReq) (Scenario, error) {
	if len(request.GetAttackerRoles()) != 1 || len(request.GetDefenderRoles()) != 1 {
		return Scenario{}, fmt.Errorf("pvp requires exactly one attacker role and one defender role")
	}
	if request.GetMonsterGroupConfigId() != 0 {
		return Scenario{}, fmt.Errorf("pvp forbids monster group config id")
	}
	return Scenario{Attackers: request.GetAttackerRoles(), Defenders: request.GetDefenderRoles()}, nil
}
