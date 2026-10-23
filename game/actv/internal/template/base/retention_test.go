package base_test

import (
	"testing"
	"time"

	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/trigger"
	"github.com/gogu-x/gogs/glconf"
)

// TestSettlementRetention 验证结算边界、无结束时间以及过期实例脏数据保存后再回收。
func TestSettlementRetention(t *testing.T) {
	now := time.Now().Unix()
	meta := &service.ActvMeta{EndTs: now}
	if meta.IsExpired(now+service.SettlementRetentionSeconds-1) || !meta.IsExpired(now+service.SettlementRetentionSeconds) {
		t.Fatal("settlement deadline must be exactly fourteen days after end")
	}
	if (&service.ActvMeta{}).IsExpired(now) {
		t.Fatal("activity without end time must remain resident")
	}
	svc := service.NewActvService(1)
	svc.RegTpl(testTemplate, &testBehavior{}, service.ScopeServer)
	expired, behavior := newTestActv(
		t, svc, 101, service.ScopeServer, 1,
	)
	svc.SetPlayerLoaded(7)
	behavior.member.Get(7).Score = 10
	behavior.member.MarkDirty(7)
	expired.GetMeta().EndTs = now - service.SettlementRetentionSeconds
	behavior.inst.MarkDirty()
	valid, _ := newTestActv(
		t, svc, 102, service.ScopeServer, 1,
	)
	valid.GetMeta().EndTs = now - service.SettlementRetentionSeconds + 1
	before := svc.ListAllActvs()
	if ids := svc.SharedActivityIDs(0, now); len(ids) != 1 || ids[0] != 102 {
		t.Fatalf("expired activity included in player load: %v", ids)
	}
	svc.RemoveExpiredActivities(now)
	if svc.GetActvByID(101) == nil {
		t.Fatal("dirty expired activity must remain until saved")
	}
	expired.GetMeta().SetDirty(false)
	behavior.inst.ClearDirty(behavior.inst.Revision())
	svc.RemoveExpiredActivities(now)
	if svc.GetActvByID(101) == nil {
		t.Fatal("dirty player component must prevent expiry removal")
	}
	behavior.member.ClearDirty(7, behavior.member.Revision(7))
	svc.RemoveExpiredActivities(now)
	if svc.GetActvByID(101) != nil || svc.GetActvByID(102) == nil || len(svc.ListServerActvs()) != 1 {
		t.Fatal("expiry must remove only expired clean activity from all indexes")
	}
	if len(before) != 2 || before[0] != expired {
		t.Fatal("cleanup must preserve existing iteration views")
	}
}

// TestBatchRemoveActivities 验证跨归属批量删除、重复编号及旧遍历视图。
func TestBatchRemoveActivities(t *testing.T) {
	svc := service.NewActvService(1)
	svc.RegTpl(testTemplate, &testBehavior{}, service.ScopeServer)
	newTestActv(
		t, svc, 1, service.ScopePlayer, 7,
	)
	newTestActv(
		t, svc, 2, service.ScopePlayer, 7,
	)
	newTestActv(
		t, svc, 3, service.ScopePlayer, 8,
	)
	newTestActv(
		t, svc, 4, service.ScopeServer, 1,
	)
	all, owner := svc.ListAllActvs(), svc.ListActvs(service.ScopePlayer, 7)
	svc.RemoveActvs([]int64{1, 2, 1, 999, 4})
	if len(all) != 4 || len(owner) != 2 {
		t.Fatal("batch removal changed existing iteration views")
	}
	if len(svc.ListAllActvs()) != 1 || svc.GetActvByID(3) == nil || svc.GetActvByID(1) != nil || svc.GetActvByID(2) != nil || svc.GetActvByID(4) != nil {
		t.Fatal("batch removal did not keep indexes consistent")
	}
	if svc.ListActvs(service.ScopePlayer, 7) != nil || svc.ListServerActvs() != nil {
		t.Fatal("empty owner indexes must be removed")
	}
}

// TestSettlementRounds 验证配置保留期不影响统一十四天窗口，短周期活动不丢失历史轮次。
func TestSettlementRounds(t *testing.T) {
	plan := &trigger.Plan{CfgID: 1, FirstStartTs: 10000000, Duration: 3600, Reopen: true, RepeatInterval: 3600}
	now := plan.FirstStartTs + trigger.SettlementRetentionSeconds
	rounds := plan.SettlementRounds(now)
	if len(rounds) != 337 || rounds[0].Round != 1 || rounds[len(rounds)-1].Round != 337 {
		t.Fatalf("short cycle settlement rounds missing: count=%d", len(rounds))
	}
	plan.Reopen = false
	for _, retention := range []int64{0, 30 * 86400} {
		plan.CloseAfterEnd = retention
		deadline := plan.FirstStartTs + plan.Duration + trigger.SettlementRetentionSeconds
		if len(plan.SettlementRounds(deadline-1)) != 1 || len(plan.SettlementRounds(deadline)) != 0 {
			t.Fatal("configured close duration changed settlement deadline")
		}
		if plan.CloseAfterEnd != retention {
			t.Fatal("settlement query modified plan")
		}
	}
}

// TestExpiredRoundNotCreated 验证创建路径在十四天截止点排除旧轮次，截止前仍允许补建。
func TestExpiredRoundNotCreated(t *testing.T) {
	now := time.Now().Unix()
	svc := service.NewActvService(1)
	svc.RegTpl(testTemplate, &testBehavior{}, service.ScopeServer)
	plan := &trigger.Plan{
		Cfg:   &glconf.ActvOnlineCfg{ID: 101, Type: int32(testTemplate)},
		CfgID: 101, TemplateType: int32(testTemplate),
		FirstStartTs: now - trigger.SettlementRetentionSeconds - 3600,
		Duration:     3600, CloseAfterEnd: 30 * 86400,
	}
	plans := []*trigger.Plan{plan}
	if created := svc.CreateDueServerActs(plans, now); len(created) != 0 {
		t.Fatal("expired round must not be created even if configuration retains thirty days")
	}
	if created := svc.CreateDueServerActs(plans, now-1); len(created) != 1 {
		t.Fatal("round inside settlement window must be available for creation")
	}
}
