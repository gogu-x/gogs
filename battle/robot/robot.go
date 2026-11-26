// Package robot 提供按策略主动提交动作的通用机器人。
package robot

import (
	"container/heap"
	"github.com/gogu-x/gogs/battle/robot/strategy"
	"time"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
)

// Policy 根据可控动作和上次回执选择下一次输入。
type Policy func(*pb.BattleControl, *pb.BattleInputAck) string

// Command 仅在本进程使用，将机器人输入与网络请求区分。
type Command struct {
	Input *pb.BattleInputReq // 本次动作。
}

// RobotActor 调度同一场战斗内各单位的独立决策。
type RobotActor struct {
	router   tree.Router
	battleID string
	target   tree.PID
	start    time.Time
	units    map[string]*unitAI
	queue    decisionQueue
	wheel    *timer.TimeWheel
	wake     *timer.WheelTimer
	wakeMS   int64
}

type unitAI struct {
	control  *pb.BattleControl
	position int32
	rules    []strategy.Rule
	policy   Policy
	due      int64
	sequence uint64
	waiting  bool
	last     *pb.BattleInputAck
	index    int
}

// New 建立与战斗共用时间原点的调度器。
func New(battleID string, target tree.PID, start time.Time) *RobotActor {
	return &RobotActor{battleID: battleID, target: target, start: start, units: make(map[string]*unitAI)}
}

// AddUnit 在启动 Actor 前登记单位及其独立策略。
func (r *RobotActor) AddUnit(control *pb.BattleControl, position int32,
	firstDelay int64, policy Policy, rules []strategy.Rule) {
	unit := &unitAI{rules: rules, control: control, position: position, policy: policy, due: firstDelay}
	r.units[control.GetUnitId()] = unit
	heap.Push(&r.queue, unit)
}

// UnitRemoved 通知调度器清理已死亡单位。
type UnitRemoved struct {
	UnitID string // 死亡单位的战斗实例编号。
}

// ConfiguredRotation 为每个机器人保存独立的循环位置。拒绝时尝试普攻，
// 成功行动才推进一步，同一回执触发的投递重试沿用原选择。
func ConfiguredRotation(sequence []string) Policy {
	index := 0
	var processed uint64
	return func(control *pb.BattleControl, last *pb.BattleInputAck) string {
		if len(sequence) == 0 {
			// 旧配置按已有主动技能顺序出招，无主动技能时使用普攻。
			sequence = control.GetActiveSkills()
			if len(sequence) == 0 {
				return control.GetBasicSkill()
			}
		}
		if last != nil {
			if last.GetCode() != pb_common.ErrCode_OK {
				return control.GetBasicSkill()
			}
			if seq := last.GetSequence(); seq != processed {
				index = (index + 1) % len(sequence)
				processed = seq
			}
		}
		return sequence[index]
	}
}

// Name 返回 Actor 的注册名称。
func (r *RobotActor) Name() string { return "robot-" + r.battleID }

// OnInit 初始化消息路由及运行所需的资源。
func (r *RobotActor) OnInit(ctx tree.Context) { r.registerMessages(); r.initTimer(ctx.Self()) }

// OnStop 停止当前 Actor 的调度资源。
func (r *RobotActor) OnStop(tree.Context) { r.wheel.Stop() }

// HandleMessage 将邮箱消息交给已注册的路由。
func (r *RobotActor) HandleMessage(ctx tree.Context, msg interface{}) { r.router.Route(ctx, msg) }
