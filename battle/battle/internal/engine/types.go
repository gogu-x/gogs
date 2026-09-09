package engine

import (
	"fmt"

	. "github.com/gogu-x/gogs/battle/iproto"
)

type EventType uint8

const (
	EventActionStarted EventType = iota + 1
	EventSkillUsed
	EventDamage
	EventHeal
	EventStatusApplied
	EventActionSkipped
	EventActionEnded
	EventBattleEnded
)

type Event struct {
	Sequence uint64    `json:"sequence"`
	Action   uint64    `json:"action"`
	Tick     int64     `json:"tick"`
	Type     EventType `json:"type"`
	ActorID  string    `json:"actor_id,omitempty"`
	TargetID string    `json:"target_id,omitempty"`
	SkillID  string    `json:"skill_id,omitempty"`
	StatusID string    `json:"status_id,omitempty"`
	Amount   int64     `json:"amount,omitempty"`
	HPBefore int64     `json:"hp_before,omitempty"`
	HPAfter  int64     `json:"hp_after,omitempty"`
	Detail   string    `json:"detail,omitempty"`
}

type Outcome uint8

const (
	OutcomeAttackerWin Outcome = iota + 1
	OutcomeDefenderWin
	OutcomeDraw
)

type UnitResult struct {
	InstanceID string `json:"instance_id"`
	Team       Team   `json:"team"`
	HP         int64  `json:"hp"`
	MaxHP      int64  `json:"max_hp"`
	Alive      bool   `json:"alive"`
}

type Result struct {
	BattleID string       `json:"battle_id"`
	Outcome  Outcome      `json:"outcome"`
	Tick     int64        `json:"tick"`
	Units    []UnitResult `json:"units"`
	Events   []Event      `json:"events"`
	Checksum string       `json:"checksum"`
	Replay   Replay       `json:"replay"`
}

func invalid(field, reason string) error { return fmt.Errorf("invalid %s: %s", field, reason) }
