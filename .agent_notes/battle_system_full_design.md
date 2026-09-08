# 速度条驱动战斗系统设计方案（1v1 / 1vN）

## 1. 需求确认

- 行动顺序由**速度条（Action Bar / ATB）**驱动，非固定回合制
- 支持 **1v1** 和 **1vN**：一个行动单位可以选择单个目标，或释放 AOE 技能命中多个目标
- 完整技能系统：**主动技能**（冷却时间、目标选择规则）+ **被动效果**（常驻加成）+ **状态效果**（buff/debuff、控制类如眩晕/沉默/禁疗）
- 战斗持续到**一方全灭**为止
- **人工/自动双模式**：单位行动时的技能与目标决策，可以由玩家手动选择，也可以由系统自动决策（挂机/自动战斗），且允许战斗过程中动态切换
- **多种攻击来源**：普通攻击、技能触发攻击、宝物（神器/法宝类道具）攻击等需要统一处理，不各自独立一套流程
- 服务端权威计算，客户端仅提交指令、展示结果
- 语言：Go

## 2. 总体架构

```
┌──────────────────────────────────────────────────────────────┐
│                         BattleInstance                        │
│  (一场战斗的顶层容器，驱动主循环)                                │
└───────────────────────────┬────────────────────────────────────┘
                            │
         ┌──────────────────┼──────────────────┬───────────────┐
         ▼                  ▼                  ▼               ▼
   ActionBarScheduler   UnitRegistry       BattleLog     ActionDecider
   (速度条/行动顺序)     (参战单位管理)      (战报记录)     (人工/自动决策)
         │                  │                                  │
         ▼                  ▼                                  ▼
   ActionExecutor      BattleUnit                       PlayerInputSource
   (执行一次行动)        ├─ Attributes (属性系统)          / AutoAIStrategy
         │              ├─ SkillSet   (技能集)
         ▼              ├─ StatusList (状态效果列表)
   TargetSelector       └─ Combatant  (供伤害计算复用)
   (目标选择/AOE范围)
         │
         ▼
   AttackSource (普攻/技能/宝物 统一攻击来源模型)
         │
         ▼
   DamagePipeline (复用前述伤害计算框架)
```

分七大子系统：**时序调度**、**单位与属性**、**技能与效果**、**目标选择**、**人工/自动决策**、**攻击来源统一模型**、**战报**。逐一展开。

## 3. 时序调度：速度条系统

### 3.1 核心模型

每个单位有 `Speed`（速度属性）和 `ActionValue`（行动值，累积到阈值后行动，即 ATB 常见实现）：

```go
// ActionBarScheduler 管理所有参战单位的行动顺序
type ActionBarScheduler struct {
    units     []*BattleUnit
    threshold float64 // 行动阈值，通常固定为10000之类的常量
}

// Tick 推进时间，返回本次达到阈值、需要行动的单位（可能多个同时达到，需排序）
func (s *ActionBarScheduler) Tick() []*BattleUnit {
    var ready []*BattleUnit
    for _, u := range s.units {
        if u.IsDead() {
            continue
        }
        u.ActionValue += u.EffectiveSpeed() // 受buff影响的实际速度
        if u.ActionValue >= s.threshold {
            ready = append(ready, u)
        }
    }
    // 多个同时达到阈值时，按 ActionValue 超出阈值的多少 + Speed 排序，保证确定性
    sort.Slice(ready, func(i, j int) bool {
        if ready[i].ActionValue != ready[j].ActionValue {
            return ready[i].ActionValue > ready[j].ActionValue
        }
        return ready[i].EffectiveSpeed() > ready[j].EffectiveSpeed()
    })
    return ready
}

// Consume 单位行动后消耗行动值（通常清零或扣除阈值，取决于是否允许连续行动的余量结转）
func (u *BattleUnit) Consume() {
    u.ActionValue -= u.scheduler.threshold
}
```

**设计要点**：
- 用"行动值累积"而非"固定回合数"，天然支持速度差异带来的多次行动（速度快的单位在同一时间窗口内可能行动两次）
- 排序规则必须确定性（不能用 map 遍历顺序），否则同一份输入在不同机器上跑出不同战报，无法回放/复现
- `EffectiveSpeed()` 而非裸 `Speed` 字段，因为加速/减速类 buff（如"眩晕"期间速度归零、"加速"buff提升速度）需要实时生效，不能直接改写基础属性

### 3.2 主循环

```go
type BattleInstance struct {
    scheduler *ActionBarScheduler
    unitsA    []*BattleUnit // 阵营A
    unitsB    []*BattleUnit // 阵营B
    log       *BattleLog
    maxTicks  int // 保底上限，防止极端配置导致死循环（比如双方都无法击杀对方）
}

func (b *BattleInstance) Run() *BattleResult {
    for tick := 0; tick < b.maxTicks; tick++ {
        if b.checkVictory(); result := b.checkVictory(); result != nil {
            return result
        }
        readyUnits := b.scheduler.Tick()
        for _, unit := range readyUnits {
            if unit.IsDead() {
                continue // 行动值达到阈值后但在排队执行前死亡（被其他单位杀死）
            }
            b.executeAction(unit)
            unit.Consume()
            // 每个单位行动后立即检查胜负，避免多余的后续行动单位继续执行
            if result := b.checkVictory(); result != nil {
                return result
            }
        }
    }
    return b.timeoutResult() // 超过maxTicks的兜底结算（比如判定为平局或按剩余血量判定）
}

func (b *BattleInstance) checkVictory() *BattleResult {
    aDead := allDead(b.unitsA)
    bDead := allDead(b.unitsB)
    switch {
    case aDead && bDead:
        return &BattleResult{Draw: true}
    case aDead:
        return &BattleResult{WinnerSide: SideB}
    case bDead:
        return &BattleResult{WinnerSide: SideA}
    }
    return nil
}
```

**设计要点**：
- `maxTicks` 兜底是必须的：状态效果配置错误（比如无限护盾）可能导致战斗永不结束，纯理论上的"一方全灭为止"在生产环境必须有超时保护
- 每次单位行动后立刻检查胜负，而不是等一整轮所有单位都行动完，否则会出现"死人还在行动"的逻辑错误

## 4. 单位与属性系统

### 4.1 BattleUnit

```go
// BattleUnit 一个可参战的最小单位（可以是英雄、召唤物、boss等）
type BattleUnit struct {
    ID          int64
    Side        Side // 阵营A/B
    Attrs       *AttributeSet
    SkillSet    *SkillSet
    StatusList  *StatusEffectList
    ActionValue float64
    scheduler   *ActionBarScheduler

    currentHP float64
}

func (u *BattleUnit) IsDead() bool { return u.currentHP <= 0 }

func (u *BattleUnit) EffectiveSpeed() float64 {
    base := u.Attrs.Get(AttrSpeed)
    return u.StatusList.ModifyValue(AttrSpeed, base) // 状态效果对速度的加成/削减在这里统一生效
}
```

### 4.2 属性系统：基础值 + 修正层，而非直接改写字段

不直接用 `unit.Attack += 100` 这种命令式修改（这样一旦buff过期，很难精确撤销"当时具体加了多少"，尤其多个buff叠加时顺序敏感）。改为**分层计算模型**：

