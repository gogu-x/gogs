package service

import (
	"fmt"

	"github.com/gogu-x/gogs/battle/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

func InputFromProto(in *pb.BattleInput) (engine.BattleInput, error) {
	if in == nil {
		return engine.BattleInput{}, fmt.Errorf("battle service: input is required")
	}
	out := engine.BattleInput{ConfigVersion: in.ConfigVersion, Seed: in.Seed, Combatants: make([]engine.CombatantInput, 0, len(in.Combatants))}
	for _, unit := range in.Combatants {
		if unit == nil {
			return engine.BattleInput{}, fmt.Errorf("battle service: nil combatant")
		}
		out.Combatants = append(out.Combatants, engine.CombatantInput{InstanceID: unit.InstanceId, ConfigID: unit.UnitConfigId, Team: engine.Team(unit.Team), Position: int(unit.Position)})
	}
	return out, nil
}

func inputToProto(in engine.BattleInput) *pb.BattleInput {
	out := &pb.BattleInput{ConfigVersion: in.ConfigVersion, Seed: in.Seed, Combatants: make([]*pb.BattleCombatantInput, 0, len(in.Combatants))}
	for _, unit := range in.Combatants {
		out.Combatants = append(out.Combatants, &pb.BattleCombatantInput{InstanceId: unit.InstanceID, UnitConfigId: unit.ConfigID, Team: pb.BattleTeam(unit.Team), Position: int32(unit.Position)})
	}
	return out
}

func eventToProto(event engine.Event) *pb.BattleEvent {
	return &pb.BattleEvent{Sequence: event.Sequence, Action: event.Action, Tick: event.Tick, Type: pb.BattleEventType(event.Type), ActorId: event.ActorID, TargetId: event.TargetID, SkillId: event.SkillID, StatusId: event.StatusID, Amount: event.Amount, HpBefore: event.HPBefore, HpAfter: event.HPAfter, Detail: event.Detail}
}

func eventFromProto(event *pb.BattleEvent) engine.Event {
	if event == nil {
		return engine.Event{}
	}
	return engine.Event{Sequence: event.Sequence, Action: event.Action, Tick: event.Tick, Type: engine.EventType(event.Type), ActorID: event.ActorId, TargetID: event.TargetId, SkillID: event.SkillId, StatusID: event.StatusId, Amount: event.Amount, HPBefore: event.HpBefore, HPAfter: event.HpAfter, Detail: event.Detail}
}

func unitToProto(unit engine.UnitResult) *pb.BattleUnitResult {
	return &pb.BattleUnitResult{InstanceId: unit.InstanceID, Team: pb.BattleTeam(unit.Team), Hp: unit.HP, MaxHp: unit.MaxHP, Alive: unit.Alive}
}

func unitFromProto(unit *pb.BattleUnitResult) engine.UnitResult {
	if unit == nil {
		return engine.UnitResult{}
	}
	return engine.UnitResult{InstanceID: unit.InstanceId, Team: engine.Team(unit.Team), HP: unit.Hp, MaxHP: unit.MaxHp, Alive: unit.Alive}
}

func replayToProto(replay engine.Replay) *pb.BattleReplay {
	out := &pb.BattleReplay{BattleId: replay.BattleID, ConfigVersion: replay.ConfigVersion, ConfigHash: replay.ConfigHash, Input: inputToProto(replay.Input), Checksum: replay.Checksum, Events: make([]*pb.BattleEvent, 0, len(replay.Events))}
	for _, event := range replay.Events {
		out.Events = append(out.Events, eventToProto(event))
	}
	return out
}

func replayFromProto(replay *pb.BattleReplay) (engine.Replay, error) {
	if replay == nil {
		return engine.Replay{}, fmt.Errorf("battle service: replay is required")
	}
	input, err := InputFromProto(replay.Input)
	if err != nil {
		return engine.Replay{}, err
	}
	out := engine.Replay{BattleID: replay.BattleId, ConfigVersion: replay.ConfigVersion, ConfigHash: replay.ConfigHash, Input: input, Checksum: replay.Checksum, Events: make([]engine.Event, 0, len(replay.Events))}
	for _, event := range replay.Events {
		out.Events = append(out.Events, eventFromProto(event))
	}
	return out, nil
}

func ResultToProto(result engine.Result) *pb.StartBattleAck {
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

func ResultFromFinished(msg *pb.BattleFinishedNtf, events []engine.Event) (engine.Result, error) {
	if msg == nil || msg.BattleId == "" {
		return engine.Result{}, fmt.Errorf("battle client: invalid finished message")
	}
	replay, err := replayFromProto(msg.Replay)
	if err != nil {
		return engine.Result{}, err
	}
	out := engine.Result{BattleID: msg.BattleId, Outcome: engine.Outcome(msg.Outcome), Checksum: msg.Checksum, Replay: replay, Events: append([]engine.Event(nil), events...), Units: make([]engine.UnitResult, 0, len(msg.Units))}
	for _, unit := range msg.Units {
		out.Units = append(out.Units, unitFromProto(unit))
	}
	return out, nil
}
