package talent

import (
	"github.com/gogu-x/gogs/game/play/internal/player/loadouttest"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"testing"
)

func TestTalentPrerequisitesBudgetAndReset(t *testing.T) {
	loadouttest.Load(t)
	mgr := New()
	if code := Learn(mgr, 4, 6); code == pb_common.ErrCode_OK {
		t.Fatal("prerequisite was bypassed")
	}
	for _, nodeID := range []int32{1, 2, 3, 4, 5, 6} {
		if code := Learn(mgr, nodeID, 6); code != pb_common.ErrCode_OK {
			t.Fatal(code)
		}
	}
	if code := Learn(mgr, 7, 6); code == pb_common.ErrCode_OK {
		t.Fatal("point budget was exceeded")
	}
	if code := Learn(mgr, 1, 6); code == pb_common.ErrCode_OK {
		t.Fatal("duplicate node was learned")
	}
	if SpentPoints(mgr) != 6 || Bonuses(mgr).GetAttackPermille() == 0 {
		t.Fatal("incorrect spent points or battle modifiers")
	}
	if code := Reset(mgr, 0); code != pb_common.ErrCode_OK {
		t.Fatal("reset failed")
	}
	if SpentPoints(mgr) != 0 || len(mgr.GetLearned()) != 4 {
		t.Fatal("reset did not refund points or preserve roots")
	}
	if code := Learn(mgr, 25, 6); code != pb_common.ErrCode_OK {
		t.Fatal("learning another branch failed")
	}
	if code := Reset(mgr, 0); code != pb_common.ErrCode_OK {
		t.Fatal("branch reset failed")
	}
	if !mgr.GetLearned()[25] {
		t.Fatal("reset cleared a different branch")
	}
	if code := Reset(mgr, -1); code != pb_common.ErrCode_OK {
		t.Fatal("full reset failed")
	}
	if SpentPoints(mgr) != 0 {
		t.Fatal("full reset did not refund all points")
	}
}

func TestTalentCatalogAndPointRulesAreConfigurable(t *testing.T) {
	loadouttest.Load(t)
	t.Cleanup(func() { loadouttest.Load(t) })
	if err := glconf.SetTalentRuleCfgForTest([]*glconf.TalentRuleCfg{{CfgID: 7, BasePoints: 4, LevelPointStart: 10, PointsPerLevel: 2, PointsPerBreakthrough: 3, MaxPoints: 40}}); err != nil {
		t.Fatal(err)
	}
	if err := glconf.SetTalentBranchCfgForTest([]*glconf.TalentBranchCfg{{CfgID: 9, Name: "configured-branch", IconKey: "branch-icon"}}); err != nil {
		t.Fatal(err)
	}
	if err := glconf.SetTalentNodeCfgForTest([]*glconf.TalentNodeCfg{
		{CfgID: 1000, BranchID: 9, ParentID: -1, IsRoot: true, Name: "configured-root"},
		{CfgID: 2000, BranchID: 9, UIIndex: 5, ParentID: 1000, PointCost: 3, Name: "configured-node", Effect: "configured-effect", IconKey: "node-icon", AttackPermille: 25},
		{CfgID: 3000, BranchID: 9, UIIndex: 8, ParentID: 2000, PointCost: 2, Name: "configured-child", SpeedPermille: 7},
	}); err != nil {
		t.Fatal(err)
	}
	mgr := New()
	if mgr == nil || len(mgr.GetLearned()) != 1 || !mgr.GetLearned()[1000] {
		t.Fatal("configured roots were ignored")
	}
	if EarnedPoints(11, 2) != 12 || EarnedPoints(100, 100) != 40 {
		t.Fatal("configured point formula was ignored")
	}
	if code := Learn(mgr, 3000, 4); code != pb_common.ErrCode_TALENT_PREREQUISITE_MISSING {
		t.Fatal("configured prerequisite was bypassed")
	}
	if code := Learn(mgr, 2000, 4); code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	if SpentPoints(mgr) != 3 || Bonuses(mgr).GetAttackPermille() != 25 {
		t.Fatal("configured cost or modifier was ignored")
	}
	if code := Learn(mgr, 3000, 4); code != pb_common.ErrCode_INSUFFICIENT_TALENT_POINTS {
		t.Fatal("configured point cost was not enforced")
	}
	view := View(mgr, "role", 4)
	if len(view.GetNodes()) != 3 || len(view.GetBranches()) != 1 || view.GetBranches()[0].GetBranchId() != 9 {
		t.Fatal("configured catalog was not published")
	}
	node := view.GetNodes()[1]
	if node.GetUiIndex() != 5 || node.GetPointCost() != 3 || node.GetIconKey() != "node-icon" || node.GetEffect() != "configured-effect" {
		t.Fatal("configured node metadata was ignored")
	}
	if code := Reset(mgr, 9); code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	if SpentPoints(mgr) != 0 || !mgr.GetLearned()[1000] {
		t.Fatal("configured branch reset failed")
	}
}

func TestTalentRejectsBrokenConfiguredPrerequisites(t *testing.T) {
	loadouttest.Load(t)
	t.Cleanup(func() { loadouttest.Load(t) })
	if err := glconf.SetTalentNodeCfgForTest([]*glconf.TalentNodeCfg{{CfgID: 1, BranchID: 0, ParentID: 2, PointCost: 1}, {CfgID: 2, BranchID: 0, ParentID: 1, PointCost: 1}}); err != nil {
		t.Fatal(err)
	}
	if New() != nil {
		t.Fatal("cyclic catalog initialized a playable state")
	}
}

func TestServerProgressionAndNodeCatalog(t *testing.T) {
	loadouttest.Load(t)
	if EarnedPoints(1, 0) != 6 || EarnedPoints(8, 2) != 15 || EarnedPoints(2147483647, 2147483647) != 92 {
		t.Fatal("incorrect authoritative point calculation")
	}
	view := View(New(), "role-100", 6)
	nodes := view.GetNodes()
	if len(nodes) != 96 || view.GetAvailablePoints() != 6 {
		t.Fatal("incorrect catalog or initial points")
	}
	for id, node := range nodes {
		if node.GetNodeId() != int32(id) || node.GetName() == "" || node.GetEffect() == "" {
			t.Fatal("catalog entry missing")
		}
		if id%24 == 0 && node.GetParentId() != -1 {
			t.Fatal("root has a prerequisite")
		}
		if id%24 != 0 && (node.GetParentId()/24 != int32(id)/24 || node.GetParentId() >= int32(id)) {
			t.Fatal("prerequisite has an invalid branch or cycle")
		}
	}
}
