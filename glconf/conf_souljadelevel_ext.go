package glconf

import (
	"sort"
	"sync/atomic"
)

var soulJadeInit atomic.Pointer[LoadoutInitCfg]

// GetSoulJadeInit 返回通过校验的魂玉初始化配置。
func GetSoulJadeInit() *LoadoutInitCfg { return soulJadeInit.Load() }

func loadSoulJadeInit(table *CsvConf) {
	if table.NumRecord() != 1 {
		soulJadeInit.Store(nil)
		return
	}
	row := table.Record(0).(*LoadoutInitCfg)
	if row.GetSoulJadeSlotCount() <= 0 || row.GetSoulJadeEssence() < 0 {
		soulJadeInit.Store(nil)
		return
	}
	soulJadeInit.Store(row)
}

// SoulJadeRefineCandidate 表示洗练池中的一个配置候选项。
type SoulJadeRefineCandidate struct {
	ID     int32 // 基础属性或效果词条编号。
	Weight int64 // 抽取权重。
	Effect bool  // 是否为效果词条。
}

// GetID 返回词条编号。
func (candidate SoulJadeRefineCandidate) GetID() int32 { return candidate.ID }

// GetWeight 返回抽取权重。
func (candidate SoulJadeRefineCandidate) GetWeight() int64 { return candidate.Weight }

// GetEffect 判断候选项是否为效果词条。
func (candidate SoulJadeRefineCandidate) GetEffect() bool { return candidate.Effect }

type soulJadeCatalog struct {
	levels  []*SoulJadeLevelCfg
	valid   map[int32]*SoulJadeLevelCfg
	pools   map[int32][]SoulJadeRefineCandidate
	effects map[int32]*SoulJadeEffectAffixCfg
}

func (catalog *soulJadeCatalog) getLevels() []*SoulJadeLevelCfg                { return catalog.levels }
func (catalog *soulJadeCatalog) getValid() map[int32]*SoulJadeLevelCfg         { return catalog.valid }
func (catalog *soulJadeCatalog) getPools() map[int32][]SoulJadeRefineCandidate { return catalog.pools }
func (catalog *soulJadeCatalog) getEffects() map[int32]*SoulJadeEffectAffixCfg {
	return catalog.effects
}

var soulJadeConfig atomic.Pointer[soulJadeCatalog]

func loadSoulJadeLevels(levels *CsvConf) {
	effects, _ := csvSoulJadeEffectAffixCfg.Load().(*CsvConf)
	soulJadeConfig.Store(buildSoulJadeCatalog(levels, effects))
}

func loadSoulJadeEffects(effects *CsvConf) {
	levels, _ := csvSoulJadeLevelCfg.Load().(*CsvConf)
	soulJadeConfig.Store(buildSoulJadeCatalog(levels, effects))
}

func buildSoulJadeCatalog(levels, effects *CsvConf) *soulJadeCatalog {
	effectRows := make(map[int32]*SoulJadeEffectAffixCfg)
	var effectCandidates []SoulJadeRefineCandidate
	allEffectsValid := true
	for index := range effects.NumRecord() {
		effect := effects.Record(index).(*SoulJadeEffectAffixCfg)
		if !ValidSoulJadeEffectAffixCfg(effect) {
			allEffectsValid = false
			continue
		}
		id := effect.GetCfgID()
		effectRows[id] = effect
		effectCandidates = append(effectCandidates, SoulJadeRefineCandidate{ID: id, Weight: int64(effect.GetRollWeight()), Effect: true})
	}
	sort.Slice(effectCandidates, func(i, j int) bool { return effectCandidates[i].GetID() < effectCandidates[j].GetID() })

	rows := make([]*SoulJadeLevelCfg, 0, levels.NumRecord())
	valid := make(map[int32]*SoulJadeLevelCfg)
	pools := make(map[int32][]SoulJadeRefineCandidate)
	for index := range levels.NumRecord() {
		level := levels.Record(index).(*SoulJadeLevelCfg)
		rows = append(rows, level)
		if !validSoulJadeLevel(level) {
			continue
		}
		id := level.GetCfgID()
		valid[id] = level
		if allEffectsValid {
			pool := make([]SoulJadeRefineCandidate, 0, len(level.GetAllowedAttributes())+len(effectCandidates))
			for _, attribute := range level.GetAllowedAttributes() {
				pool = append(pool, SoulJadeRefineCandidate{ID: attribute, Weight: 1})
			}
			pools[id] = append(pool, effectCandidates...)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].GetCfgID() < rows[j].GetCfgID() })
	return &soulJadeCatalog{levels: rows, valid: valid, pools: pools, effects: effectRows}
}

func validSoulJadeLevel(row *SoulJadeLevelCfg) bool {
	if row.GetCfgID() <= 0 || row.GetUpgradeCost() < 0 || row.GetRefineCost() < 0 || row.GetUpgradePermille() < 0 {
		return false
	}
	minimum, maximum := row.GetRefineMinPermille(), row.GetRefineMaxPermille()
	if minimum <= 0 || maximum < minimum || row.GetNewAffixPermille() <= 0 || row.GetNewAffixPermille() > maximum {
		return false
	}
	var seen [5]bool
	attributes := row.GetAllowedAttributes()
	for _, attribute := range attributes {
		if attribute < 1 || attribute > 4 || seen[attribute] {
			return false
		}
		seen[attribute] = true
	}
	count := row.GetAffixCount()
	return count > 0 && int(count) <= len(attributes)
}

// GetValidSoulJadeLevel 返回通过校验的魂玉等级行。
func GetValidSoulJadeLevel(id int32) *SoulJadeLevelCfg {
	if catalog := soulJadeConfig.Load(); catalog != nil {
		return catalog.getValid()[id]
	}
	return nil
}

// GetNextSoulJadeLevel 返回编号大于当前等级的第一条配置。
func GetNextSoulJadeLevel(current int32) *SoulJadeLevelCfg {
	if catalog := soulJadeConfig.Load(); catalog != nil {
		levels := catalog.getLevels()
		index := sort.Search(len(levels), func(i int) bool { return levels[i].GetCfgID() > current })
		if index < len(levels) {
			return levels[index]
		}
	}
	return nil
}

// GetMaxSoulJadeLevel 返回配置的最大等级编号。
func GetMaxSoulJadeLevel() int32 {
	if catalog := soulJadeConfig.Load(); catalog != nil {
		levels := catalog.getLevels()
		if len(levels) > 0 {
			return max(0, levels[len(levels)-1].GetCfgID())
		}
	}
	return 0
}

// GetSoulJadeRefinePool 返回可用于本次抽取的只读候选池。
func GetSoulJadeRefinePool(id int32) []SoulJadeRefineCandidate {
	if catalog := soulJadeConfig.Load(); catalog != nil {
		return catalog.getPools()[id]
	}
	return nil
}

// GetValidSoulJadeEffectAffix 返回通过校验的效果词条。
func GetValidSoulJadeEffectAffix(id int32) *SoulJadeEffectAffixCfg {
	if catalog := soulJadeConfig.Load(); catalog != nil {
		return catalog.getEffects()[id]
	}
	return nil
}
