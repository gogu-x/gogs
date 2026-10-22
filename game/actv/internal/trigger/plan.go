// Package trigger 解析活动配置 ActvOnlineCfg 的时间规则，计算每一轮的预告、开始、结束、过期时间。
// 本包只做纯计算，不依赖活动服务，可单独测试。
package trigger

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/tree/tlog"
)

// Type 标识活动配置 ActvOnlineCfg.trigger_type 的时间类型。
type Type int32

const (
	// TypeAbsolute 绝对日期：trigger_val 为 "2006-01-02 15:04:05"。
	TypeAbsolute Type = 1
	// TypeServerOpen 开服后 N 天：trigger_val 为时长（如 "7d"），从开服当天 0 点起算。
	TypeServerOpen Type = 2
)

const (
	// dateLayout 是绝对日期类型的时间格式。
	dateLayout = "2006-01-02 15:04:05"
	// maxActiveRoundScan 限制回溯仍在有效期内的历史轮次数量，防止配置异常时无限循环。
	maxActiveRoundScan = 64
	// maxRoundsPerConfig 是单个配置的最大轮次数，保证实例编号不冲突。
	maxRoundsPerConfig = 10000
)

// ErrUnsupported 表示配置的时间类型当前版本尚未支持。
var ErrUnsupported = errors.New("unsupported activity trigger type")

// Plan 是一条活动配置解析后的时间规则，所有时间为 Unix 秒。
// 第 k 轮（k 从 0 开始）开始时间 = FirstStartTs + k*RepeatInterval；不循环时只有第 0 轮。
type Plan struct {
	Cfg            *glconf.ActvOnlineCfg // 源配置行，创建实例时交给模板。
	CfgID          int32                 // 活动配置编号。
	TemplateType   int32                 // 活动模板类型。
	TriggerType    Type                  // 时间类型。
	FirstStartTs   int64                 // 首轮开始时间。
	Duration       int64                 // 每轮持续时长：结束时间 = 开始时间 + Duration。
	CloseAfterEnd  int64                 // 结束后保留时长：过期时间 = 结束时间 + CloseAfterEnd。
	Forecast       int64                 // 预告时长：展示时间 = 开始时间 - Forecast。
	Reopen         bool                  // 是否循环开启。
	RepeatInterval int64                 // 循环间隔，Reopen 且大于 0 时生效。
	EarliestStart  int64                 // 允许开启的最早时间（开服时间 + open_day_limit），早于它的轮次跳过。
}

// Round 是一条配置某一轮的具体时间。
type Round struct {
	Round    int32 // 轮次，从 1 开始。
	ShowTs   int64 // 预告（可见）时间。
	StartTs  int64 // 开始时间。
	EndTs    int64 // 结束时间。
	ExpireTs int64 // 过期时间，到达后删除实例和数据。
}

// InstanceID 返回该轮对应的确定性活动实例编号，重启后可据此识别已创建的实例。
func (plan *Plan) InstanceID(round int32) int64 {
	return int64(plan.CfgID)*maxRoundsPerConfig + int64(round)
}

// NewPlan 解析活动配置的时间规则。openTs 为开服时间（Unix 秒），loc 为配置日期所在时区。
// 配置未启用、时间类型不支持或字段格式错误时返回错误。
func NewPlan(cfg *glconf.ActvOnlineCfg, openTs int64, loc *time.Location) (*Plan, error) {
	if cfg == nil || cfg.GetID() <= 0 {
		return nil, fmt.Errorf("activity config is empty")
	}
	if !cfg.GetAvailable() {
		return nil, fmt.Errorf("activity config %d is not available", cfg.GetID())
	}
	if loc == nil {
		loc = time.Local
	}
	plan := &Plan{
		Cfg:          cfg,
		CfgID:        cfg.GetID(),
		TemplateType: cfg.GetType(),
		TriggerType:  Type(cfg.GetTriggerType()),
		Reopen:       cfg.IsReopen(),
	}
	var err error
	if plan.Duration, err = ParseDuration(cfg.GetDuration()); err != nil {
		return nil, fmt.Errorf("config %d duration: %w", cfg.GetID(), err)
	}
	if plan.Duration <= 0 {
		return nil, fmt.Errorf("config %d duration must be positive, got %q", cfg.GetID(), cfg.GetDuration())
	}
	if plan.CloseAfterEnd, err = ParseDuration(cfg.CloseAfterEnd); err != nil {
		return nil, fmt.Errorf("config %d close_after_end: %w", cfg.GetID(), err)
	}
	if plan.Forecast, err = ParseDuration(cfg.ForecastBeforeStart); err != nil {
		return nil, fmt.Errorf("config %d forecast_before_start: %w", cfg.GetID(), err)
	}
	if plan.RepeatInterval, err = ParseDuration(cfg.GetTriggerRept()); err != nil {
		return nil, fmt.Errorf("config %d trigger_rept: %w", cfg.GetID(), err)
	}
	openDayLimit, err := ParseDuration(cfg.OpenDayLimit)
	if err != nil {
		return nil, fmt.Errorf("config %d open_day_limit: %w", cfg.GetID(), err)
	}
	if openDayLimit > 0 {
		if openTs <= 0 {
			return nil, fmt.Errorf("config %d open_day_limit requires server open time", cfg.GetID())
		}
		plan.EarliestStart = openTs + openDayLimit
	}

	switch plan.TriggerType {
	case TypeAbsolute:
		start, err := time.ParseInLocation(dateLayout, strings.TrimSpace(cfg.GetTriggerVal()), loc)
		if err != nil {
			return nil, fmt.Errorf("config %d trigger_val date: %w", cfg.GetID(), err)
		}
		plan.FirstStartTs = start.Unix()
	case TypeServerOpen:
		if openTs <= 0 {
			return nil, fmt.Errorf("config %d trigger type %d requires server open time", cfg.GetID(), plan.TriggerType)
		}
		offset, err := ParseDuration(cfg.GetTriggerVal())
		if err != nil {
			return nil, fmt.Errorf("config %d trigger_val offset: %w", cfg.GetID(), err)
		}
		plan.FirstStartTs = dayStart(openTs, loc) + offset
	default:
		return nil, fmt.Errorf("config %d trigger type %d: %w", cfg.GetID(), plan.TriggerType, ErrUnsupported)
	}
	return plan, nil
}

