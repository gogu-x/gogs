package player

import (
	"github.com/gogu-x/gogs/game/play/internal/player/growth"
	"github.com/gogu-x/gogs/game/play/internal/player/souljade"
	"github.com/gogu-x/gogs/game/play/internal/player/talent"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_souljade"
	"github.com/gogu-x/gogs/pb/cspb/pb_talent"
)

// BuildSoulJadeData builds an authoritative snapshot from loaded memory.
func (p *Player) BuildSoulJadeData() *pb_souljade.RoleSoulJadeData {
	roleID, _ := roleState(p)
	return souljade.View(p.GetSoulJadeMgr(), roleID)
}

// BuildTalentData derives talent points from server character progression.
func (p *Player) BuildTalentData() *pb_talent.RoleTalentData {
	roleID, earned := roleState(p)
	return talent.View(p.GetTalentMgr(), roleID, earned)
}

func roleState(p *Player) (string, int32) {
	if p == nil {
		return "", 0
	}
	roleMgr := p.GetRoleMgr()
	if character, ok := roleMgr.GetCharacter(); ok {
		return character.GetID(), talent.EarnedPoints(character.GetLevel(), character.GetBreakthrough())
	}
	return "", 0
}

// BattleBonuses merges only server-owned growth modifiers.
func (p *Player) BattleBonuses() *pb_common.GrowthBonuses {
	result := souljade.Bonuses(p.GetSoulJadeMgr())
	growth.Add(result, talent.Bonuses(p.GetTalentMgr()))
	return result
}