```go
type AttributeSet struct {
    base map[AttrType]float64
}

// AttributeModifier 由 Buff/装备/被动技能提供，不直接修改base，而是登记一条"修正规则"
type AttributeModifier struct {
    Attr    AttrType
    Mode    ModifyMode // Additive(加法) / Multiplicative(乘法) / Override(覆盖)
    Value   float64
    Source  string // 来源标识，方便调试和技能失效时精确移除
}

// AttributeSet.Resolve 每次查询属性时，实时按 base + 所有当前生效的 Modifier 计算，
// 而不是缓存一个"当前值"字段——避免修正来源增删时忘记同步缓存导致的数值bug
func (a *AttributeSet) Resolve(attr AttrType, modifiers []AttributeModifier) float64 {
    base := a.base[attr]
    var additive, multiplicative float64 = 0, 1
    for _, m := range modifiers {
        if m.Attr != attr {
            continue
        }
        switch m.Mode {
        case Additive:
            additive += m.Value
        case Multiplicative:
            multiplicative *= 1 + m.Value
        case Override:
            return m.Value // 覆盖类修正直接短路返回（如"定身期间速度强制为0"）
        }
    }
    return (base + additive) * multiplicative
}
```

**设计要点**：
- 属性永远"按需实时计算"而非缓存最终值，配合下面状态效果的增删，可以保证任何时刻查询到的都是当前真实生效值，不存在"忘记回滚"的风险
- `Source` 字段是关键：状态效果过期时，按 `Source` 精确移除对应的 `Modifier`，而不是重新全量计算（虽然本设计允许全量重算，但保留 Source 便于调试和日志追溯，比如"这次减速是被谁的技能造成的"）

## 5. 技能与效果系统

### 5.1 三层模型：主动技能 / 被动效果 / 状态效果

这三者语义不同，不能用同一个接口强行统一（现有代码把技能和effect混在一套Phase事件里，导致理解成本高），分开建模：

```go
// ActiveSkill 主动技能：有CD、有目标选择规则、由玩家/AI主动触发
type ActiveSkill struct {
    ID           int32
    Cooldown     int32 // 以"行动次数"或"tick"计
    CurrentCD    int32
    TargetRule   TargetRule // 单体/AOE/全体/自身 等
    Effects      []SkillEffect // 命中后产生的效果（伤害/治疗/施加状态等）
}

func (s *ActiveSkill) IsReady() bool { return s.CurrentCD <= 0 }

func (s *ActiveSkill) OnUsed() { s.CurrentCD = s.Cooldown }

func (s *ActiveSkill) OnTick() {
    if s.CurrentCD > 0 {
        s.CurrentCD--
    }
}

// PassiveEffect 被动效果：常驻生效，不需要主动触发，通常在特定事件时机响应
// （如"受到攻击时有5%概率反击"），生命周期与拥有者相同，不会过期
type PassiveEffect struct {
    ID       int32
    Trigger  TriggerCondition // 触发条件：OnHit / OnCrit / OnKill / OnTurnStart 等
    Effect   SkillEffect
    Chance   float64
}

// StatusEffect 状态效果：buff/debuff，有持续时间，可叠加/驱散/免疫
type StatusEffect struct {
    ID           int32
    Category     StatusCategory // Buff / Debuff / Control(控制类：眩晕/沉默/禁疗)
    Duration     int32          // 剩余持续时间（以"该单位行动次数"计，而非全局tick，避免速度不同导致buff实际时长不一致）
    StackCount   int32
    MaxStack     int32
    Modifiers    []AttributeModifier // 生效期间提供的属性修正
    OnApply      func(target *BattleUnit)
    OnExpire     func(target *BattleUnit)
    OnTurnEnd    func(target *BattleUnit) // 用于DOT/HOT类效果，每次该单位行动结束时触发一次
}
```

### 5.2 状态效果管理：叠加规则与冲突处理

```go
type StatusEffectList struct {
    effects []*StatusEffect
}

// Add 施加新状态，处理叠加/覆盖/免疫逻辑
func (l *StatusEffectList) Add(effect *StatusEffect, owner *BattleUnit) {
    if owner.IsImmuneTo(effect.Category) {
        return // 免疫判断前置，比如"控制免疫"buff存在时直接拒绝施加眩晕
    }
    for _, existing := range l.effects {
        if existing.ID == effect.ID {
            switch effect.StackPolicy() {
            case RefreshDuration:
                existing.Duration = effect.Duration // 同ID效果只刷新时长，不叠加
                return
            case StackUpToMax:
                existing.StackCount = min(existing.StackCount+1, existing.MaxStack)
                existing.Duration = effect.Duration
                return
            case IndependentInstance:
                // 允许多个独立实例并存（如"燃烧"每次施加都是独立DOT，各自计时）
            }
        }
    }
    l.effects = append(l.effects, effect)
    effect.OnApply(owner)
}

// ModifyValue 汇总当前所有生效状态对某属性的修正
func (l *StatusEffectList) ModifyValue(attr AttrType, base float64) float64 {
    var modifiers []AttributeModifier
    for _, e := range l.effects {
        modifiers = append(modifiers, e.Modifiers...)
    }
    return resolveWithModifiers(base, attr, modifiers)
}

// HasControl 判断是否处于某类控制状态（眩晕会导致跳过本次行动）
func (l *StatusEffectList) HasControl(category StatusCategory) bool {
    for _, e := range l.effects {
        if e.Category == category {
            return true
        }
    }
    return false
}
```

**设计要点**：
- **叠加策略需要在配置层显式声明**（`RefreshDuration`/`StackUpToMax`/`IndependentInstance`），不能靠代码里硬编码判断，否则策划新增一个buff就要改代码
- **持续时间以"该单位自身的行动次数"计**，而非全局tick数——这是速度条系统的关键细节：如果用全局tick计时，速度快的单位在同样"3回合"buff下实际能多行动几次，导致buff实际收益因单位速度不同而失衡；用"该单位行动次数"计时则时长语义稳定
- **控制类效果**（眩晕/沉默）不是普通buff，需要在"行动执行"环节前置检查（见下文 ActionExecutor）

### 5.3 行动执行流程（概要）

行动执行的完整流程（决策、校验、目标解析、效果应用）在第7节统一说明，这里先给出与状态效果相关的骨架：

```go
func (b *BattleInstance) executeAction(unit *BattleUnit) {
    unit.StatusList.TickTurnStart(unit) // 回合开始类效果（如中毒扣血）

    if unit.StatusList.HasControl(ControlStun) {
        b.log.RecordSkip(unit, "眩晕跳过行动")
        unit.SkillSet.TickCooldowns() // CD仍然正常流转，不因跳过行动而卡住
        return
    }

    // 决策、校验、执行的完整流程见 7.5 节，此处不假设决策来源是AI还是玩家
    b.decideAndExecute(unit)

    unit.SkillSet.TickCooldowns()
    unit.StatusList.TickTurnEnd(unit)
    unit.StatusList.RemoveExpired()
}
```

## 6. 目标选择（支持1v1和1vN，可扩展设计）

### 6.1 设计问题

目标选择规则（单体/AOE/随机N个/最低血量等）如果用单一 `switch` 语句处理，3~5种规则时够用，但增长到"排除嘲讽后随机2个""HP低于30%的友方中攻击力最高的"这类组合条件时，`switch` 分支会指数级膨胀，且新增一种规则=修改已有函数，存在回归风险。因此采用**策略注册表**解决类型膨胀问题，用**Filter+Picker两层拆分**解决组合爆炸问题。

### 6.2 策略注册表：接口化 + 新增不改旧代码

