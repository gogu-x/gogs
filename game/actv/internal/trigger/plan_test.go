package trigger

import (
	"bufio"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/tree/tlog"
)

func TestMain(m *testing.M) {
	logger, err := tlog.New("error", "", tlog.NoFlags)
	if err != nil {
		panic(err)
	}
	tlog.Log = logger
	os.Exit(m.Run())
}

var testLoc = time.FixedZone("UTC+8", 8*3600)

func mustTs(t *testing.T, value string) int64 {
	t.Helper()
	ts, err := time.ParseInLocation(dateLayout, value, testLoc)
	if err != nil {
		t.Fatal(err)
	}
	return ts.Unix()
}

func TestParseDuration(t *testing.T) {
	cases := map[string]int64{"": 0, "0": 0, "0h": 0, "7d": 7 * 86400, "24h": 86400, "30m": 1800, "496798s": 496798, " 3d ": 3 * 86400}
	for input, want := range cases {
		got, err := ParseDuration(input)
		if err != nil || got != want {
			t.Fatalf("%q => %d, %v; want %d", input, got, err, want)
		}
	}
	for _, bad := range []string{"7", "d", "-1d", "1w", "1.5d"} {
		if _, err := ParseDuration(bad); err == nil {
			t.Fatalf("%q should fail", bad)
		}
	}
}

// TestAbsoluteTrigger 绝对日期、不循环：开始、结束、过期时间与活跃轮次。
func TestAbsoluteTrigger(t *testing.T) {
	cfg := &glconf.ActvOnlineCfg{ID: 1050202, Available: true, Type: 37, TriggerType: 1,
		TriggerVal: "2026-09-25 00:00:00", Duration: "7d", CloseAfterEnd: "24h", ForecastBeforeStart: "1h"}
	plan, err := NewPlan(cfg, 0, testLoc)
	if err != nil {
		t.Fatal(err)
	}
	start := mustTs(t, "2026-09-25 00:00:00")
	round := plan.RoundAt(0)
	if round.Round != 1 || round.StartTs != start || round.ShowTs != start-3600 ||
		round.EndTs != start+7*86400 || round.ExpireTs != start+8*86400 {
		t.Fatalf("round = %+v", round)
	}
	if plan.InstanceID(1) != 1050202*10000+1 {
		t.Fatalf("instance id = %d", plan.InstanceID(1))
	}
	if len(plan.ActiveRounds(start-3601)) != 0 {
		t.Fatal("not shown yet")
	}
	if rounds := plan.ActiveRounds(start - 3600); len(rounds) != 1 {
		t.Fatal("should be active from forecast time")
	}
	if rounds := plan.ActiveRounds(round.ExpireTs - 1); len(rounds) != 1 {
		t.Fatal("should stay active until expire")
	}
	if len(plan.ActiveRounds(round.ExpireTs)) != 0 {
		t.Fatal("should expire")
	}
	if next, ok := plan.NextShowTs(start - 7200); !ok || next != round.ShowTs {
		t.Fatalf("next show = %d %v", next, ok)
	}
	if _, ok := plan.NextShowTs(start); ok {
		t.Fatal("non-repeating plan has no next round")
	}
}

// TestServerOpenTrigger 开服后 N 天：从开服当天 0 点起算。
func TestServerOpenTrigger(t *testing.T) {
	openTs := mustTs(t, "2026-10-01 15:30:00")
	cfg := &glconf.ActvOnlineCfg{ID: 2300101, Available: true, Type: 12, TriggerType: 2, TriggerVal: "7d", Duration: "7d"}
	plan, err := NewPlan(cfg, openTs, testLoc)
	if err != nil {
		t.Fatal(err)
	}
	if want := mustTs(t, "2026-10-08 00:00:00"); plan.FirstStartTs != want {
		t.Fatalf("start = %d, want %d", plan.FirstStartTs, want)
	}
	if _, err := NewPlan(cfg, 0, testLoc); err == nil {
		t.Fatal("server-open trigger requires open time")
	}
	zero := &glconf.ActvOnlineCfg{ID: 2, Available: true, TriggerType: 2, TriggerVal: "0d", Duration: "7d"}
	if plan, _ := NewPlan(zero, openTs, testLoc); plan.FirstStartTs != mustTs(t, "2026-10-01 00:00:00") {
		t.Fatal("0d starts on open day")
	}
}

