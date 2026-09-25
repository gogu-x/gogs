package internal

import (
	"fmt"

	"github.com/gogu-x/gogs/battle/battle/internal/configcompiler"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/battle/battle/internal/mode"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

func validateBattleRequest(request *pb.StartBattleReq) error {
	if request == nil {
		return fmt.Errorf("battle request is required")
	}
	rule, ok := mode.DefaultRegistry()[request.GetBattleType()]
	if !ok {
		return fmt.Errorf("unsupported battle type=%d", request.GetBattleType())
	}
	_, err := rule.Resolve(request)
	return err
}

func buildRoleUnit(role *pb.Role, team pb.BattleTeam) (engine.UnitConfig, error) {
	return configcompiler.NewDefault().BuildRoleUnit(role, team)
}

func buildSkill(id int32) (engine.SkillConfig, error) {
	return configcompiler.NewDefault().BuildSkill(id)
}