```go
type TargetRule int

const (
    TargetSingleEnemy TargetRule = iota // 单体：需选择一个目标
    TargetAllEnemies                    // 全体敌方（AOE）
    TargetRandomN                       // 随机N个敌方
    TargetSelf                          // 自身
    TargetLowestHPAlly                  // 血量最低的友方（治疗技能常用）
)

// TargetRuleHandler 每种目标选择规则的独立实现
type TargetRuleHandler interface {
    // RequiresClientInput 是否需要外部（玩家/AI）指定目标，决定客户端是否要展示选择框
    RequiresClientInput() bool
    // Resolve 计算最终命中目标
    Resolve(caster *BattleUnit, preSelected []*BattleUnit, ctx *TargetContext) []*BattleUnit
}

// TargetContext 收敛所有规则可能需要的上下文，避免每个Handler签名都不一样
type TargetContext struct {
    Enemies []*BattleUnit
    Allies  []*BattleUnit
    RNG     *SeededRandom
    Params  map[string]float64 // 规则的可配置参数，如 TargetRandomN 的 N 值、"血量阈值"等
}

var targetRuleRegistry = map[TargetRule]TargetRuleHandler{}

func RegisterTargetRule(rule TargetRule, handler TargetRuleHandler) {
    if _, exists := targetRuleRegistry[rule]; exists {
        panic(fmt.Sprintf("target rule %v already registered", rule))
    }
    targetRuleRegistry[rule] = handler
}
```

`TargetSelector.Resolve` 退化成纯粹的分发器，不再关心具体规则怎么算：

```go
type TargetSelector struct{}

// Resolve 解析技能的命中目标。preSelected 是玩家/AI已经指定的目标（若TargetRule要求外部指定，
// 如TargetSingleEnemy），AOE/全体类规则会忽略preSelected，自动展开范围内所有目标
func (s *TargetSelector) Resolve(rule TargetRule, caster *BattleUnit, preSelected []*BattleUnit, sideA, sideB []*BattleUnit) []*BattleUnit {
    handler, ok := targetRuleRegistry[rule]
    if !ok {
        tlog.Errorf("unknown target rule: %v", rule)
        return nil
    }
    ctx := &TargetContext{
        Enemies: opponentsOf(caster, sideA, sideB),
        Allies:  alliesOf(caster, sideA, sideB),
        RNG:     caster.battle.rng, // 使用战斗实例的带种子随机源，保证可复现
        Params:  ruleParamsOf(rule), // 从技能配置或规则元数据里取参数
    }
    return handler.Resolve(caster, preSelected, ctx)
}
```

### 6.3 具体规则实现（各自独立，新增不改旧代码）

```go
// SingleEnemyHandler 单体：完全依赖外部指定
type SingleEnemyHandler struct{}

func (h SingleEnemyHandler) RequiresClientInput() bool { return true }
func (h SingleEnemyHandler) Resolve(caster *BattleUnit, preSelected []*BattleUnit, ctx *TargetContext) []*BattleUnit {
    return preSelected // 由 ActionCommand.TargetIDs 解析而来，见7.2节
}

// AllEnemiesHandler 全体敌方AOE
type AllEnemiesHandler struct{}

func (h AllEnemiesHandler) RequiresClientInput() bool { return false }
func (h AllEnemiesHandler) Resolve(caster *BattleUnit, _ []*BattleUnit, ctx *TargetContext) []*BattleUnit {
    return filterAlive(ctx.Enemies)
}

// RandomNHandler 随机N个敌方，N从Params读取，不用为每个"随机2个""随机3个"单独定义新规则
type RandomNHandler struct{}

func (h RandomNHandler) RequiresClientInput() bool { return false }
func (h RandomNHandler) Resolve(caster *BattleUnit, _ []*BattleUnit, ctx *TargetContext) []*BattleUnit {
    n := int(ctx.Params["count"])
    return pickRandomAlive(ctx.Enemies, n, ctx.RNG)
}

// LowestHPAllyHandler 血量最低的友方
type LowestHPAllyHandler struct{}

func (h LowestHPAllyHandler) RequiresClientInput() bool { return false }
func (h LowestHPAllyHandler) Resolve(caster *BattleUnit, _ []*BattleUnit, ctx *TargetContext) []*BattleUnit {
    return []*BattleUnit{lowestHP(filterAlive(ctx.Allies))}
}

func init() {
    RegisterTargetRule(TargetSingleEnemy, SingleEnemyHandler{})
    RegisterTargetRule(TargetAllEnemies, AllEnemiesHandler{})
    RegisterTargetRule(TargetRandomN, RandomNHandler{})
    RegisterTargetRule(TargetLowestHPAlly, LowestHPAllyHandler{})
    // 未来新增：只加一行，不改前面的代码
    // RegisterTargetRule(TargetHighestAttackEnemy, HighestAttackEnemyHandler{})
}
```

### 6.4 组合规则：Filter + Picker 两层拆分

规则数量上升后，容易出现"低血量的敌方"、"排除嘲讽单位后随机2个"这种**多重条件组合**。如果每种组合都单独定义一个Handler，注册表会爆炸式增长。此时把规则拆成"过滤器（Filter）+ 选择器（Picker）"两层：

```go
// TargetFilter 缩小候选范围（可组合叠加）
type TargetFilter func(units []*BattleUnit, ctx *TargetContext) []*BattleUnit

// TargetPicker 从候选范围里最终选出目标
type TargetPicker func(units []*BattleUnit, ctx *TargetContext) []*BattleUnit

// CompositeRule 由若干Filter + 一个Picker组成，覆盖组合场景，不需要为每个组合单独写Handler
type CompositeRule struct {
    Filters []TargetFilter
    Picker  TargetPicker
}

func (r CompositeRule) RequiresClientInput() bool { return false }

func (r CompositeRule) Resolve(caster *BattleUnit, preSelected []*BattleUnit, ctx *TargetContext) []*BattleUnit {
    candidates := ctx.Enemies
    for _, f := range r.Filters {
        candidates = f(candidates, ctx)
    }
    return r.Picker(candidates, ctx)
}

// 常用Filter/Picker作为可复用的构建块
var ExcludeTaunted TargetFilter = func(units []*BattleUnit, _ *TargetContext) []*BattleUnit {
    var result []*BattleUnit
    for _, u := range units {
        if !u.StatusList.HasCategory(StatusTaunt) {
            result = append(result, u)
        }
    }
    return result
}

var PickRandomN TargetPicker = func(units []*BattleUnit, ctx *TargetContext) []*BattleUnit {
    return pickRandomAlive(units, int(ctx.Params["count"]), ctx.RNG)
}

// "排除嘲讽后随机2个" 变成配置层的组合，不是新写代码
func init() {
    RegisterTargetRule(TargetRandomExcludeTaunt, CompositeRule{
        Filters: []TargetFilter{ExcludeTaunted},
        Picker:  PickRandomN,
    })
}
```

策划或后端在**配置层**就能拼出新组合，代码层的可复用构建块（Filter/Picker）保持在个位数增长，而不是随着规则数量线性膨胀。

### 6.5 1v1 与 1vN 的统一

不区分"1v1模式"和"1vN模式"，而是让**技能的 `TargetRule` 决定命中范围**——1v1场景下敌方只有一个单位，`TargetAllEnemies` 和 `TargetSingleEnemy` 结果自然一致；1vN场景下同一套规则自动支持AOE。战斗实例本身不需要关心"是1v1还是1vN"，只需要维护 `unitsA`/`unitsB` 两个切片，长度是1还是N对核心逻辑透明。

需要外部指定目标的规则（`TargetSingleEnemy`）依赖第7节的 `ActionCommand.TargetIDs`；不需要外部指定的规则（AOE、全体、最低血量友方）由 `TargetSelector` 自动计算，玩家/AI在这类技能上不需要（也不能）指定目标，客户端UI应据此隐藏目标选择框。`TargetRuleHandler.RequiresClientInput()` 就是这个判断的显式依据，不需要客户端另外维护一份"哪些规则需要选目标"的名单。