// TestReopenTrigger 循环开启：按间隔生成轮次，开服天数限制跳过过早的轮次，保留期重叠时返回多轮。
func TestReopenTrigger(t *testing.T) {
	first := mustTs(t, "2025-01-20 00:00:00")
	cfg := &glconf.ActvOnlineCfg{ID: 9, Available: true, TriggerType: 1, TriggerVal: "2025-01-20 00:00:00",
		Reopen: true, TriggerRept: "28d", Duration: "7d"}
	plan, err := NewPlan(cfg, 0, testLoc)
	if err != nil {
		t.Fatal(err)
	}
	now := first + 28*86400*10 + 3*86400 // 第 11 轮进行中
	rounds := plan.ActiveRounds(now)
	if len(rounds) != 1 || rounds[0].Round != 11 || rounds[0].StartTs != first+28*86400*10 {
		t.Fatalf("rounds = %+v", rounds)
	}
	if len(plan.ActiveRounds(first+28*86400*10+8*86400)) != 0 {
		t.Fatal("gap between rounds has no active round")
	}
	if next, ok := plan.NextShowTs(now); !ok || next != first+28*86400*11 {
		t.Fatalf("next = %d %v", next, ok)
	}

	// 开服天数限制：开服 2025-03-01，限制 46 天 => 2025-04-16 之后的首轮为 2025-05-12（第 5 轮）。
	limited := *cfg
	limited.OpenDayLimit = "46d"
	plan, err = NewPlan(&limited, mustTs(t, "2025-03-01 00:00:00"), testLoc)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ActiveRounds(first+3*86400)) != 0 {
		t.Fatal("rounds before open-day limit must be skipped")
	}
	if next, ok := plan.NextShowTs(first); !ok || next != mustTs(t, "2025-05-12 00:00:00") {
		t.Fatalf("first allowed round = %d %v", next, ok)
	}

	// 保留期长于间隔：上一轮未过期时与当前轮同时活跃。
	overlap := &glconf.ActvOnlineCfg{ID: 10, Available: true, TriggerType: 1, TriggerVal: "2025-01-20 00:00:00",
		Reopen: true, TriggerRept: "7d", Duration: "6d", CloseAfterEnd: "3d"}
	plan, _ = NewPlan(overlap, 0, testLoc)
	if rounds := plan.ActiveRounds(first + 7*86400 + 3600); len(rounds) != 2 || rounds[0].Round != 1 || rounds[1].Round != 2 {
		t.Fatalf("overlap rounds = %+v", rounds)
	}

	// 不循环且首轮早于开服限制：永不开启。
	never := &glconf.ActvOnlineCfg{ID: 11, Available: true, TriggerType: 1, TriggerVal: "2025-01-20 00:00:00",
		Duration: "7d", OpenDayLimit: "10d"}
	plan, _ = NewPlan(never, mustTs(t, "2025-01-19 00:00:00"), testLoc)
	if len(plan.ActiveRounds(first+86400)) != 0 {
		t.Fatal("should never open")
	}
	if _, ok := plan.NextShowTs(0); ok {
		t.Fatal("should have no next round")
	}
}

func TestTriggerConfigErrors(t *testing.T) {
	base := glconf.ActvOnlineCfg{ID: 1, Available: true, TriggerType: 1, TriggerVal: "2026-01-01 00:00:00", Duration: "7d"}
	unsupported := base
	unsupported.TriggerType = 3
	if _, err := NewPlan(&unsupported, 1, testLoc); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("type 3 err = %v", err)
	}
	disabled := base
	disabled.Available = false
	badDate := base
	badDate.TriggerVal = "2026/01/01"
	noDuration := base
	noDuration.Duration = "0h"
	for name, cfg := range map[string]glconf.ActvOnlineCfg{"disabled": disabled, "badDate": badDate, "noDuration": noDuration} {
		cfg := cfg
		if _, err := NewPlan(&cfg, 1, testLoc); err == nil {
			t.Fatalf("%s should fail", name)
		}
	}
	plans := BuildPlans([]*glconf.ActvOnlineCfg{&base, &unsupported, &disabled, &badDate, nil}, 1, testLoc)
	if len(plans) != 1 || plans[0].CfgID != 1 {
		t.Fatalf("plans = %+v", plans)
	}
}

// TestRealConfigTable 用实际活动表校验：类型 1、2 的启用配置全部能解析。表文件不存在时跳过。
func TestRealConfigTable(t *testing.T) {
	file, err := os.Open("/data/g1/glconf/ActvOnlineCfg.tsv")
	if err != nil {
		t.Skip("config table not found")
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	var header []string
	line := 0
	parsed := 0
	openTs := mustTs(t, "2026-01-01 00:00:00")
	for scanner.Scan() {
		line++
		fields := strings.Split(strings.TrimRight(scanner.Text(), "\r"), "\t")
		if line == 4 {
			header = fields // 第 4 行为字段名
			continue
		}
		if line <= 8 || len(fields) < len(header) {
			continue
		}
		row := map[string]string{}
		for i, name := range header {
			row[name] = strings.TrimSpace(fields[i])
		}
		if row["Available"] != "1" || (row["TriggerType"] != "1" && row["TriggerType"] != "2") {
			continue
		}
		cfg := &glconf.ActvOnlineCfg{
			ID: atoi32(row["ID"]), Available: true, Type: atoi32(row["Type"]), TriggerType: atoi32(row["TriggerType"]),
			TriggerVal: row["TriggerVal"], Reopen: row["Reopen"] == "1", OpenDayLimit: row["OpenDayLimit"],
			TriggerRept: row["TriggerRept"], ForecastBeforeStart: row["ForecastBeforeStart"],
			Duration: row["Duration"], CloseAfterEnd: row["CloseAfterEnd"],
		}
		if _, err := NewPlan(cfg, openTs, testLoc); err != nil {
			t.Errorf("line %d: %v", line, err)
			continue
		}
		parsed++
	}
	if parsed == 0 {
		t.Fatal("no type 1/2 configs parsed")
	}
	t.Logf("parsed %d type 1/2 configs", parsed)
}

func atoi32(value string) int32 {
	var n int32
	for _, c := range value {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int32(c-'0')
	}
	return n
}
