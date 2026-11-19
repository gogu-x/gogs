package glconf

import (
	"sort"
	"sync/atomic"
)

// TalentCatalog 保存已校验的天赋目录，返回的配置行和列表供只读使用。
type TalentCatalog struct {
	nodes    []*TalentNodeCfg
	branches []*TalentBranchCfg
	roots    []int32
}

// GetNodes 按编号升序返回天赋节点。
func (catalog *TalentCatalog) GetNodes() []*TalentNodeCfg { return catalog.nodes }

// GetBranches 按编号升序返回天赋分支。
func (catalog *TalentCatalog) GetBranches() []*TalentBranchCfg { return catalog.branches }

// GetRoots 返回所有免费根节点编号。
func (catalog *TalentCatalog) GetRoots() []int32 {
	if catalog == nil {
		return nil
	}
	return catalog.roots
}

var talentCatalog atomic.Pointer[TalentCatalog]
var validTalentRule atomic.Pointer[TalentRuleCfg]

// GetTalentCatalog 返回已校验的天赋目录。
func GetTalentCatalog() *TalentCatalog { return talentCatalog.Load() }

// GetValidTalentRule 返回通过校验的全局天赋规则。
func GetValidTalentRule() *TalentRuleCfg { return validTalentRule.Load() }

func loadTalentRule(table *CsvConf) {
	if table.NumRecord() != 1 {
		validTalentRule.Store(nil)
		return
	}
	rule := table.Record(0).(*TalentRuleCfg)
	if rule.GetBasePoints() < 0 || rule.GetPointsPerLevel() < 0 || rule.GetPointsPerBreakthrough() < 0 ||
		rule.GetMaxPoints() < rule.GetBasePoints() || rule.GetLevelPointStart() < 0 {
		validTalentRule.Store(nil)
		return
	}
	validTalentRule.Store(rule)
}

func loadTalentNodes(nodes *CsvConf) {
	branches, _ := csvTalentBranchCfg.Load().(*CsvConf)
	talentCatalog.Store(buildTalentCatalog(nodes, branches))
}

func loadTalentBranches(branches *CsvConf) {
	nodes, _ := csvTalentNodeCfg.Load().(*CsvConf)
	talentCatalog.Store(buildTalentCatalog(nodes, branches))
}

func buildTalentCatalog(nodes, branches *CsvConf) *TalentCatalog {
	if nodes.NumRecord() == 0 || branches.NumRecord() == 0 {
		return nil
	}
	branchRows := make([]*TalentBranchCfg, 0, branches.NumRecord())
	for index := range branches.NumRecord() {
		branchRows = append(branchRows, branches.Record(index).(*TalentBranchCfg))
	}
	nodeRows := make([]*TalentNodeCfg, 0, nodes.NumRecord())
	var roots []int32
	for index := range nodes.NumRecord() {
		node := nodes.Record(index).(*TalentNodeCfg)
		if node.GetCfgID() < 0 || branches.Get(node.GetBranchID()) == nil || node.GetPointCost() < 0 ||
			node.GetMaxHpPermille() < 0 || node.GetAttackPermille() < 0 ||
			node.GetDefensePermille() < 0 || node.GetSpeedPermille() < 0 || !validTalentPath(node, nodes) {
			return nil
		}
		nodeRows = append(nodeRows, node)
		if node.GetIsRoot() {
			roots = append(roots, node.GetCfgID())
		}
	}
	sort.Slice(nodeRows, func(i, j int) bool { return nodeRows[i].GetCfgID() < nodeRows[j].GetCfgID() })
	sort.Slice(branchRows, func(i, j int) bool { return branchRows[i].GetCfgID() < branchRows[j].GetCfgID() })
	return &TalentCatalog{nodes: nodeRows, branches: branchRows, roots: roots}
}

func validTalentPath(node *TalentNodeCfg, nodes *CsvConf) bool {
	seen := make(map[int32]bool)
	for node != nil {
		id := node.GetCfgID()
		if seen[id] {
			return false
		}
		seen[id] = true
		if node.GetIsRoot() {
			return node.GetPointCost() == 0 && node.GetParentID() < 0
		}
		if node.GetPointCost() <= 0 {
			return false
		}
		node, _ = nodes.Get(node.GetParentID()).(*TalentNodeCfg)
	}
	return false
}
