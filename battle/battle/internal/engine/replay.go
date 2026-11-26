package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// resultChecksum 根据战斗结果计算用于回放比对的校验值。
func resultChecksum(result Result) (string, error) {
	payload, err := json.Marshal(struct {
		BattleID   string `json:"battle_id"`
		BattleType int32  `json:"battle_type"`
		Outcome    int32  `json:"outcome"`
		ElapsedMs  int64  `json:"elapsed_ms"`
		Units      any    `json:"units"`
		Events     any    `json:"events"`
	}{result.BattleID, int32(result.BattleType), int32(result.Outcome), result.ElapsedMs, result.Units, result.Events})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
