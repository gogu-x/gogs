package glconf

import (
	"strings"
	"testing"
)

func installBattleTestConfigs(t *testing.T) {
	t.Helper()
	err := SetBattleConfigsForTest(
		[]*BattleRoleCfg{{CfgID: 1, MaxHP: 100, Attack: 10, Defense: 5, Speed: 10, DefaultPosition: 1, BasicSkillID: 1}},
		[]*BattleSkillCfg{{CfgID: 1, TargetRule: 1, Effects: []BattleEffectCfg{{Kind: 1, CoefficientPermille: 1000}}}},
		[]*BattleEquipCfg{{CfgID: 1, Modifier: BattleAttributeModifierCfg{Attack: 5}}},
		[]*BattleMonsterCfg{{CfgID: 1, MaxHP: 50, Attack: 8, Defense: 3, Speed: 7, BasicSkillID: 1}},
		[]*BattleMonsterGroupCfg{{CfgID: 1, Members: []BattleMonsterGroupMemberCfg{{MonsterCfgID: 1, Position: 2}}}},
		[]*BattleRuleCfg{{BattleType: 1, ATBThreshold: 1000, MaxActions: 100, CritMultiplierPermille: 1500}},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBattleConfigGetters(t *testing.T) {
	installBattleTestConfigs(t)
	if cfg := GetBattleRoleCfg(1); cfg == nil || cfg.GetCfgID() != 1 {
		t.Fatalf("role config = %#v", cfg)
	}
	if cfg := GetBattleSkillCfg(1); cfg == nil || cfg.GetCfgID() != 1 {
		t.Fatalf("skill config = %#v", cfg)
	}
	if cfg := GetBattleEquipCfg(1); cfg == nil || cfg.GetCfgID() != 1 {
		t.Fatalf("equip config = %#v", cfg)
	}
	if cfg := GetBattleMonsterCfg(1); cfg == nil || cfg.GetCfgID() != 1 {
		t.Fatalf("monster config = %#v", cfg)
	}
	if cfg := GetBattleMonsterGroupCfg(1); cfg == nil || cfg.GetCfgID() != 1 {
		t.Fatalf("group config = %#v", cfg)
	}
	if cfg := GetBattleRuleCfg(1); cfg == nil || cfg.GetBattleType() != 1 {
		t.Fatalf("rule config = %#v", cfg)
	}
}

func TestValidateBattleMonsterGroupCfg(t *testing.T) {
	installBattleTestConfigs(t)
	tests := []struct {
		name    string
		group   *BattleMonsterGroupCfg
		wantErr string
	}{
		{name: "valid", group: GetBattleMonsterGroupCfg(1)},
		{name: "missing group", wantErr: "required"},
		{name: "empty members", group: &BattleMonsterGroupCfg{CfgID: 2}, wantErr: "no members"},
		{name: "missing monster", group: &BattleMonsterGroupCfg{CfgID: 3, Members: []BattleMonsterGroupMemberCfg{{MonsterCfgID: 2, Position: 1}}}, wantErr: "missing monster"},
		{name: "duplicate position", group: &BattleMonsterGroupCfg{CfgID: 4, Members: []BattleMonsterGroupMemberCfg{{MonsterCfgID: 1, Position: 1}, {MonsterCfgID: 1, Position: 1}}}, wantErr: "duplicate position"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBattleMonsterGroupCfg(tt.group)
			if tt.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
