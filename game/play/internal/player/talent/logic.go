package talent

import (
	"math"
	"sort"

	"github.com/gogu-x/gogs/game/play/internal/player/growth"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_talent"
)

func ruleConfig() *glconf.TalentRuleCfg {
	rules := glconf.GetAllTalentRuleCfg()
	if len(rules) != 1 {
		return nil
	}
	rule := rules[0]
	if rule.GetBasePoints() < 0 || rule.GetPointsPerLevel() < 0 || rule.GetPointsPerBreakthrough() < 0 || rule.GetMaxPoints() < rule.GetBasePoints() || rule.GetLevelPointStart() < 0 {
		return nil
	}
	return rule
}

func catalogReady() bool {
	if ruleConfig() == nil {
		return false
	}
	nodes := glconf.GetAllTalentNodeCfg()
	if len(nodes) == 0 || len(glconf.GetAllTalentBranchCfg()) == 0 {
		return false
	}
	for _, node := range nodes {
		if node.GetCfgID() < 0 || glconf.GetTalentBranchCfgNoErr(node.GetBranchID()) == nil || node.GetPointCost() < 0 {
			return false
		}
		if node.GetMaxHpPermille() < 0 || node.GetAttackPermille() < 0 || node.GetDefensePermille() < 0 || node.GetSpeedPermille() < 0 {
			return false
		}
		seen := make(map[int32]bool)
		cursor := node
		for cursor != nil {
			id := cursor.GetCfgID()
			if seen[id] {
				return false
			}
			seen[id] = true
			if cursor.GetIsRoot() {
				if cursor.GetPointCost() != 0 || cursor.GetParentID() >= 0 {
					return false
				}
				break
			}
			if cursor.GetPointCost() <= 0 {
				return false
			}
			cursor = glconf.GetTalentNodeCfgNoErr(cursor.GetParentID())
			if cursor == nil {
				return false
			}
		}
	}
	return true
}

func rootNodes() map[int32]bool {
	result := make(map[int32]bool)
	for _, node := range glconf.GetAllTalentNodeCfg() {
		if node.GetIsRoot() {
			result[node.GetCfgID()] = true
		}
	}
	return result
}

// EnsureRoots grants newly configured free roots without resetting learned nodes.
func EnsureRoots(mgr *Mgr) bool {
	if mgr == nil || !catalogReady() {
		return false
	}
	learned := mgr.GetLearned()
	if learned == nil {
		learned = make(map[int32]bool)
		mgr.SetLearned(learned)
	}
	changed := false
	for nodeID := range rootNodes() {
		if !learned[nodeID] {
			learned[nodeID] = true
			changed = true
		}
	}
	if changed {
		mgr.SetRevision(mgr.GetRevision() + 1)
	}
	return changed
}

// EarnedPoints derives points exclusively from server progression and rule data.
func EarnedPoints(level, breakthrough int32) int32 {
	rule := ruleConfig()
	if rule == nil {
		return 0
	}
	points := int64(rule.GetBasePoints())
	if level > rule.GetLevelPointStart() {
		points += (int64(level) - int64(rule.GetLevelPointStart())) * int64(rule.GetPointsPerLevel())
	}
	if breakthrough > 0 {
		points += int64(breakthrough) * int64(rule.GetPointsPerBreakthrough())
	}
	if points > int64(rule.GetMaxPoints()) {
		return rule.GetMaxPoints()
	}
	return int32(points)
}

// SpentPoints totals configured costs of learned non-root nodes.
func SpentPoints(mgr *Mgr) int32 {
	var spent int64
	for nodeID, learned := range mgr.GetLearned() {
		node := glconf.GetTalentNodeCfgNoErr(nodeID)
		if learned && node != nil && !node.GetIsRoot() {
			spent += int64(node.GetPointCost())
		}
	}
	if spent > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(spent)
}

// Learn validates the configured prerequisite and point cost.
func Learn(mgr *Mgr, nodeID, earned int32) pb_common.ErrCode {
	if mgr == nil {
		return pb_common.ErrCode_LOADOUT_UNAVAILABLE
	}
	if !catalogReady() {
		return pb_common.ErrCode_CONFIG_UNAVAILABLE
	}
	node := glconf.GetTalentNodeCfgNoErr(nodeID)
	if node == nil {
		return pb_common.ErrCode_INVALID_TALENT_NODE
	}
	learned := mgr.GetLearned()
	if learned[nodeID] {
		return pb_common.ErrCode_TALENT_ALREADY_LEARNED
	}
	if !node.GetIsRoot() && !learned[node.GetParentID()] {
		return pb_common.ErrCode_TALENT_PREREQUISITE_MISSING
	}
	if int64(SpentPoints(mgr))+int64(node.GetPointCost()) > int64(earned) {
		return pb_common.ErrCode_INSUFFICIENT_TALENT_POINTS
	}
	if learned == nil {
		learned = rootNodes()
		mgr.SetLearned(learned)
	}
	learned[nodeID] = true
	mgr.SetRevision(mgr.GetRevision() + 1)
	return pb_common.ErrCode_OK
}

