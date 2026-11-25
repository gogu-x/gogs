package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"google.golang.org/protobuf/proto"
)

var battleScenario = flag.Bool("battle", false, "登录后演示即时战斗，建议配合 -users 1")
var allowJoin = flag.Bool("battle-allow-join", false, "允许其他玩家中途加入")
var joinBattle = flag.String("battle-join", "", "要加入的共享战斗编号")
var preferredSkill = flag.String("battle-skill", "", "优先使用的已装备技能或法宝技能编号，冷却时自动普攻")

// playBattle 根据服务端回执安排下一次出手，唯一写入方同时处理结束通知。
func (c *client) playBattle() error {
	if err := c.send(&pb.StartBattleReq{AllowJoin: *allowJoin, JoinBattleId: *joinBattle}); err != nil {
		return err
	}
	type received struct {
		message proto.Message
		err     error
	}
	inbox := make(chan received)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			msg, err := c.recv()
			select {
			case inbox <- received{msg, err}:
			case <-done:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	var control *pb.BattleControl
	var battleID string
	var sequence uint64
	var frame time.Duration
	var fire <-chan time.Time
	var useBasic bool
	for {
		select {
		case <-fire:
			fire = nil
			skill := control.GetBasicSkill()
			if !useBasic {
				for _, id := range control.GetActiveSkills() {
					if id == *preferredSkill {
						skill = id
						break
					}
				}
			}
			sequence++
			if err := c.send(&pb.BattleInputReq{BattleId: battleID, UnitId: control.GetUnitId(), SkillId: skill, Sequence: sequence}); err != nil {
				return err
			}
		case item := <-inbox:
			if item.err != nil {
				return item.err
			}
			switch msg := item.message.(type) {
			case *pb.StartBattleAck:
				if msg.GetCode() != pb_common.ErrCode_OK {
					return fmt.Errorf("创建战斗失败 %v", msg.GetCode())
				}
			case *pb.BattleAdmissionFailedNtf:
				if msg.GetCode() != pb_common.ErrCode_OK {
					return fmt.Errorf("组队失败 %v", msg.GetCode())
				}
			case *pb.BattleCreatedNtf:
				if control != nil && battleID != msg.GetBattleId() {
					continue
				}
				if len(msg.GetControls()) != 1 {
					return fmt.Errorf("需要本人的角色控制信息")
				}
				control = msg.GetControls()[0]
				battleID = msg.GetBattleId()
				frame = time.Duration(msg.GetTickDurationMs()) * time.Millisecond
				sequence = max(sequence, control.GetLastInputSequence())
				if *users == 1 {
					log.Printf("战斗 %s 当前帧 %d", battleID, msg.GetCurrentTick())
				}
				fire = time.After(time.Duration(max(0, control.GetNextReadyTick()-msg.GetCurrentTick())) * frame)
			case *pb.BattleActionNtf:
				if *users == 1 && msg.GetBattleId() == battleID {
					event := msg.GetEvent()
					log.Printf("帧 %d %s 单位=%s 技能=%s 目标=%s 数值=%d", event.GetTick(), event.GetType(), event.GetActorId(), event.GetSkillId(), event.GetTargetId(), event.GetAmount())
				}
			case *pb.BattleInputAck:
				if msg.GetBattleId() != battleID || msg.GetSequence() != sequence {
					continue
				}
				useBasic = msg.GetCode() != pb_common.ErrCode_OK
				fire = time.After(time.Duration(max(1, msg.GetReadyTick()-msg.GetCurrentTick())) * frame)
			case *pb.BattleFinishedNtf:
				if msg.GetBattleId() == battleID {
					return nil
				}
			}
		}
	}
}
