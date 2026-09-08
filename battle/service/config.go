package service

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gogu-x/gogs/battle/engine"
)

func LoadConfigRepository(path string) (*engine.MemoryConfigRepository, error) {
	repo := engine.NewMemoryConfigRepository()
	if path == "" {
		if err := repo.Put(DefaultConfig()); err != nil {
			return nil, err
		}
		return repo, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var configs []engine.Config
	if err := json.Unmarshal(data, &configs); err != nil {
		var single engine.Config
		if singleErr := json.Unmarshal(data, &single); singleErr != nil {
			return nil, fmt.Errorf("battle config: expected object or array: %w", err)
		}
		configs = []engine.Config{single}
	}
	if len(configs) == 0 {
		return nil, fmt.Errorf("battle config: no versions")
	}
	for _, config := range configs {
		if err := repo.Put(config); err != nil {
			return nil, err
		}
	}
	return repo, nil
}

// DefaultConfig keeps local development and tests self-contained. Production
// can mount immutable versioned JSON through --battle-config.
func DefaultConfig() engine.Config {
	return engine.Config{
		Version:                "default",
		ATBThreshold:           100,
		MaxActions:             100,
		DamageVariancePermille: 0,
		CritChancePermille:     0,
		CritMultiplierPermille: 1500,
		Units: map[string]engine.UnitConfig{
			"hero":  {ID: "hero", MaxHP: 120, Attack: 35, Defense: 8, Speed: 20, BasicSkillID: "attack"},
			"enemy": {ID: "enemy", MaxHP: 100, Attack: 25, Defense: 6, Speed: 15, BasicSkillID: "attack"},
		},
		Skills: map[string]engine.SkillConfig{
			"attack": {ID: "attack", TargetRule: engine.TargetEnemySingle, Effects: []engine.EffectConfig{{Kind: engine.EffectDamage, CoefficientPermille: 1000}}},
		},
		Statuses: map[string]engine.StatusConfig{},
	}
}