// Reset refunds a configured branch, or all branches for the protocol sentinel.
func Reset(mgr *Mgr, branch int32) pb_common.ErrCode {
	if mgr == nil {
		return pb_common.ErrCode_LOADOUT_UNAVAILABLE
	}
	if !catalogReady() {
		return pb_common.ErrCode_CONFIG_UNAVAILABLE
	}
	if branch != -1 && glconf.GetTalentBranchCfgNoErr(branch) == nil {
		return pb_common.ErrCode_INVALID_TALENT_BRANCH
	}
	mgr.resetBranch(branch)
	mgr.SetRevision(mgr.GetRevision() + 1)
	return pb_common.ErrCode_OK
}

func (mgr *Mgr) resetBranch(branch int32) {
	learned := mgr.GetLearned()
	if learned == nil {
		learned = make(map[int32]bool)
		mgr.SetLearned(learned)
	}
	for nodeID := range learned {
		node := glconf.GetTalentNodeCfgNoErr(nodeID)
		if branch == -1 || (node != nil && node.GetBranchID() == branch) {
			delete(learned, nodeID)
		}
	}
	for nodeID := range rootNodes() {
		learned[nodeID] = true
	}
}

func nodeDefinition(node *glconf.TalentNodeCfg) *pb_talent.TalentNodeDefinition {
	return &pb_talent.TalentNodeDefinition{NodeId: node.GetCfgID(), ParentId: node.GetParentID(), Name: node.GetName(), Effect: node.GetEffect(), BranchId: node.GetBranchID(), UiIndex: node.GetUIIndex(), PointCost: node.GetPointCost(), IsRoot: node.GetIsRoot(), IconKey: node.GetIconKey(), Bonuses: &pb_common.GrowthBonuses{MaxHpPermille: node.GetMaxHpPermille(), AttackPermille: node.GetAttackPermille(), DefensePermille: node.GetDefensePermille(), SpeedPermille: node.GetSpeedPermille()}}
}

// Bonuses aggregates battle modifiers declared by the learned definitions.
func Bonuses(mgr *Mgr) *pb_common.GrowthBonuses {
	result := &pb_common.GrowthBonuses{}
	for nodeID, learned := range mgr.GetLearned() {
		if node := glconf.GetTalentNodeCfgNoErr(nodeID); learned && node != nil {
			growth.Add(result, nodeDefinition(node).GetBonuses())
		}
	}
	return result
}

// View publishes authoritative rules and presentation metadata from configuration.
func View(mgr *Mgr, roleID string, earned int32) *pb_talent.RoleTalentData {
	if mgr == nil || !catalogReady() {
		return nil
	}
	spent := SpentPoints(mgr)
	available := earned - spent
	if available < 0 {
		available = 0
	}
	definitions := glconf.GetAllTalentNodeCfg()
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].GetCfgID() < definitions[j].GetCfgID() })
	nodes := make([]*pb_talent.TalentNodeDefinition, 0, len(definitions))
	owned := make([]int32, 0, len(mgr.GetLearned()))
	learned := mgr.GetLearned()
	for _, definition := range definitions {
		nodes = append(nodes, nodeDefinition(definition))
		if learned[definition.GetCfgID()] {
			owned = append(owned, definition.GetCfgID())
		}
	}
	branchConfigs := glconf.GetAllTalentBranchCfg()
	sort.Slice(branchConfigs, func(i, j int) bool { return branchConfigs[i].GetCfgID() < branchConfigs[j].GetCfgID() })
	branches := make([]*pb_talent.TalentBranchDefinition, 0, len(branchConfigs))
	for _, branch := range branchConfigs {
		branches = append(branches, &pb_talent.TalentBranchDefinition{BranchId: branch.GetCfgID(), Name: branch.GetName(), IconKey: branch.GetIconKey()})
	}
	return &pb_talent.RoleTalentData{Revision: mgr.GetRevision(), EarnedPoints: earned, SpentPoints: spent, AvailablePoints: available, LearnedNodeIds: owned, Nodes: nodes, Bonuses: Bonuses(mgr), RoleId: roleID, Branches: branches}
}
