package glconf

import (
	"fmt"
	"math"
	"sync/atomic"

	"github.com/gogu-x/gogs/pb/cspb/pb_common"
)

// BattleMonsterMember 保存怪物编组成员和加载时发现的配置错误。
type BattleMonsterMember struct {
	id, position int32
	err          error
}

// GetID 返回怪物配置编号。
func (member BattleMonsterMember) GetID() int32 { return member.id }

// GetPosition 返回怪物站位。
func (member BattleMonsterMember) GetPosition() int32 { return member.position }

// GetError 返回该成员的配置错误。
func (member BattleMonsterMember) GetError() error { return member.err }

type battleMonsterGroups map[int32][]BattleMonsterMember

var preparedBattleMonsterGroups atomic.Pointer[battleMonsterGroups]

func loadBattleMonsterGroups(table *CsvConf) {
	groups := make(battleMonsterGroups, table.NumRecord())
	for index := range table.NumRecord() {
		row := table.Record(index).(*BattleMonsterGroupCfg)
		groupID := row.getID()
		members := row.getMembers()
		result := make([]BattleMonsterMember, 0, len(members))
		positions := make(map[int32]bool)
		for _, raw := range members {
			member := parseBattleMonsterMember(raw)
			position := member.GetPosition()
			if err := member.GetError(); err != nil {
				member.err = fmt.Errorf("battle monster group config id=%d: %w", groupID, err)
			} else if positions[position] {
				member.err = fmt.Errorf("battle monster group config id=%d has duplicate position=%d", groupID, position)
			}
			positions[position] = true
			result = append(result, member)
		}
		groups[groupID] = result
	}
	preparedBattleMonsterGroups.Store(&groups)
}

func (row *BattleMonsterGroupCfg) getID() int32                      { return row.CfgID }
func (row *BattleMonsterGroupCfg) getMembers() []*pb_common.TypIDVal { return row.Members }

func parseBattleMonsterMember(row *pb_common.TypIDVal) BattleMonsterMember {
	if row == nil {
		return BattleMonsterMember{err: fmt.Errorf("has nil monster member")}
	}
	if row.GetTyp() != "battle_monster" {
		return BattleMonsterMember{err: fmt.Errorf("has invalid monster member type=%q", row.GetTyp())}
	}
	id, position := row.GetId(), row.GetVal()
	if id == 0 {
		return BattleMonsterMember{err: fmt.Errorf("has empty monster config id")}
	}
	if position <= 0 || position > math.MaxInt32 {
		return BattleMonsterMember{err: fmt.Errorf("has invalid monster position=%d", position)}
	}
	return BattleMonsterMember{id: id, position: int32(position)}
}

// GetBattleMonsterMembers 返回按配置顺序处理好的怪物成员，供只读使用。
func GetBattleMonsterMembers(id int32) ([]BattleMonsterMember, error) {
	if groups := preparedBattleMonsterGroups.Load(); groups != nil {
		if members, ok := (*groups)[id]; ok {
			if len(members) == 0 {
				return nil, fmt.Errorf("battle monster group config id=%d has no members", id)
			}
			return members, nil
		}
	}
	return nil, fmt.Errorf("battle monster group config not found: group_config_id=%d", id)
}
