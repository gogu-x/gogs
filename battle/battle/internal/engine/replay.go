package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func resultChecksum(result Result) (string, error) {
	payload, err := json.Marshal(struct {
		BattleID       string `json:"battle_id"`
		Outcome        int32  `json:"outcome"`
		Tick           int64  `json:"tick"`
		TickDurationMS int32  `json:"tick_duration_ms"`
		Units          any    `json:"units"`
		Events         any    `json:"events"`
	}{result.BattleID, int32(result.Outcome), result.Tick, result.TickDurationMS, result.Units, result.Events})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