### 6.6 演进节奏建议

不建议一开始就上注册表+Filter/Picker这套完整方案。3~5种规则时 `switch` 完全够用、足够直观；当规则数量增长到10+种，或者出现明显的组合需求时，才是引入注册表和组合模式的合适时机。过早引入会增加不必要的抽象层，增加新人理解成本。

## 7. 人工 / 自动双模式决策

### 7.1 核心问题

轮到某个单位行动时，"选哪个技能、打哪个目标"这个决策可能来自三种渠道：
1. 玩家手动操作（客户端提交指令）
2. AI 自动决策（挂机/自动战斗）
3. 战斗过程中随时从①切换到②，或反之（比如玩家挂机后突然手动干预）

服务端权威计算意味着：**无论决策来自谁，最终都必须转换成同一种"行动指令"结构，再交给统一的执行流程**，执行流程本身不应该关心这个指令是人还是AI给出的。

### 7.2 统一指令模型

```go
// ActionCommand 一次行动的完整指令，人工和AI最终都产出这个结构
type ActionCommand struct {
    ActorID    int64
    SkillSlot  SkillSlotType // 普攻 / 技能1~4 / 宝物 等，见8.攻击来源统一模型
    TargetIDs  []int64       // 玩家/AI指定的目标；AOE类技能无需外部指定，由TargetRule自动展开
}

// ActionDecider 决策接口：人工/AI都实现该接口，执行层只依赖这个抽象
type ActionDecider interface {
    Decide(ctx context.Context, unit *BattleUnit, battle *BattleInstance) (*ActionCommand, error)
}
```

### 7.3 人工决策：等待客户端指令 + 超时自动托管

```go
// PlayerInputDecider 人工决策：阻塞等待客户端提交指令，带超时兜底
type PlayerInputDecider struct {
    inputCh  chan *ActionCommand // 由网络层收到客户端协议后写入
    timeout  time.Duration
    fallback ActionDecider // 超时后转交给自动决策，保证战斗不会卡死
}

func (d *PlayerInputDecider) Decide(ctx context.Context, unit *BattleUnit, battle *BattleInstance) (*ActionCommand, error) {
    select {
    case cmd := <-d.inputCh:
        return cmd, nil
    case <-time.After(d.timeout):
        // 超时自动托管：常见于战斗中玩家掉线/未操作，用AI兜底而不是让战斗停滞
        battle.log.RecordAutoTakeover(unit)
        return d.fallback.Decide(ctx, unit, battle)
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}
```

**设计要点**：
- 战斗主循环推进到某个单位行动时，如果该单位当前处于"人工模式"，必须**暂停主循环**，等待指令到达或超时，不能用忙等（polling）浪费资源，用 `channel + select` 阻塞等待是合适的实现
- 超时兜底是刚性要求：真实网络环境下客户端断线、卡顿是常态，服务端权威战斗绝不能因为等不到一个客户端指令就让整场战斗卡死，必须有超时自动接管逻辑
- 由网络层（收到客户端发来的"使用技能"协议）负责把解析后的 `ActionCommand` 写入对应单位的 `inputCh`，战斗逻辑与网络协议解耦

### 7.4 自动决策：AI策略

```go
// AutoAIDecider 自动战斗：不依赖外部输入，纯规则/权重计算决策
type AutoAIDecider struct {
    strategy AIStrategy // 可替换的策略实现：贪心、优先级脚本、随机等
}

func (d *AutoAIDecider) Decide(ctx context.Context, unit *BattleUnit, battle *BattleInstance) (*ActionCommand, error) {
    skill := d.strategy.ChooseSkill(unit, battle)     // 决定用哪个技能/普攻/宝物
    targets := d.strategy.ChooseTargets(unit, skill, battle)
    return &ActionCommand{
        ActorID:   unit.ID,
        SkillSlot: skill.Slot,
        TargetIDs: idsOf(targets),
    }, nil
}
```

AI 决策不需要等待网络输入，同步计算即可返回，天然没有超时问题。`AIStrategy` 是可替换的策略接口，不同难度/性格的AI通过替换策略实现，不改动 `AutoAIDecider` 本身。

### 7.5 单位级别的模式切换与统一执行

自动/手动的开关粒度落在**单位**层面（而非整场战斗一刀切），允许"我方英雄手动、其他单位自动"这类混合场景，也支持整场战斗随时一键切换：

```go
type BattleUnit struct {
    // ...已有字段（见4.1）
    decider ActionDecider // 当前生效的决策器，可动态替换
}

// SetAutoMode 切换单位的行动决策模式，战斗中随时可调用（如客户端点击"自动战斗"开关）
func (u *BattleUnit) SetAutoMode(auto bool, playerDecider, autoDecider ActionDecider) {
    if auto {
        u.decider = autoDecider
    } else {
        u.decider = playerDecider
    }
}
```

`executeAction`（5.3节）中调用的 `decideAndExecute`，统一处理决策、校验、目标解析、效果应用：

```go
func (b *BattleInstance) decideAndExecute(unit *BattleUnit) {
    cmd, err := unit.decider.Decide(b.ctx, unit, b)
    if err != nil {
        b.log.RecordError(unit, err)
        return // 决策失败（如ctx取消）视为本次行动放弃，不影响后续单位
    }

    source := unit.SkillSet.ResolveAttackSource(cmd.SkillSlot) // 见8.攻击来源统一模型
    if !b.validateCommand(unit, source, cmd) {
        // 服务端权威校验：客户端提交的技能可能CD未就绪/目标已死亡，必须二次校验，不能信任客户端
        b.log.RecordInvalidCommand(unit, cmd)
        return
    }

    preSelected := b.resolveUnitsByID(cmd.TargetIDs)
    targets := b.targetSelector.Resolve(source.TargetRule(), unit, preSelected, b.unitsA, b.unitsB)

    for _, target := range targets {
        if unit.StatusList.HasControl(ControlSilence) && source.IsMagic() {
            continue // 沉默：屏蔽法术类攻击来源，物理类不受影响（具体规则按策划设计）
        }
        ExecuteEffectChain(b, unit, target, source.Effects(), 0) // 效果执行细节见8.4节
    }

    source.OnUsed() // 技能进CD / 宝物消耗次数扣减，取决于具体来源类型
}
```

**关键约束——服务端二次校验**：客户端提交的 `ActionCommand` 只是"意图"，服务端必须重新校验：技能/宝物是否真的冷却或次数就绪、目标是否存活/在有效范围内、是否被沉默/禁锢限制了该类型的攻击。这一步不能省略，否则存在客户端伪造指令的安全风险（比如伪造CD未到的技能、攻击已死亡目标刷分等）。

## 8. 攻击来源统一模型（普攻 / 技能 / 宝物）

### 8.1 为什么需要统一模型

普通攻击、技能攻击、宝物（神器/法宝类道具）攻击，从"外部触发方式"上看差异很大：
- 普攻：无消耗，通常无CD或极短CD，随时可用
- 技能：有CD，可能消耗资源（怒气/法力）
- 宝物：可能有独立的使用次数限制（如"每场战斗限用1次"），也可能是被动触发（满足条件自动发动，不占用玩家的行动机会）

但从"命中后产生什么效果"的角度看，三者本质相同：都是"选定目标 → 结算一组 `SkillEffect`（伤害/治疗/施加状态）"。因此设计上让三者共享同一个效果结算管线（`applyAttackEffects`），只在"能不能用、要不要消耗资源"这一层保留差异。

### 8.2 统一接口

