package internal

import (
	"fmt"

	"github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
)

// OnCreateBattle 创建一场战斗
func (s *Server) OnCreateBattle(ctx tree.Context, req *pb_battle.StartBattleReq) {
	if req == nil || req.UID == 0 || req.ServerID == 0 || req.SourceNodeId == 0 {
		ctx.Response(nil, fmt.Errorf("battle service: uid, source server and source node are required"))
		return
	}

}
