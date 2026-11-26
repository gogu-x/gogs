package engine

import (
	"fmt"
	"math"
	"math/bits"
	"slices"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"google.golang.org/protobuf/proto"
)

// CheckGrant 校验目标及发放编号，区分已处理的重投和编号冲突。
func (b *Battle) CheckGrant(id string, grant *pb.BattleGrant) (bool, error) {
	state := b.inputs[id]
	if state == nil || grant.GetGrantId() == "" || grant.GetEffect() == nil {
		return false, fmt.Errorf("无效的目标或发放编号")
	}
	if received := b.ReceivedGrant(id, grant.GetGrantId()); received != nil {
		if proto.Equal(received, grant) {
			return true, nil
		}
		return false, fmt.Errorf("发放编号与既有记录冲突")
	}
	if b.done || b.finished || !state.actor.alive() {
		return false, fmt.Errorf("目标已退场")
	}
	return false, nil
}

// ApplyGrant 在本场邮箱中应用已编译的增益，保留伤血、技能冷却、状态及行动进度。
func (b *Battle) ApplyGrant(
	id string, grant *pb.BattleGrant, config *UnitConfig, status *StatusConfig,
) error {
	if done, err := b.CheckGrant(id, grant); done || err != nil {
		return err
	}
	state := b.inputs[id]
	actor := state.actor
	before, interval := actor.HP, b.attackInterval(actor)
	if status != nil {
		if err := b.applyStatus(actor, actor, "", *status); err != nil {
			return err
		}
	} else {
		if err := b.updateGrantLoadout(actor, config); err != nil {
			return err
		}
		b.adjustReady(state, interval)
	}
	received := proto.Clone(grant).(*pb.BattleGrant)
	actor.grants = append(actor.grants, received)
	event := b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_GRANT_APPLIED, id, id, "", "", 0, before, actor.HP, eventInfo{}, nil)
	event.Grant = received
	event.UpdatedUnit = actor.healthSnapshot()
	event.UpdatedControl = b.control(actor)
	return nil
}

// updateGrantLoadout 校验装备技能与原有被动的兼容性，通过后原子替换战中属性。
func (b *Battle) updateGrantLoadout(actor *unit, config *UnitConfig) error {
	next := actor.Config
	if config == nil || config.MaxHP < next.MaxHP || config.Speed < 1 || config.Attack < 0 || config.Defense < 0 {
		return fmt.Errorf("无效的角色增益属性")
	}
	missing := next.MaxHP - actor.HP
	next.MaxHP, next.Attack, next.Defense, next.Speed = config.MaxHP, config.Attack, config.Defense, config.Speed
	next.ActiveSkills = config.ActiveSkills
	setup := b.setup
	setup.Units = slices.Clone(setup.Units)
	for i, unit := range setup.Units {
		if unit.InstanceID == next.InstanceID {
			setup.Units[i] = next
			break
		}
	}
	if err := setup.validate(); err != nil {
		return err
	}
	b.setup = setup
	actor.Config = next
	actor.HP = next.MaxHP - missing
	return nil
}

// adjustReady 按当前等待进度折算剩余时间并向上取整，乘积使用双字宽避免溢出。
func (b *Battle) adjustReady(state *inputState, old int64) {
	if state == nil || state.ready <= b.elapsedMS {
		return
	}
	interval := b.attackInterval(state.actor)
	if old == interval {
		return
	}
	remaining := uint64(state.ready - b.elapsedMS)
	hi, lo := bits.Mul64(remaining, uint64(interval))
	limit := uint64(math.MaxInt64 - b.elapsedMS)
	if hi >= uint64(old) {
		state.ready = math.MaxInt64
		return
	}
	wait, rem := bits.Div64(hi, lo, uint64(old))
	if wait >= limit {
		state.ready = math.MaxInt64
		return
	}
	if rem != 0 {
		wait++
	}
	state.ready = b.elapsedMS + int64(wait)
}

// ReceivedGrant 查找当前单位已入账的发放记录。
func (b *Battle) ReceivedGrant(unitID, id string) *pb.BattleGrant {
	if state := b.inputs[unitID]; state != nil {
		for _, received := range state.actor.grants {
			if received.GetGrantId() == id {
				return received
			}
		}
	}
	return nil
}

// syncStatusSpeed 同步玩家状态变动导致的行动时间变化，供客户端重排行动。
func (b *Battle) syncStatusSpeed(actor *unit, old int64) {
	state := b.inputs[actor.Config.InstanceID]
	if state == nil || actor.Config.Team != pb.BattleTeam_BATTLE_TEAM_ATTACKER || old == b.attackInterval(actor) {
		return
	}
	b.adjustReady(state, old)
	event := b.events[len(b.events)-1]
	event.UpdatedControl = b.control(actor)
}