```go
// AttackSource 抽象普攻/技能/宝物等一切可以造成攻击效果的来源
type AttackSource interface {
    Slot() SkillSlotType         // 普攻 / 技能1~4 / 宝物1~2 等
    IsReady() bool                // 是否可用（CD/次数/触发条件）
    TargetRule() TargetRule       // 目标选择规则
    Effects() []SkillEffect       // 命中后产生的效果集合
    IsMagic() bool                // 是否属于法术类（用于沉默判定等规则）
    OnUsed()                      // 使用后的消耗结算（进CD/扣次数）
    IsActive() bool               // 是否需要占用一次行动机会（宝物可能是被动触发，不占用）
}

type SkillSlotType int32

const (
    SlotNormalAttack SkillSlotType = iota // 普通攻击
    SlotSkill1
    SlotSkill2
    SlotSkill3
    SlotSkill4
    SlotArtifact1 // 宝物1
    SlotArtifact2
)
```

### 8.3 三种具体实现

```go
// NormalAttack 普通攻击：无CD或固定极短CD，永远IsReady（除非被禁疗禁攻类效果限制）
type NormalAttack struct {
    effects []SkillEffect
}

func (a *NormalAttack) Slot() SkillSlotType   { return SlotNormalAttack }
func (a *NormalAttack) IsReady() bool          { return true }
func (a *NormalAttack) TargetRule() TargetRule { return TargetSingleEnemy }
func (a *NormalAttack) Effects() []SkillEffect { return a.effects }
func (a *NormalAttack) IsMagic() bool          { return false }
func (a *NormalAttack) OnUsed()                {} // 普攻无消耗，无需处理
func (a *NormalAttack) IsActive() bool         { return true } // 占用一次行动机会

// ActiveSkill 主动技能：有CD，需要行动机会（与第5节定义的ActiveSkill保持同一结构，此处补充接口实现）
func (s *ActiveSkill) Slot() SkillSlotType   { return s.SlotID }
func (s *ActiveSkill) IsReady() bool          { return s.CurrentCD <= 0 }
func (s *ActiveSkill) TargetRule() TargetRule { return s.targetRule }
func (s *ActiveSkill) Effects() []SkillEffect { return s.effects }
func (s *ActiveSkill) OnUsed()                { s.CurrentCD = s.Cooldown }
func (s *ActiveSkill) IsActive() bool         { return true } // 占用一次行动机会

// Artifact 宝物：可主动使用（占用行动机会，有使用次数限制），也可被动触发（不占用行动机会）
type Artifact struct {
    SlotID       SkillSlotType
    triggerMode  ArtifactTriggerMode // Active(主动使用) / Passive(被动触发)
    usesLeft     int32
    maxUses      int32
    triggerCond  TriggerCondition    // 被动触发时的条件（如"HP低于30%时自动发动"）
    effects      []SkillEffect
}

func (a *Artifact) Slot() SkillSlotType   { return a.SlotID }
func (a *Artifact) IsReady() bool          { return a.usesLeft > 0 }
func (a *Artifact) TargetRule() TargetRule { return a.targetRule }
func (a *Artifact) Effects() []SkillEffect { return a.effects }
func (a *Artifact) OnUsed()                { a.usesLeft-- }
func (a *Artifact) IsActive() bool         { return a.triggerMode == ArtifactTriggerActive }
```

**被动触发宝物的处理**：`IsActive() == false` 的宝物不出现在 `ActionCommand.SkillSlot` 的可选项里（玩家/AI无法主动选择），而是在特定事件时机（如"受到攻击时""HP低于阈值时"）由事件系统自动检查 `triggerCond` 并结算，复用与被动效果（`PassiveEffect`，见5.1节）相同的触发机制。两者的区别仅在于：被动效果通常是"固有能力"（无使用次数限制、无法被移除），被动触发宝物是"装备类资源"（有次数限制、可更换）。

### 8.4 攻击来源的统一效果结算（可扩展设计）

#### 8.4.1 设计问题

技能命中后产生的效果（伤害/治疗/施加状态）如果用单一 `switch` 语句处理，真实场景远不止这三种：驱散、护盾、复活、位移（击退/拉近）、召唤单位、免死、反弹伤害、资源回复（怒气/法力）等。而且比目标选择规则更复杂的是——**技能效果往往是一串按顺序执行的效果序列，后面的效果可能依赖前面效果的执行结果**（例如"造成伤害，若击杀目标则回复自身20%生命"）。因此除了用**执行器注册表**解决类型膨胀，还需要额外的**效果链**机制处理顺序依赖。

#### 8.4.2 效果执行器注册表

```go
// EffectExecutor 每种技能效果类型的独立执行单元
type EffectExecutor interface {
    Execute(execCtx *EffectExecContext) *EffectResult
}

// EffectExecContext 一次效果执行所需的完整上下文
type EffectExecContext struct {
    Battle *BattleInstance
    Caster *BattleUnit
    Target *BattleUnit
    Effect *SkillEffect      // 当前效果的配置（倍率、状态ID等）
    Chain  *EffectChainState // 效果链的累积状态，供后续效果读取前面效果的结果
    Depth  int               // 当前嵌套深度，防止效果内部触发子效果时无限递归（见8.4.6）
}

// EffectResult 单个效果执行后的结果，用通用键值存储而非固定字段，
// 避免每新增一种判断条件（如"是否暴击""是否击杀"）就要修改结构体
type EffectResult struct {
    Values map[string]float64 // 如 "damage_dealt", "is_crit", "target_died"
}

func (r *EffectResult) Get(key string) float64 {
    if r == nil {
        return 0
    }
    return r.Values[key]
}

var effectExecutorRegistry = map[EffectType]EffectExecutor{}

func RegisterEffectExecutor(t EffectType, executor EffectExecutor) {
    effectExecutorRegistry[t] = executor
}
```

#### 8.4.3 具体效果的独立实现（新增不改旧代码）

```go
// DamageExecutor 伤害效果：接入第9节的DamagePipeline，不重复实现伤害公式
type DamageExecutor struct{}

func (e DamageExecutor) Execute(ctx *EffectExecContext) *EffectResult {
    dmgCtx := &DamageContext{
        Attacker: ctx.Caster.AsCombatant(),
        Defender: ctx.Target.AsCombatant(),
    }
    dmgCtx.Value = ctx.ResolvedValue * ctx.Effect.Ratio // ResolvedValue默认是攻击力，也可引用前置效果结果，见8.4.4
    ctx.Battle.damagePipeline.Run(dmgCtx)
    ctx.Target.ApplyDamage(dmgCtx.Value)
    ctx.Battle.log.RecordDamage(ctx.Caster, ctx.Target, ctx.Effect.SourceSlot, dmgCtx)

    return &EffectResult{Values: map[string]float64{
        "damage_dealt": dmgCtx.Value,
        "target_died":  boolToFloat(ctx.Target.IsDead()),
        "is_crit":      boolToFloat(dmgCtx.WasCrit),
    }}
}

// HealExecutor 治疗效果：数值基准统一用ResolvedValue，不关心它来自攻击力还是上一步伤害
type HealExecutor struct{}

func (e HealExecutor) Execute(ctx *EffectExecContext) *EffectResult {
    healValue := ctx.ResolvedValue * ctx.Effect.Ratio
    ctx.Target.ApplyHeal(healValue)
    ctx.Battle.log.RecordHeal(ctx.Caster, ctx.Target, ctx.Effect.SourceSlot, healValue)
    return &EffectResult{Values: map[string]float64{"heal_amount": healValue}}
}

// ApplyStatusExecutor 施加状态效果
type ApplyStatusExecutor struct{}

func (e ApplyStatusExecutor) Execute(ctx *EffectExecContext) *EffectResult {
    status := gdconf.NewStatusEffect(ctx.Effect.StatusID)
    ctx.Target.StatusList.Add(status, ctx.Target)
    ctx.Battle.log.RecordStatus(ctx.Caster, ctx.Target, ctx.Effect.SourceSlot, status)
    return &EffectResult{}
}

func init() {
    RegisterEffectExecutor(EffectDamage, DamageExecutor{})
    RegisterEffectExecutor(EffectHeal, HealExecutor{})
    RegisterEffectExecutor(EffectApplyStatus, ApplyStatusExecutor{})
    // 未来新增：只加一行，不改前面的代码
    // RegisterEffectExecutor(EffectKnockback, KnockbackExecutor{})
    // RegisterEffectExecutor(EffectSummon, SummonExecutor{})
}
```

