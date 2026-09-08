package engine

import (
	"fmt"
	"reflect"
	"sort"
)

type Replay struct {
	BattleID      string      `json:"battle_id"`
	ConfigVersion string      `json:"config_version"`
	ConfigHash    string      `json:"config_hash"`
	Input         BattleInput `json:"input"`
	Events        []Event     `json:"events"`
	Checksum      string      `json:"checksum"`
}

type checksumStatus struct {
	ID        string `json:"id"`
	Remaining int    `json:"remaining"`
}
type checksumUnit struct {
	ID        string           `json:"id"`
	HP        int64            `json:"hp"`
	Gauge     int64            `json:"gauge"`
	Cooldowns [][2]any         `json:"cooldowns"`
	Statuses  []checksumStatus `json:"statuses"`
}

func (b *Battle) resultChecksum(result Result) (string, error) {
	state := make([]checksumUnit, 0, len(b.units))
	for _, u := range b.units {
		cu := checksumUnit{ID: u.Input.InstanceID, HP: u.HP, Gauge: u.Gauge}
		ids := make([]string, 0, len(u.Cooldowns))
		for id := range u.Cooldowns {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			cu.Cooldowns = append(cu.Cooldowns, [2]any{id, u.Cooldowns[id]})
		}
		for _, s := range u.Statuses {
			cu.Statuses = append(cu.Statuses, checksumStatus{ID: s.Config.ID, Remaining: s.Remaining})
		}
		state = append(state, cu)
	}
	return hashJSON(struct {
		BattleID   string         `json:"battle_id"`
		ConfigHash string         `json:"config_hash"`
		Outcome    Outcome        `json:"outcome"`
		Tick       int64          `json:"tick"`
		Units      []UnitResult   `json:"units"`
		State      []checksumUnit `json:"state"`
		Events     []Event        `json:"events"`
	}{b.id, b.configHash, result.Outcome, result.Tick, result.Units, state, result.Events})
}

func VerifyReplay(repo ConfigRepository, replay Replay) error {
	if replay.BattleID == "" || replay.Checksum == "" {
		return invalid("replay", "battle_id and checksum required")
	}
	if replay.ConfigVersion != replay.Input.ConfigVersion {
		return invalid("replay.config_version", "does not match input")
	}
	config, err := repo.Get(replay.ConfigVersion)
	if err != nil {
		return err
	}
	hash, err := StableConfigHash(config)
	if err != nil {
		return err
	}
	if hash != replay.ConfigHash {
		return fmt.Errorf("replay config hash mismatch")
	}
	battle, err := NewBattle(repo, replay.Input)
	if err != nil {
		return err
	}
	result, err := battle.Run()
	if err != nil {
		return err
	}
	if result.BattleID != replay.BattleID {
		return fmt.Errorf("replay battle id mismatch")
	}
	if result.Checksum != replay.Checksum {
		return fmt.Errorf("replay checksum mismatch")
	}
	if !reflect.DeepEqual(result.Events, replay.Events) {
		return fmt.Errorf("replay event stream mismatch")
	}
	return nil
}
