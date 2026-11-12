package trigger

import (
	"github.com/gogu-x/gogs/glconf"
	"time"
)

// Point 标识触发检查的生命周期入口，可继续扩展联盟初始化等入口。
type Point int32

const (
	OnServerTick Point = 1        // 服务器定时创建检查。
	OnPlayerInit Point = 2        // 玩家活动数据初始化完成。
	scopePlayer        = int32(1) // 玩家归属，数值与服务归属协议一致。
	scopeServer        = int32(2) // 服务器归属，数值与服务归属协议一致。
)

// Selection 保存触发规则选出的创建计划；实例构造与存盘仍由活动服务执行。
type Selection struct {
	serverPlans   []*Plan
	playerConfigs []*glconf.ActvOnlineCfg
}

// ServerPlans 返回服务器时间计划。
func (s Selection) ServerPlans() []*Plan { return s.serverPlans }

// PlayerConfigs 返回本次允许创建的个人配置。
func (s Selection) PlayerConfigs() []*glconf.ActvOnlineCfg { return s.playerConfigs }

// Rule 根据当前时刻和归属者创建历史计算需要创建的活动。
type Rule func(now int64, history map[int32]bool) Selection

// Engine 统一保存配置解析结果和各触发入口的规则，仅由活动 Actor 串行使用。
type Engine struct{ rules map[Point]Rule }

// NewEngine 按模板归属分流配置，并注册服务器定时、玩家初始化两种触发规则。
func NewEngine(
	rows []*glconf.ActvOnlineCfg, scopeOf func(int32) int32, openTs int64, loc *time.Location,
) *Engine {
	var serverRows, playerRows []*glconf.ActvOnlineCfg
	for _, row := range rows {
		if row == nil || !row.GetAvailable() {
			continue
		}
		switch scopeOf(row.GetType()) {
		case scopeServer:
			serverRows = append(serverRows, row)
		case scopePlayer:
			playerRows = append(playerRows, row)
		}
	}
	plans := BuildPlans(serverRows, openTs, loc)
	engine := &Engine{rules: make(map[Point]Rule)}
	engine.Register(OnServerTick, func(now int64, _ map[int32]bool) Selection {
		var due []*Plan
		for _, plan := range plans {
			if len(plan.SettlementRounds(now)) > 0 {
				due = append(due, plan)
			}
		}
		return Selection{serverPlans: due}
	})
	engine.Register(OnPlayerInit, func(_ int64, history map[int32]bool) Selection {
		return Selection{playerConfigs: PlayerOnceConfigs(playerRows, history)}
	})
	return engine
}

// Register 注册或替换某个触发入口的规则，新增规则无需修改玩家加载流程。
func (e *Engine) Register(point Point, rule Rule) { e.rules[point] = rule }

// Check 是所有活动触发创建的统一检查入口，未知入口不创建活动。
func (e *Engine) Check(point Point, now int64, history map[int32]bool) Selection {
	if e == nil {
		return Selection{}
	}
	if rule := e.rules[point]; rule != nil {
		return rule(now, history)
	}
	return Selection{}
}

// PlayerOnceConfigs 统一执行个人一次性配置规则，已完成或无法恢复的历史记录也阻止重复创建。
func PlayerOnceConfigs(rows []*glconf.ActvOnlineCfg, history map[int32]bool) []*glconf.ActvOnlineCfg {
	selected := make([]*glconf.ActvOnlineCfg, 0)
	seen := make(map[int32]bool)
	for _, row := range rows {
		if row == nil || !row.GetAvailable() || row.IsReopen() || len(row.GetServerGroups()) > 0 {
			continue
		}
		id := row.GetID()
		if id <= 0 || history[id] || seen[id] {
			continue
		}
		seen[id] = true
		selected = append(selected, row)
	}
	return selected
}

// NewSelection 供扩展规则返回服务器计划或个人配置，不暴露引擎内部容器。
func NewSelection(serverPlans []*Plan, playerConfigs []*glconf.ActvOnlineCfg) Selection {
	return Selection{serverPlans: serverPlans, playerConfigs: playerConfigs}
}