#### 8.4.4 效果链：处理顺序执行与条件/数值依赖

`SkillEffect` 增加"执行条件"和"数值引用"配置字段，让效果之间的依赖变成**配置层描述**，而不是在 `Executor` 代码里硬编码判断：

```go
type SkillEffect struct {
    Type       EffectType
    Ratio      float64
    StatusID   int32
    SourceSlot SkillSlotType    // 记录来自哪个AttackSource，供战报展示
    Condition  *EffectCondition // 执行条件，nil表示无条件执行
    ValueRef   *ValueReference  // 数值来源，若设置则Ratio作为该引用值的系数，而非默认的攻击力
}

// EffectCondition 引用效果链历史中某一步的某个字段做判断
type EffectCondition struct {
    StepIndex int       // 引用第几步的结果，-1表示上一步
    Field     string    // 如 "target_died"
    Operator  CompareOp // Equals / GreaterThan / LessThan
    Value     float64
}

// ValueReference 数值来源引用（替代"固定用攻击力*Ratio"的硬编码假设）
type ValueReference struct {
    StepIndex int
    Field     string // 如 "damage_dealt"
}

// EffectChainState 贯穿一次技能命中的多个效果，累积结果供后续效果读取
type EffectChainState struct {
    History []*EffectResult
}

func (c *EffectChainState) stepAt(index int) *EffectResult {
    if index < 0 {
        index = len(c.History) + index
    }
    if index < 0 || index >= len(c.History) {
        return &EffectResult{}
    }
    return c.History[index]
}

// ExecuteEffectChain 按配置顺序执行一串效果，取代原applyAttackEffects中的switch分支，
// 返回的chain可供外层（如批量AOE结算）读取整体执行历史
func ExecuteEffectChain(battle *BattleInstance, caster, target *BattleUnit, effects []*SkillEffect, depth int) *EffectChainState {
    const maxDepth = 5 // 递归深度保护，见8.4.6
    if depth > maxDepth {
        tlog.Errorf("effect chain exceeded max depth, possible circular reference, caster=%v", caster.ID)
        return &EffectChainState{}
    }

    chain := &EffectChainState{}
    for _, effect := range effects {
        if effect.Condition != nil && !evalCondition(effect.Condition, chain) {
            chain.History = append(chain.History, &EffectResult{}) // 占位，保持StepIndex对齐
            continue
        }

        executor, ok := effectExecutorRegistry[effect.Type]
        if !ok {
            tlog.Errorf("unknown effect type: %v", effect.Type)
            continue
        }
        execCtx := &EffectExecContext{
            Battle: battle, Caster: caster, Target: target,
            Effect: effect, Chain: chain, Depth: depth + 1,
            ResolvedValue: resolveValue(effect.ValueRef, chain, caster),
        }
        result := executor.Execute(execCtx)
        chain.History = append(chain.History, result)
    }

    // 触发被动效果的响应（如"造成伤害后有概率附加中毒"），无论攻击来源是什么，被动响应逻辑统一
    caster.PassiveEffects.OnAttackResolved(caster, target, chain)
    return chain
}

func evalCondition(cond *EffectCondition, chain *EffectChainState) bool {
    step := chain.stepAt(cond.StepIndex)
    val := step.Get(cond.Field)
    switch cond.Operator {
    case OpEquals:
        return val == cond.Value
    case OpGreaterThan:
        return val > cond.Value
    case OpLessThan:
        return val < cond.Value
    }
    return false
}

// resolveValue 统一处理"这个效果的数值基准是什么"，取代Executor内部各自写死Attack属性
func resolveValue(ref *ValueReference, chain *EffectChainState, caster *BattleUnit) float64 {
    if ref == nil {
        return caster.Attrs.Resolve(AttrAttack) // 默认基准：攻击力
    }
    return chain.stepAt(ref.StepIndex).Get(ref.Field)
}
```

"击杀后吸血"不需要专门写一个 `LifeStealOnKillExecutor`，而是**在配置层组合出来**：复用通用的 `HealExecutor`，配上条件（上一步 `target_died==1`）和数值引用（上一步的 `damage_dealt`）：

```go
skill.Effects = []*SkillEffect{
    {Type: EffectDamage, Ratio: 1.0},
    {
        Type:      EffectHeal,
        Condition: &EffectCondition{StepIndex: -1, Field: "target_died", Operator: OpEquals, Value: 1},
        ValueRef:  &ValueReference{StepIndex: -1, Field: "damage_dealt"}, // 数值来源=上一步的伤害
        Ratio:     0.2, // 吸血系数
    },
    {Type: EffectApplyStatus, StatusID: 1001},
}
```

#### 8.4.5 跨回合依赖与多目标批次关联

同技能内的条件/数值依赖归效果链管，但还有两类关联场景效果链不适合直接承载：

- **跨多次行动的累积触发**（如"连续攻击3次后引爆"）：不应该塞进单次技能的效果链（生命周期太短），而是复用第5节的状态效果系统，用一个隐藏的计数器 `StatusEffect` 通过事件总线累积攻击次数，达到阈值后再调用 `ExecuteEffectChain` 触发具体效果
- **一次AOE命中多个目标的整体关联**（如"若本次击杀数≥3，额外触发"）：需要在 `ExecuteEffectChain` 外层维护一个批次级状态 `BatchChainState{PerTarget map[int64]*EffectChainState; Aggregate map[string]float64}`，逐个目标调用效果链后再判断批次级效果，仅在明确需要感知"整批目标汇总结果"时才引入，避免所有AOE技能都强制走批次结算

三者职责边界：**同技能内依赖→效果链，跨时间累积→状态效果系统，跨目标汇总→批次层**，不用一套机制硬扛所有场景。

#### 8.4.6 效果嵌套：一个效果内部触发另一个效果

某些效果的执行逻辑需要在运行时动态调用另一套独立配置的效果（如"附加灼烧"这种多个技能共享的效果包），切入点是在 `EffectExecutor.Execute` 方法体内部，通过 `ctx.Battle` 反向调用 `ExecuteEffectChain`：

```go
// TriggerSubEffectExecutor 触发一个独立配置的可复用子效果组（如共享的"灼烧包"）
type TriggerSubEffectExecutor struct{}

func (e TriggerSubEffectExecutor) Execute(ctx *EffectExecContext) *EffectResult {
    subEffects := gdconf.GetSkillEffectGroup(ctx.Effect.SubEffectID)
    if subEffects == nil {
        return &EffectResult{}
    }
    // 子效果链拿到自己独立的EffectChainState，不污染外层链的历史记录；depth+1传递，防止无限递归
    ExecuteEffectChain(ctx.Battle, ctx.Caster, ctx.Target, subEffects, ctx.Depth)
    return &EffectResult{Values: map[string]float64{"sub_effect_triggered": 1}}
}
```

