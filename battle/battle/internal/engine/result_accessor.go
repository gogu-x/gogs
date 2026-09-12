package engine

import pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"

// GetBattleID 返回战斗 ID。
func (r Result) GetBattleID() string { return r.BattleID }

// GetOutcome 返回战斗结果。
func (r Result) GetOutcome() pb.BattleOutcome { return r.Outcome }

// GetUnits 返回单位结果。
func (r Result) GetUnits() []*pb.BattleUnitResult { return r.Units }

// GetEvents 返回事件流。
func (r Result) GetEvents() []*pb.BattleEvent { return r.Events }

// GetChecksum 返回结果校验值。
func (r Result) GetChecksum() string { return r.Checksum }

// GetTickDurationMS 返回客户端播放使用的 tick 毫秒数。
func (r Result) GetTickDurationMS() int32 { return r.TickDurationMS }
