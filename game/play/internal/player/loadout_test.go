package player

import (
	"github.com/gogu-x/gogs/game/play/internal/player/loadouttest"
	"github.com/gogu-x/gogs/glconf"
	"go.mongodb.org/mongo-driver/v2/bson"
	"testing"
)

func TestLoadoutMigrationOnlyGrantsOnce(t *testing.T) {
	loadouttest.Load(t)
	p := &Player{UID: 99}
	if !p.EnsureLoadoutModules() || !p.IsDirty() {
		t.Fatal("legacy document was not migrated during load")
	}
	jade := p.GetSoulJadeMgr()
	jade.SetEssence(17)
	jade.SetSlots(map[int32]string{})
	p.SetDirty(false)
	if p.EnsureLoadoutModules() || p.IsDirty() {
		t.Fatal("migration changed an initialized document")
	}
	if jade.GetEssence() != 17 || len(jade.GetSlots()) != 0 {
		t.Fatal("migration regranted materials or equipped removed items")
	}
}

func TestConfigMigrationPreservesInventoryAndLearntNodes(t *testing.T) {
	loadouttest.Load(t)
	t.Cleanup(func() { loadouttest.Load(t) })
	p := NewPlayerData(99)
	jade := p.GetSoulJadeMgr()
	jade.SetEssence(37)
	talents := p.GetTalentMgr()
	talents.GetLearned()[1] = true
	config := *glconf.GetAllLoadoutInitCfg()[0]
	config.SoulJadeSlotCount = 2
	if err := glconf.SetLoadoutInitCfgForTest([]*glconf.LoadoutInitCfg{&config}); err != nil {
		t.Fatal(err)
	}
	nodes := glconf.GetAllTalentNodeCfg()
	nodes = append(nodes, &glconf.TalentNodeCfg{CfgID: 999, BranchID: 0, ParentID: -1, IsRoot: true, Name: "new-root"})
	if err := glconf.SetTalentNodeCfgForTest(nodes); err != nil {
		t.Fatal(err)
	}
	p.SetDirty(false)
	if !p.EnsureLoadoutModules() || !p.IsDirty() {
		t.Fatal("configured migration was not persisted")
	}
	if len(jade.GetSlots()) != 2 || len(jade.GetItems()) != 10 || jade.GetEssence() != 37 {
		t.Fatal("migration regranted or removed inventory")
	}
	if !talents.GetLearned()[999] || !talents.GetLearned()[1] {
		t.Fatal("migration failed to add roots or preserve learned nodes")
	}
}

func TestLoadoutBSONRoundTripAndSnapshotIsolation(t *testing.T) {
	loadouttest.Load(t)
	p := NewPlayerData(99)
	jade := p.GetSoulJadeMgr()
	jade.SetEssence(1234)
	jade.SetRevision(9)
	p.GetTalentMgr().GetLearned()[1] = true
	p.GetTalentMgr().SetRevision(4)
	encoded, err := bson.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var loaded Player
	if err := bson.Unmarshal(encoded, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.EnsureLoadoutModules() {
		t.Fatal("BSON roundtrip must not regrant inventory")
	}
	loadedJade := loaded.GetSoulJadeMgr()
	if loadedJade.GetEssence() != 1234 || loadedJade.GetRevision() != 9 || !loaded.GetTalentMgr().GetLearned()[1] {
		t.Fatal("BSON roundtrip lost growth state")
	}
	snapshot := p.DeepCopy()
	copyJade := snapshot.GetSoulJadeMgr()
	id := copyJade.GetSlots()[1]
	copyJade.GetItems()[id].GetAffixes()[0].SetPermille(999)
	delete(copyJade.GetSlots(), 1)
	snapshot.GetTalentMgr().GetLearned()[2] = true
	if jade.GetItems()[id].GetAffixes()[0].GetPermille() == 999 || jade.GetSlots()[1] == "" || p.GetTalentMgr().GetLearned()[2] {
		t.Fatal("snapshot aliases runtime state")
	}
}
