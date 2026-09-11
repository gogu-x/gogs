package internal

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/gogu-x/gogs/battle/battle"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"github.com/google/uuid"
)

// OnCreateBattle 只校验请求包络、生成 BattleID/Seed 并创建 BattleActor。
func (s *Server) OnCreateBattle(ctx tree.Context, req *pb.StartBattleReq) {
	if req == nil || req.GetUID() == 0 || req.GetServerID() == 0 || req.GetSourceNodeId() == 0 {
		ctx.Response(nil, fmt.Errorf("battle service: uid, source server and source node are required"))
		return
	}
	if req.GetBattleType() == pb.BattleType_BATTLE_TYPE_UNSPECIFIED {
		ctx.Response(nil, fmt.Errorf("battle service: battle type is required"))
		return
	}
	battleID := uuid.NewString()
	seed, err := newSeed()
	if err != nil {
		ctx.Response(nil, err)
		return
	}
	actor := battle.New(battle.Params{
		BattleID:      battleID,
		Seed:          seed,
		Request:       req,
		UID:           req.GetUID(),
		Source:        battle.Source{ServerID: int(req.GetServerID()), NodeID: int(req.GetSourceNodeId())},
		ManagerPID:    s.self,
		Mode:          battle.ModeRun,
		Reports:       s.reports,
		Notifier:      s.notifier,
		RetryDelay:    time.Second,
		RetryAttempts: 3,
	})
	pid := s.system.SpawnOne(actor)
	s.active[battleID] = pid
	if tlog.Log != nil {
		tlog.Log.Info("[战斗/创建] 创建BattleActor, battleID=%v seed=%v uid=%v", battleID, seed, req.GetUID())
	}
	ctx.Response(&pb.StartBattleAck{BattleId: battleID}, nil)
}

// OnActorStopped 清理已结束 BattleActor 的 active 登记。
func (s *Server) OnActorStopped(_ tree.Context, msg *battle.ActorStopped) {
	if msg == nil || msg.BattleID == "" {
		return
	}
	if pid, exists := s.active[msg.BattleID]; exists && pid == msg.PID {
		delete(s.active, msg.BattleID)
		if tlog.Log != nil {
			tlog.Log.Info("[战斗/回收] 清理BattleActor, battleID=%v", msg.BattleID)
		}
	}
}

func newSeed() (uint64, error) {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return 0, fmt.Errorf("battle service: generate seed: %w", err)
	}
	return binary.LittleEndian.Uint64(bytes[:]), nil
}
