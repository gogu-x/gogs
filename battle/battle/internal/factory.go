package internal

import (
	"github.com/gogu-x/gogs/battle/battle/internal/configcompiler"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// newBattleFromRequest preserves the existing package-level helper used by
// actor and engine tests while compilation lives in its own package.
func newBattleFromRequest(battleID string, seed uint64, request *pb.StartBattleReq) (*engine.Battle, error) {
	setup, err := configcompiler.NewDefault().Compile(battleID, seed, request)
	if err != nil {
		return nil, err
	}
	return engine.NewBattle(setup)
}