若子效果不是简单的效果组，而是要走一遍**完整的技能流程**（如"召唤一个单位并让它立即释放技能"，需要自己选目标），切入点要连带调用 `ctx.Battle.targetSelector.Resolve`，因为子技能不能复用父效果已经选好的目标。**任何嵌套调用都必须传递并检查 `Depth`**（8.4.4的 `maxDepth` 保护），防止策划配置错误（效果A的子效果又引用了A自己）导致无限递归。

#### 8.4.7 与整体设计的整合

`ExecuteEffectChain` 取代原来 `applyAttackEffects` 里的 `switch` 分支，`AttackSource.Effects()` 返回的 `[]SkillEffect` 语义不变（仍是"这次攻击命中后要执行的效果列表"），只是执行方式从硬编码分支改成注册表分发+链式状态传递。第7.5节 `decideAndExecute` 中调用效果结算的位置相应改为：

```go
for _, target := range targets {
    if unit.StatusList.HasControl(ControlSilence) && source.IsMagic() {
        continue
    }
    ExecuteEffectChain(b, unit, target, source.Effects(), 0) // depth从0开始
}
```

普攻、技能、宝物三种攻击来源（8.2~8.3节统一模型）复用同一套 `ExecuteEffectChain`，不需要各自实现。

## 9. 伤害计算的接入

第8.4.3节 `DamageExecutor.Execute` 中的伤害结算，实际调用的是《伤害计算管线设计方案》里的 `DamagePipeline`：

```go
dmgCtx := &DamageContext{
    Attacker: ctx.Caster.AsCombatant(),
    Defender: ctx.Target.AsCombatant(),
}
dmgCtx.Value = ctx.ResolvedValue * ctx.Effect.Ratio
ctx.Battle.damagePipeline.Run(dmgCtx)
```

`BattleUnit.AsCombatant()` 实现伤害计算框架定义的 `Combatant` 接口，是两套设计之间唯一的耦合点：**攻击来源系统（普攻/技能/宝物）不需要知道伤害计算内部的因子叠加细节（药水增减伤、士兵天赋等），伤害计算框架也不需要知道调用方是普攻、技能还是宝物**——`DamageExecutor` 只负责算出"这一次攻击的基础伤害值（`dmgCtx.Value`）该是多少"，剩下的增减伤修正、暴君/保底特殊效果、最终取整全部交给 `DamagePipeline` 统一处理。这正是第8节"攻击来源统一模型"能够成立的关键：三种来源只在"能不能用、消耗什么资源"上有差异，一旦进入效果结算阶段，走的是完全相同的伤害管线，且 `DamageExecutor` 本身只是效果执行器注册表（8.4.2节）里众多 `EffectExecutor` 实现中的一个，与治疗、施加状态等其他效果类型享有相同的执行契约。

## 10. 战报与回放

```go
type BattleLog struct {
    Events []BattleEvent
}

type BattleEvent struct {
    Tick      int
    Type      EventType // ActionStart/Damage/Heal/StatusApplied/StatusExpired/Death/Victory/AutoTakeover/InvalidCommand
    ActorID   int64
    TargetID  int64
    SkillSlot SkillSlotType // 记录本次效果来自普攻/哪个技能/哪个宝物
    Value     float64
    Detail    string // 复用 DamageContext.Trace 里的公式细节
}
```

**设计要点**：
- 所有随机数（暴击、闪避、AI目标选择的随机项）必须由**独立的、带种子的随机源**驱动，且种子作为战报的一部分持久化——这样同一份种子+同样的初始状态可以完全复现整场战斗，用于回放、观战、争议排查
- 事件粒度到"每一次伤害/状态变化"，而非只记录最终结果，客户端表现层（打击特效、飘字）依赖完整事件流播放
- 人工模式的超时托管（7.3节）、服务端校验失败（7.5节）都作为独立事件类型记录，便于事后排查"这局为什么自动接管了""客户端为什么被拒绝执行"

## 11. 关键设计取舍总结

| 问题 | 取舍 | 原因 |
|---|---|---|
| 时序驱动方式 | 行动值累积（ATB）而非固定回合 | 天然支持速度差异、多次行动 |
| 属性修改方式 | 分层实时计算，不缓存最终值 | 避免buff增删时忘记回滚的数值bug |
| buff持续时间计时基准 | 按单位自身行动次数，非全局tick | 避免速度不同导致buff实际收益失衡 |
| 1v1/1vN是否区分模式 | 不区分，由技能TargetRule决定范围 | 核心逻辑统一，减少特殊分支 |
| 随机数来源 | 独立带种子随机源，种子持久化 | 支持战斗回放与结果复现 |
| 技能/被动/状态是否用同一接口 | 分离建模（三层模型） | 语义不同（触发方式、生命周期），强行统一会增加理解成本 |
| 人工/自动决策是否用同一接口 | 统一 `ActionDecider` 接口，产出同一种 `ActionCommand` | 执行层不关心决策来源，逻辑单一入口，便于校验和审计 |
| 人工模式下等不到指令怎么办 | 超时后自动转交AI兜底，而非阻塞战斗 | 服务端权威战斗不能因客户端掉线/卡顿而卡死 |
| 客户端指令是否直接信任执行 | 服务端二次校验CD/目标合法性/状态限制 | 防止客户端伪造指令导致的安全风险 |
| 普攻/技能/宝物是否各自独立流程 | 统一 `AttackSource` 接口，共享同一套效果结算管线 | 三者仅在"能不能用、消耗什么"上有差异，命中后处理逻辑一致 |
| 被动触发宝物如何接入 | 复用被动效果的事件触发机制，不占用行动机会 | 与主动使用类攻击来源解耦，同一套Effect结构复用 |
| 目标选择规则如何应对种类膨胀 | 策略注册表（`TargetRuleHandler`），组合场景用Filter+Picker两层拆分 | 新增规则只做加法不改旧代码，组合需求不必逐一定义新Handler |
| 技能效果如何应对种类膨胀 | 执行器注册表（`EffectExecutor`），同样新增不改旧代码 | 与目标选择规则共用同一套设计原则 |
| 效果之间的顺序依赖（如击杀后吸血）如何表达 | 效果链（`EffectChainState`）+ 配置层的条件/数值引用 | 依赖关系变成策划可配置的数据，不需要为每种依赖组合写专用Executor |
| 跨回合累积触发（如连击3次引爆）归谁管 | 复用状态效果系统的隐藏计数器，触发时才调用效果链 | 效果链生命周期只有一次技能命中，太短，不适合承载跨时间状态 |
| 效果内部触发另一个效果如何做 | Executor内部通过`ctx.Battle`反向调用`ExecuteEffectChain`，必须传递递归深度 | 复用同一套效果执行入口；深度保护防止配置错误导致无限递归 |
| 超时保护 | maxTicks兜底 | 防止极端配置下战斗无法结束 |

## 12. 后续可扩展点

- **AI策略**：`AIStrategy`（7.4节）是接口占位，可替换为不同难度的AI实现（贪心选最优目标、随机、脚本化boss AI等），与核心战斗流程解耦
- **多单位编队（集结）**：若未来需要支持"多个友方单位组成一队"，可在 `BattleUnit` 之上加一层 `Formation`，`unitsA`/`unitsB` 存储的是展开后的所有个体单位，编队仅影响出场顺序/站位加成，不改变本设计的核心调度逻辑
- **技能配置化**：`ActiveSkill`/`StatusEffect`/`Artifact` 的具体数值应从策划配置表加载（类似现有 `SlgSkillCfg`/`SlgEffectCfg`），代码只提供解析后的运行时结构，避免硬编码
- **指令队列/预输入**：当前设计是"逐单位阻塞等待指令"，如果未来需要支持"提前排好后续几个行动再连续演出"，可以在 `PlayerInputDecider` 前增加一层指令队列缓冲，不改变核心执行流程