// BuildPlans 解析全部活动配置；未启用的配置静默跳过，不支持的时间类型和格式错误记录日志后跳过。
func BuildPlans(cfgs []*glconf.ActvOnlineCfg, openTs int64, loc *time.Location) []*Plan {
	plans := make([]*Plan, 0, len(cfgs))
	unsupported := make(map[int32]int)
	for _, cfg := range cfgs {
		if cfg == nil || cfg.GetID() <= 0 || !cfg.GetAvailable() {
			continue
		}
		plan, err := NewPlan(cfg, openTs, loc)
		if errors.Is(err, ErrUnsupported) {
			unsupported[cfg.GetTriggerType()]++
			continue
		}
		if err != nil {
			tlog.Log.Error("[activity/trigger.BuildPlans] 活动配置时间规则无效，跳过, config=%v err=%v", cfg.GetID(), err)
			continue
		}
		plans = append(plans, plan)
	}
	for triggerType, count := range unsupported {
		tlog.Log.Info("[activity/trigger.BuildPlans] 活动时间类型暂不支持，跳过, triggerType=%v count=%v", triggerType, count)
	}
	return plans
}

// Round 返回第 index 轮（从 0 开始）的时间。
func (plan *Plan) RoundAt(index int64) Round {
	start := plan.FirstStartTs + index*plan.interval()
	end := start + plan.Duration
	return Round{
		Round:    int32(index + 1),
		ShowTs:   start - plan.Forecast,
		StartTs:  start,
		EndTs:    end,
		ExpireTs: end + plan.CloseAfterEnd,
	}
}

// ActiveRounds 返回 now 时刻已到预告时间且尚未过期的轮次（按轮次升序），用于创建或恢复实例。
// 循环活动的结束保留期可能与下一轮重叠，因此可能返回多轮。
func (plan *Plan) ActiveRounds(now int64) []Round {
	last, ok := plan.lastShownIndex(now)
	if !ok {
		return nil
	}
	first := plan.firstIndex()
	var rounds []Round
	for index, scanned := last, 0; index >= first && scanned < maxActiveRoundScan; index, scanned = index-1, scanned+1 {
		round := plan.RoundAt(index)
		if round.ExpireTs <= now {
			break // 更早的轮次结束得更早，同样已过期。
		}
		rounds = append([]Round{round}, rounds...)
	}
	return rounds
}

// NextShowTs 返回 now 之后下一轮的预告时间；没有后续轮次时返回 false。用于计算下一次需要检查的时刻。
func (plan *Plan) NextShowTs(now int64) (int64, bool) {
	index := plan.firstIndex()
	if index < 0 {
		return 0, false // 不循环且首轮早于开服天数限制，永不开启。
	}
	if last, ok := plan.lastShownIndex(now); ok {
		index = last + 1
	}
	if !plan.repeating() && index > 0 {
		return 0, false
	}
	if index >= maxRoundsPerConfig {
		return 0, false
	}
	return plan.RoundAt(index).ShowTs, true
}

// lastShownIndex 返回预告时间不晚于 now 的最大轮次下标。
func (plan *Plan) lastShownIndex(now int64) (int64, bool) {
	first := plan.firstIndex()
	if first < 0 || plan.RoundAt(first).ShowTs > now {
		return 0, false
	}
	if !plan.repeating() {
		return 0, true
	}
	index := (now + plan.Forecast - plan.FirstStartTs) / plan.RepeatInterval
	if index >= maxRoundsPerConfig {
		index = maxRoundsPerConfig - 1
	}
	return index, true
}

// firstIndex 返回满足开服天数限制的首个轮次下标；不循环且首轮早于限制时返回 -1（永不开启）。
func (plan *Plan) firstIndex() int64 {
	if plan.EarliestStart <= plan.FirstStartTs {
		return 0
	}
	if !plan.repeating() {
		return -1
	}
	gap := plan.EarliestStart - plan.FirstStartTs
	return (gap + plan.RepeatInterval - 1) / plan.RepeatInterval
}

func (plan *Plan) repeating() bool { return plan.Reopen && plan.RepeatInterval > 0 }

func (plan *Plan) interval() int64 {
	if plan.repeating() {
		return plan.RepeatInterval
	}
	return 0
}

// ParseDuration 解析配置中的时长，单位支持 d/h/m/s，返回秒数。空串和 "0" 视为 0。
func ParseDuration(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return 0, nil
	}
	unit := value[len(value)-1]
	var seconds int64
	switch unit {
	case 'd':
		seconds = 86400
	case 'h':
		seconds = 3600
	case 'm':
		seconds = 60
	case 's':
		seconds = 1
	default:
		return 0, fmt.Errorf("invalid duration %q: unit must be d/h/m/s", value)
	}
	number, err := strconv.ParseInt(value[:len(value)-1], 10, 64)
	if err != nil || number < 0 {
		return 0, fmt.Errorf("invalid duration %q", value)
	}
	return number * seconds, nil
}

// dayStart 返回 ts 所在自然日 0 点。
func dayStart(ts int64, loc *time.Location) int64 {
	t := time.Unix(ts, 0).In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).Unix()
}
