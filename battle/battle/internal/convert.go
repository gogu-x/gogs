package internal

import (
	"fmt"

	engine2 "github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/battle/iproto"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// InputFromProto 把 pb 输入还原为引擎输入（manager 建战/校验时使用）。
func InputFromProto(in *pb.BattleInput) (iproto.BattleInput, error) {
	if in == nil {
		return iproto.BattleInput{}, fmt.Errorf("battle service: input is required")
	}
	out := iproto.BattleInput{ConfigVersion: in.ConfigVersion, Seed: in.Seed, Combatants: make([]iproto.CombatantInput, 0, len(in.Combatants))}
	for _, unit := range in.Combatants {
		if unit == nil {
			return iproto.BattleInput{}, fmt.Errorf("battle service: nil combatant")
		}
		out.Combatants = append(out.Combatants, iproto.CombatantInput{InstanceID: unit.InstanceId, ConfigID: unit.UnitConfigId, Team: iproto.Team(unit.Team), Position: int(unit.Position)})
	}
	return out, nil
}

func inputToProto(in iproto.BattleInput) *pb.BattleInput {
	out := &pb.BattleInput{ConfigVersion: in.ConfigVersion, Seed: in.Seed, Combatants: make([]*pb.BattleCombatantInput, 0, len(in.Combatants))}
	for _, unit := range in.Combatants {
		out.Combatants = append(out.Combatants, &pb.BattleCombatantInput{InstanceId: unit.InstanceID, UnitConfigId: unit.ConfigID, Team: pb.BattleTeam(unit.Team), Position: int32(unit.Position)})
	}
	return out
}

func eventToProto(event engine2.Event) *pb.BattleEvent {
	return &pb.BattleEvent{Sequence: event.Sequence, Action: event.Action, Tick: event.Tick, Type: pb.BattleEventType(event.Type), ActorId: event.ActorID, TargetId: event.TargetID, SkillId: event.SkillID, StatusId: event.StatusID, Amount: event.Amount, HpBefore: event.HPBefore, HpAfter: event.HPAfter, Detail: event.Detail}
}

func eventFromProto(event *pb.BattleEvent) engine2.Event {
	if event == nil {
		return engine2.Event{}
	}
	return engine2.Event{Sequence: event.Sequence, Action: event.Action, Tick: event.Tick, Type: engine2.EventType(event.Type), ActorID: event.ActorId, TargetID: event.TargetId, SkillID: event.SkillId, StatusID: event.StatusId, Amount: event.Amount, HPBefore: event.HpBefore, HPAfter: event.HpAfter, Detail: event.Detail}
}

func unitToProto(unit engine2.UnitResult) *pb.BattleUnitResult {
	return &pb.BattleUnitResult{InstanceId: unit.InstanceID, Team: pb.BattleTeam(unit.Team), Hp: unit.HP, MaxHp: unit.MaxHP, Alive: unit.Alive}
}

func unitFromProto(unit *pb.BattleUnitResult) engine2.UnitResult {
	if unit == nil {
		return engine2.UnitResult{}
	}
	return engine2.UnitResult{InstanceID: unit.InstanceId, Team: iproto.Team(unit.Team), HP: unit.Hp, MaxHP: unit.MaxHp, Alive: unit.Alive}
}

func replayToProto(replay engine2.Replay) *pb.BattleReplay {
	out := &pb.BattleReplay{BattleId: replay.BattleID, ConfigVersion: replay.ConfigVersion, ConfigHash: replay.ConfigHash, Input: inputToProto(replay.Input), Checksum: replay.Checksum, Events: make([]*pb.BattleEvent, 0, len(replay.Events))}
	for _, event := range replay.Events {
		out.Events = append(out.Events, eventToProto(event))
	}
	return out
}

// ReplayFromProto 把 pb 回放还原为引擎 Replay（manager 校验/重建时使用）。
func ReplayFromProto(replay *pb.BattleReplay) (engine2.Replay, error) {
	if replay == nil {
		return engine2.Replay{}, fmt.Errorf("battle service: replay is required")
	}
	input, err := InputFromProto(replay.Input)
	if err != nil {
		return engine2.Replay{}, err
	}
	out := engine2.Replay{BattleID: replay.BattleId, ConfigVersion: replay.ConfigVersion, ConfigHash: replay.ConfigHash, Input: input, Checksum: replay.Checksum, Events: make([]engine2.Event, 0, len(replay.Events))}
	for _, event := range replay.Events {
		out.Events = append(out.Events, eventFromProto(event))
	}
	return out, nil
}

// ResultToProto 把结算结果序列化为 pb（manager 查询回历史时使用）。
func ResultToProto(result engine2.Result) *pb.StartBattleAck {
	out := &pb.StartBattleAck{BattleId: result.BattleID, Outcome: pb.BattleOutcome(result.Outcome), Checksum: result.Checksum, Replay: replayToProto(result.Replay), Units: make([]*pb.BattleUnitResult, 0, len(result.Units)), Events: make([]*pb.BattleEvent, 0, len(result.Events))}
	for _, unit := range result.Units {
		out.Units = append(out.Units, unitToProto(unit))
	}
	for _, event := range result.Events {
		out.Events = append(out.Events, eventToProto(event))
	}
	return out
}

func finishedToProto(report Report) *pb.BattleFinishedNtf {
	out := &pb.BattleFinishedNtf{BattleId: report.BattleID, Uid: report.UID, Outcome: pb.BattleOutcome(report.Result.Outcome), Checksum: report.Result.Checksum, Replay: replayToProto(report.Result.Replay), Units: make([]*pb.BattleUnitResult, 0, len(report.Result.Units))}
	for _, unit := range report.Result.Units {
		out.Units = append(out.Units, unitToProto(unit))
	}
	return out
}

func resultFromFinished(msg *pb.BattleFinishedNtf, events []engine2.Event) (engine2.Result, error) {
	if msg == nil || msg.BattleId == "" {
		return engine2.Result{}, fmt.Errorf("battle client: invalid finished message")
	}
	replay, err := ReplayFromProto(msg.Replay)
	if err != nil {
		return engine2.Result{}, err
	}
	out := engine2.Result{BattleID: msg.BattleId, Outcome: engine2.Outcome(msg.Outcome), Checksum: msg.Checksum, Replay: replay, Events: append([]engine2.Event(nil), events...), Units: make([]engine2.UnitResult, 0, len(msg.Units))}
	for _, unit := range msg.Units {
		out.Units = append(out.Units, unitFromProto(unit))
	}
	return out, nil
}
