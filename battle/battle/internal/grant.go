package internal

import (
	"fmt"
	"slices"

	"github.com/gogu-x/gogs/battle/battle/internal/battleconfig"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/sspb"
	"google.golang.org/protobuf/proto"
)

// applyGrant 从本场权威角色快照和配置编译增益，成功后才替换快照并广播事件。
func (a *BattleActor) applyGrant(id string, grant *pb.BattleGrant, preview bool) error {
	b := a.p.Battle
	if done, err := b.CheckGrant(id, grant); done || err != nil {
		return err
	}
	compiler := a.p.Compiler
	if compiler == nil {
		compiler = battleconfig.NewDefault()
	}
	if effect, ok := grant.GetEffect().(*pb.BattleGrant_StatusConfigId); ok {
		status, err := compiler.BuildGrantStatus(effect.StatusConfigId)
		if err != nil {
			return err
		}
		if preview {
			return nil
		}
		if err = b.ApplyGrant(id, grant, nil, &status); err != nil {
			return err
		}
	} else {
		role, config, err := compiler.BuildGrantRole(a.roles[id], grant)
		if err != nil {
			return err
		}
		if preview {
			return nil
		}
		if err = b.ApplyGrant(id, grant, &config, nil); err != nil {
			return err
		}
		a.roles[id] = role
	}
	a.pushLiveEvents(b.Flush())
	a.scheduleStatus()
	return nil
}

// issueGrant 将玩法发放直接入账，或者只登记为拥有者可领取的掉落。
func (a *BattleActor) issueGrant(owner Member, req *sspb.ApplyBattleGrantReq) error {
	unitID, grant := req.GetUnitId(), req.GetGrant()
	if done, err := a.p.Battle.CheckGrant(unitID, grant); done || err != nil {
		return err
	}
	for _, offer := range a.offers[unitID] {
		if offer.GetGrantId() != grant.GetGrantId() {
			continue
		}
		if !req.GetRequirePickup() || !proto.Equal(offer, grant) {
			return fmt.Errorf("发放编号与待领取记录冲突")
		}
		a.sendSnapshot(owner, true)
		return nil
	}
	preview := req.GetRequirePickup()
	if err := a.applyGrant(unitID, grant, preview); err != nil {
		return err
	}
	if preview {
		a.offers[unitID] = append(a.offers[unitID], proto.Clone(grant).(*pb.BattleGrant))
		a.sendSnapshot(owner, true)
	}
	return nil
}

// claimGrant 按本单位的候选编号领取；同一编号重试仅确认此前结果。
func (a *BattleActor) claimGrant(unitID, id string) error {
	offers := a.offers[unitID]
	for i, offer := range offers {
		if offer.GetGrantId() != id {
			continue
		}
		if err := a.applyGrant(unitID, offer, false); err != nil {
			return err
		}
		a.offers[unitID] = slices.Delete(offers, i, i+1)
		return nil
	}
	if a.p.Battle.ReceivedGrant(unitID, id) != nil {
		return nil
	}
	return fmt.Errorf("找不到可领取的增益")
}