## 13. 完整生命周期：从创建到结束

前面各节分别说明了各子系统的设计，本节把它们串联成一场战斗从创建到结束的完整时间线。

### 13.1 生命周期总览

```
创建阶段(Setup)
   │
   ▼
主循环(Run) ──────────────────┐
   │                          │
   ├─ 速度条推进(Tick)          │ 每次循环重复
   ├─ 单次行动执行(ExecuteAction)│
   │    ├─ 状态效果结算          │
   │    ├─ 控制类检查(跳过判断)   │
   │    ├─ 决策(人工阻塞/AI同步) │
   │    ├─ 服务端权威校验        │
   │    ├─ 目标解析              │
   │    ├─ 效果结算(接入伤害管线) │
   │    └─ CD/状态收尾           │
   ├─ 胜负判定(每次行动后检查)    │
   └──────────────────────────┘
   │ (一方全灭 / 平局 / 超时)
   ▼
结算阶段(Finalize)
   │
   ▼
结果输出与回放(Result & Replay)
```

### 13.2 阶段一：创建（Setup）

```go
func NewBattleInstance(sideA, sideB []*UnitConfig, mode BattleMode) *BattleInstance {
    battle := &BattleInstance{
        scheduler: NewActionBarScheduler(),
        log:       NewBattleLog(),
        rng:       NewSeededRandom(genSeed()), // 生成并记录随机种子，用于后续回放
        maxTicks:  defaultMaxTicks,
        ctx:       context.Background(),
    }

    // 1. 根据配置实例化双方 BattleUnit：属性(4.2节)、技能集/攻击来源(8节)、初始状态
    battle.unitsA = buildUnits(sideA, SideA)
    battle.unitsB = buildUnits(sideB, SideB)

    // 2. 装载每个单位的初始决策器（人工/自动），初始模式由请求参数(mode)决定，
    //    并注册进速度条调度器
    for _, u := range append(battle.unitsA, battle.unitsB...) {
        u.decider = resolveInitialDecider(u, mode) // 见7.5节 SetAutoMode 的初始化版本
        u.battle = battle                          // 供 rng、log 等共享上下文访问（如6节 TargetRandomN）
        battle.scheduler.Register(u)
    }

    // 3. 触发"进入战斗"类被动效果（如"战斗开始时获得护盾"），复用被动效果触发机制(5.1节)
    battle.fireBattleEvent(EventBattleStart)

    battle.log.RecordSetup(sideA, sideB, battle.rng.Seed())
    return battle
}
```

创建阶段完成三件事：实例化参战单位（属性/技能/攻击来源就位）、确定每个单位的初始决策模式、生成并记录随机种子（保证后续整场战斗可复现）。

### 13.3 阶段二：主循环（Run）

对应3.2节的 `Run()`，逐 tick 推进速度条，每次有单位达到行动阈值就调用 `executeAction`：

```go
func (b *BattleInstance) Run() *BattleResult {
    for tick := 0; tick < b.maxTicks; tick++ {
        b.currentTick = tick
        if result := b.checkVictory(); result != nil {
            return b.finalize(result)
        }
        readyUnits := b.scheduler.Tick()
        for _, unit := range readyUnits {
            if unit.IsDead() {
                continue // 达到行动阈值后、排队执行前死亡（被其他单位杀死）
            }
            b.executeAction(unit)
            unit.Consume()
            if result := b.checkVictory(); result != nil {
                return b.finalize(result)
            }
        }
    }
    return b.finalize(b.timeoutResult())
}
```

**主循环的特殊性**：这不是一个纯粹连续计算的循环。当轮到的单位处于人工模式时，`executeAction` 内部的 `decideAndExecute`（7.5节）会通过 `PlayerInputDecider.Decide` 阻塞等待客户端指令（带超时兜底）。也就是说，主循环实际上是"计算 tick → 可能阻塞等待人工输入 → 继续计算"交替进行的状态机，纯自动战斗（所有单位都是AI）时才会退化为连续计算无阻塞。

单次行动执行的完整步骤（汇总5.3节+7.5节）：
1. 回合开始类状态效果结算（如中毒扣血）
2. 控制类状态检查（眩晕则跳过行动，但CD仍正常流转）
3. 决策阶段：`unit.decider.Decide()` 得到 `ActionCommand`（人工阻塞等待或AI同步返回）
4. 服务端权威校验：技能/宝物是否就绪、目标是否合法
5. 目标解析：`TargetSelector.Resolve()`，AOE/全体规则自动展开
6. 效果结算：`ExecuteEffectChain()`，按注册表分发到具体效果执行器并处理链式依赖，接入伤害计算管线（9节），触发被动响应
7. 攻击来源消耗结算：`source.OnUsed()`（技能进CD/宝物扣次数）
8. 技能集CD递减、状态效果回合结束结算与过期清理

### 13.4 阶段三：胜负判定（贯穿全程）

`checkVictory()` 在每次单位行动**之后**立即调用（而非等一整轮所有单位行动完），避免"死亡单位后续仍参与行动"的逻辑错误：

```go
func (b *BattleInstance) checkVictory() *BattleResult {
    aDead := allDead(b.unitsA)
    bDead := allDead(b.unitsB)
    switch {
    case aDead && bDead:
        return &BattleResult{Draw: true}
    case aDead:
        return &BattleResult{WinnerSide: SideB}
    case bDead:
        return &BattleResult{WinnerSide: SideA}
    }
    return nil
}
```

三种终止条件：一方全灭（对方胜）、双方同时全灭（如同归于尽的AOE反伤，判平局）、达到 `maxTicks` 仍未分出胜负（超时兜底，按业务规则判平局或按剩余总血量比例判定）。

### 13.5 阶段四：结算（Finalize）

```go
func (b *BattleInstance) finalize(result *BattleResult) *BattleResult {
    result.Rounds = b.currentTick
    result.Seed = b.rng.Seed()
    result.Events = b.log.Events // 完整事件流，供客户端回放演出

    // 结算类被动/效果响应（如"胜利时额外掉落"），复用被动效果触发机制
    b.fireBattleEvent(EventBattleEnd, result)

    // 落地持久化：扣血结果同步回玩家数据、发放奖励、写入战报存储
    b.persistResult(result)

    return result
}
```

结算阶段做三件事：汇总最终结果（回合数、随机种子、完整事件流）、触发战斗结束类的被动/效果响应、将结果持久化（同步玩家数据、发奖励、存战报）。

### 13.6 阶段五：结果输出与回放

- 服务端将 `BattleResult`（胜负、完整事件流、随机种子）返回给调用方（PVP匹配系统、副本挑战系统等业务模块）
- 客户端拿到事件流后本地播放战斗演出（打击特效、飘字、技能动画）：由于随机种子和事件序列都已确定，本地播放**不需要再做任何随机判定**，纯粹是表现层按事件流回放
- 战报可持久化存储，支持后续查看、申诉复核、数据分析

### 13.7 全流程一句话总结

**创建**（实例化双方单位、装载决策模式、生成随机种子）→ **主循环**（速度条推进，逐单位决策→校验→目标解析→效果结算→收尾，人工模式下可能阻塞等待）→ **胜负判定**（每次行动后实时检查，或超时兜底）→ **结算**（生成完整结果对象、触发收尾效果、持久化）→ **回放**（客户端依据确定性事件流+随机种子还原整场战斗，无需二次随机判定）。
