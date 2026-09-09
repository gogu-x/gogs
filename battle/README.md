# battle —— 确定性战斗结算服务

> 独立进程、**确定性（deterministic）** 的回合制战斗结算服务：同样的「配置版本 + seed + 参战单位」输入，在任何机器、任何时候都产出**完全一致**的事件流与结果。结果以回放（Replay）落库，可随时重放、校验、对账。
>
> 服务采用 **server-manager + 每场战斗一个独立 Actor** 的架构：`BATTLE_SERVICE` 只负责创建/登记/回收 per-battle actor；每场战斗的结算与结果分发（Created/Action/Finished/确认）都在它自己的 actor 内串行自洽，actor 间互不阻塞。战斗配置是独立公共模块，位于 `glconf/battlecfg`。

职责分层：

- **`glconf/battlecfg`（公共配置模块）**：引擎配置的**数据面 + 输入面**类型、枚举、校验、稳定哈希、多版本配置仓库、JSON 加载与内置 default。任何进程（结算、校验、对账、工具）都可复用，只依赖标准库。
- **`battle/engine`（战斗规则引擎）**：纯结算与事件面，无副作用、可离线重放。输入配置 + `battlecfg.BattleInput`，输出 `Result`（事件流 / 单位终态 / 胜负 / 校验和 / Replay）。**不含网络与存储依赖。**
- **`battle/actor`（per-battle actor 模块）**：每场战斗的执行 actor + 它的**战报领域**。`BattleActor` 一场战斗一个，独立 goroutine 自洽完成"Created→结算→落库→Action/Finished→确认/重试→退出"；`Report`/`Repository`/`Source`/`Notifier` 这些战报模型与契约也归属于此。不反向依赖 service。
- **`battle/service`（manager 模块）**：与 game/play、game/guild 等模块同构的 **external 薄壳 + internal 实现**。manager actor 只负责创建/登记/回收 `battle/actor` 派生的 per-battle actor 与只读查询；持久化（Mongo/内存）与 NATS 通知的**实现**提供给它注入的 actor。

---

## 1. 目录结构

```
glconf/battlecfg/             # 公共战斗配置模块（import gogs/glconf/battlecfg）
│   ├── types.go              # Config / UnitConfig / SkillConfig / StatusConfig / EffectConfig /
│   │                         #   AttributeModifier + Team/TargetRule/EffectKind/StatusKind +
│   │                         #   CombatantInput / BattleInput
│   ├── config.go             # Config.Validate / BattleInput.Validate / cloneConfig
│   ├── hash.go               # HashJSON / CanonicalInput / StableConfigHash / StableInputHash
│   ├── repository.go         # ConfigRepository 接口 + MemoryConfigRepository（多版本、不可变、指纹）
│   ├── loader.go             # LoadConfigRepository(path)：读 JSON(object/array) 逐版 Put
│   └── default.go            # DefaultConfig()（version="default" 的开发配置）
battle/
├── main.go                   # 进程入口：加载配置 → etcd 注册 → Mongo → 拉起 manager 与 NATS Actor
├── e2e_test.go               # 全链路端到端测试（无任何外部依赖）
├── engine/                   # 确定性结算引擎（纯逻辑库，不含网络/存储/actor）
│   ├── types.go              # 事件/结果面：EventType/Event/Outcome/UnitResult/Result（Team 复用 battlecfg）
│   ├── battle.go             # 战斗主循环：ATB、行动、技能、目标、效果、状态结算
│   ├── damage.go             # 伤害管线 DamagePipeline（5 段式，可插拔）
│   ├── rng.go                # SplitMix64 确定性随机数（跨平台固定输出）
│   ├── replay.go             # Replay / resultChecksum / VerifyReplay（离线重放校验）
│   └── *_test.go
├── actor/                    # per-battle actor 独立包（每场战斗 + 战报领域）
│   ├── report.go             # 战报模型：Report / Repository 接口 / ErrReportNotFound
│   ├── types.go              # Source（回投目标）/ Notifier 接口
│   ├── convert.go            # pb ⇄ engine/Report 双向转换（InputFromProto/ReplayFromProto/ResultToProto…）
│   ├── actor.go              # BattleActor：一场一 actor（结算、分发、确认、重试、退出）+ Params/Mode
│   └── *_test.go
└── service/                  # manager 模块（external 薄壳 + internal 实现，同 game 模块）
    ├── external.go           # 壳：New(...) + 类型 re-export（Source/Notifier/Report 源自 actor；Options/NATSNotifier 源自 internal）
    └── internal/
        ├── service.go        # Service（manager）Actor：编排 + active/retiring/recent 登记 + 只读查询
        ├── router.go         # manager 消息路由注册（tree.Router 分发）
        ├── repository.go     # Repository 实现：内存 / Mongo（战报存取装配层）
        ├── notifier.go       # NATSNotifier：把通知回投给发起方 Game 节点（actor.Notifier 实现）
        └── *_test.go
```

## 2. 概览：定位与协作角色

| 名称 | 常量/值 | 说明 |
|---|---|---|
| battle 服务进程 | — | 独立进程，可多实例（`node-id` 区分） |
| 服务 manager Actor | `def.BattleService = "BATTLE_SERVICE"` | battle 进程内收请求、创建/登记/回收 per-battle actor |
| per-battle Actor | `def.BattleActorName(battleID)` = `"BATTLE_ACTOR_" + battle_id` | 运行期动态创建，一场战斗一个；可被跨进程按名直达 |
| NATS 订阅模块 | `natsrpc.Battle = "Battle"` | 对 battle 服务的寻址模块 |
| etcd 服务类型 | `def.ServiceBattle = "Battle"` | 通用服务发现里的服务名，供 Game 侧发现节点 |
| Game 侧客户端 Actor | `def.BattleClient = "BATTLE_CLIENT"` | 见 `game/battleclient`，接收生命周期事件、回确认 |

一次战斗的完整跨进程链路：

```
Game 进程                            NATS / etcd                     battle 进程
battleclient.Client ──发现节点(etcd,轮询)──────────────────────────► BATTLE_SERVICE(manager)
  Begin ──Cast StartBattleReq──►(natsrpc.Battle/BATTLE_SERVICE)──►  校验→幂等判定→SpawnOne
  ◄──BattleCreatedNtf◄───────────────────────────────────────────   每场独立 BattleActor:
  ◄──BattleActionNtf ×N◄──────────────────────────────────────────    Created→结算→Save→Action→
  ◄──BattleFinishedNtf◄───────────────────────────────────────────    Finished→挂起重试
  ──Cast BattleResultConfirmedNtf──►(TaggerName=BATTLE_ACTOR_<id>)►  actor 收确认→落 Confirmed→Stop
  （直达战斗 actor，不经 manager 中转）                          ◄── actorStopped→manager 清理登记
```

> 可靠性不变量：**任何 Action/Finished 通知发出前，Report 必须先成功落库**——结果以落库为准，投递只是尽力送达。

---

## 3. 启动与配置

### 3.1 进程引导（`main.go`）

1. `conf.LoadAndApply`：读命令行/配置文件连接参数。
2. `tlog.NewLog` 初始化日志。
3. `battlecfg.LoadConfigRepository(conf.BattleConfigPath)`：加载引擎配置仓库（公共模块）。
4. `cluster.Init(EtcdEndpoints)` + `RegisterService(def.ServiceBattle, …)`：etcd 服务注册（失败仅告警）。
5. `mongorpc.Connect(…,"battle")` + `NewMongoRepository(db,"battle_reports")`：连接结果库。
6. `tree.Spawn(service.New(configs, reports, NATSNotifier{}, Options{…}), natsrpc.NewNats(natsrpc.Battle, …))`：把 manager 与 NATS Actor 挂到 `tree.Default()` 并启动。

### 3.2 相关启动参数（`conf/config.go`）

| Flag | 默认 | 含义 |
|---|---|---|
| `--server-id` / `--node-id` | `1` | 区服 / 节点 ID（寻址维度） |
| `--etcd` | 逗号分隔列表 | etcd 端点 |
| `--nats-url` | `nats://…` | NATS 地址 |
| `--mongo-url/-username/-password` | `mongodb://…` | MongoDB（库名固定 `battle`） |
| `--battle-base-port` / `--battle-host` | — | 服务发现地址 = `host:base+node-id` |
| `--battle-config` | 空 | 引擎 JSON 配置路径；为空用内置 `default` 开发配置 |
| `--battle-retry-count` | `3` | Finished 投递重试上限 |
| `--battle-retry-delay-ms` | `1000` | Finished 投递重试间隔(ms) |

> 生产建议用 `--battle-config` 挂载**不可变、带版本号**的 JSON（多版本并存，`battlecfg.MemoryConfigRepository` 保证同版本内容不可变并校验稳定指纹）。

---

## 4. 公共配置模块（`glconf/battlecfg`）

引擎玩法全部由配置数据驱动。配置是**数据面 + 输入面**的公共类型，独立于结算引擎：

| 分类 | 内容 |
|---|---|
| 数据面 | `Config{version, atb_threshold, max_actions, damage_variance/crit_* (‰), units, skills, statuses}`；`UnitConfig/SkillConfig/StatusConfig/EffectConfig/AttributeModifier`；枚举 `Team/TargetRule/EffectKind/StatusKind` |
| 输入面 | `CombatantInput/instance_id+config_id+team+position`；`BattleInput{config_version, seed, combatants[]}` |
| 校验 | `Config.Validate` / `BattleInput.Validate`（启动与每次建战前全量校验） |
| 稳定哈希 | `StableConfigHash` / `StableInputHash` / `CanonicalInput` / `HashJSON`（结算侧 BattleID、checksum 也复用，**单一实现**） |
| 仓库 | `ConfigRepository`(Put/Get) + 内存实现：多版本、同版本内容不可变（指纹比对）、返回深拷贝 |
| 加载 | `LoadConfigRepository(path)`：读 object/array JSON 逐版 Put；空 path 用内置 `DefaultConfig()` |

引擎 JSON 配置示例（与内置 default 等价；`target_rule`/`kind` 为**枚举数值**，见 `battlecfg/types.go`）：

```jsonc
{
  "version": "default",
  "atb_threshold": 100,
  "max_actions": 100,
  "damage_variance_permille": 0,
  "crit_chance_permille": 0,
  "crit_multiplier_permille": 1500,
  "units": {
    "hero":  {"id":"hero",  "max_hp":120,"attack":35,"defense":8, "speed":20,"basic_skill_id":"attack","active_skill_ids":[]},
    "enemy": {"id":"enemy", "max_hp":100,"attack":25,"defense":6, "speed":15,"basic_skill_id":"attack","active_skill_ids":[]}
  },
  "skills": {
    // target_rule: 1=敌方单体(enemy_single)；effects[].kind: 1=伤害(damage)
    "attack": {"id":"attack","cooldown":0,"target_rule":1,"effects":[{"kind":1,"coefficient_permille":1000,"flat":0}]}
  },
  "statuses": {}
}
```

---

## 5. 战斗规则（`engine/battle.go` / `damage.go`）

### 5.1 ATB 行动时序

所有存活单位各自积累行动条 `gauge`（每 tick 增加 `speed`）。每次循环挑选**最快攒满阈值**的单位行动；多个同时就绪时按「gauge 高 → speed 高 → position 低 → instance_id 字典序」稳定排序取首个。先到 `max_actions` 上限仍未分出胜负则判平（Draw）。

### 5.2 一个单位一次「行动」（`Battle.execute`）

1. `EventActionStarted`（行动序号 `action` +1）。
2. `tickCooldowns`：冷却按该单位自身行动次数递减。
3. `tickPeriodic`：DOT/HOT 在该单位行动时结算一次。
4. 若存活：`Stun` → 跳过（`EventActionSkipped, detail="stun"`）；否则 `chooseSkill`（非沉默挑第一个冷却好的主动技能，否则普攻）→ 按 `target_rule` 选目标 → `EventSkillUsed` 后对每个效果×目标 `applyEffect` → 给主动技能上冷却。
5. `expireStatuses`：自身状态剩余回合 -1，归零移除。
6. `gauge -= atb_threshold`（不低于 0），`EventActionEnded`。

### 5.3 目标选择规则

| `TargetRule` | 行为 |
|---|---|
| `enemy_single` | 敌方存活者按序取 1 |
| `enemy_all` | 敌方全部存活者 |
| `ally_lowest_hp` | 我方存活者中 HP% 最低者取 1 |
| `self` | 自身 |

### 5.4 效果与伤害公式

- **伤害管线**（`NewDamagePipeline`，每步可插拔 `DamageStage`）：① `amount = attack - defense`（≥1）→ ② `amount = amount*coeff/1000 + flat` → ③ 浮动 ±variance ‰ → ④ 暴击判定（命中 `critical=true`、`*crit_multiplier/1000`）→ ⑤ 兜底 ≥1。最终伤害不超过目标当前 HP。
- **治疗**：`amount = attack*coeff/1000 + flat`，不超最大 HP。
- **附加状态**：覆盖式刷新为完整时长，否则追加；发 `EventStatusApplied`。

### 5.5 状态机制（`StatusKind`）

| 状态 | 行为 |
|---|---|
| `stun` | 行动被跳过 |
| `silence` | 只能用普攻 |
| `dot` / `hot` | 单位自己行动时结算持续伤害/治疗 |
| `attribute_modifier` | 提供攻击/防御/速度**平值**修正（存活状态 `modifier` 累加） |

---

## 6. 确定性与回放校验（`rng.go` / `battlecfg` 稳定哈希 / `replay.go`）

1. **确定性 RNG**：`SplitMix64`，全程 `uint64`、固定种子线性推进，跨平台恒定（`TestFixedRNGTable` 锁表）。
2. **规范哈希**（`battlecfg`）：map 先排序再序列化（`CanonicalInput`、`StableConfigHash` 按 ID 排序、固定 JSON 字段序）。
3. **结果校验和**：对 `battle_id+config_hash+outcome+tick+units+单位终态(hp/gauge/cooldowns/statuses)+events[]` 做 SHA-256。任何行为漂移/回放篡改都会令 checksum 不一致。
4. **`VerifyReplay`**：拿 Replay 内输入 + 配置版本**整局重跑**，比对 BattleID / 配置指纹 / 校验和 / 事件流。battle 服务的 `VerifyBattleReplay` 请求即基于此。

这套机制让**作弊检测、客服对账、bug 复现、跨版本迁移**都以"回放重算"为锚点。

---

## 7. Actor 架构（manager 在 `battle/service`，per-battle actor 在 `battle/actor`）

### 7.1 职责切分

**战报（Report）归各自 per-battle actor 所有**：结算、落库、推送、确认、重试都在战斗 actor 内完成；manager 不持有、不组织任何战报，只做调度与只读查询。

| 消息 | 处理层 | 说明 |
|---|---|---|
| `StartBattleReq` | manager | 校验 → 幂等判定（`active` 命中 / 库中已有）→ 一律 `SpawnOne` 一个 actor：全新 → run-actor；库已有（确认与否）→ redeliver-actor |
| `RebuildBattleReq` | manager | 只读库取 Replay 重建并核 ID → 编排 spawn run-actor（若 active 先让位再替换） |
| `QueryBattleReq` / `VerifyBattleReplayReq` | manager | 只读查询：读库回历史结果 / `engine.VerifyReplay`（与任何活跃 actor 无关） |
| `QueryPlayerRecent` | manager | 进程内 recent 索引 |
| `BattleResultConfirmedNtf` | **per-battle actor** | 按 `BATTLE_ACTOR_<id>` 直达；幂等落 `Confirmed` 后停 |
| `retryTick` / `retireBattle` / `actorStopped` | actor / manager | actor 自触发重推 Finished；manager 请 actor 让位；actor 停后通知 manager 清理 |

### 7.2 manager（`Service`）：只编排、不碰战报

- 结构：`active map[battleID]PID`（在跑/等确认的 actor）、`retiring map[battleID]*spawnSpec`（rebuild 让位队列）、`recent map[uid][]recentBattle`、只读 `reports`。字段只在 manager 自身 goroutine 访问，无需锁。
- `OnInit` 记录 `self/system` 供之后 `SpawnOne`；`notifier` 仅转交 actor，manager 自身不再推送任何 Created/Action/Finished。
- `start`：先看 `active`（命中 → 幂等回受理回执，Created 由该 actor 自洽推）；再看库（**已有 → spawn redeliver-actor**：已确认 → 补发 Created/Action/Finished 后自停；未确认 → 重投并重试等确认）；否则 `addRecent` + spawn run-actor。manager 不读战报内容组织投递。
- `release`（收 `actorStopped`）：比对 `active[battleID]==pid` 后删除登记；若存在 `retiring` 队列则用其 spec 重新 `spawnRun`——**同一 battleID 的替换 actor 只在旧 actor 注销后才生成**，规避注册名单槽覆盖。
- `confirm`（只读落库兜底）：兼容旧客户端把确认发到 `BATTLE_SERVICE`——转发给活跃 actor；无活跃 actor 时直接幂等落库。

### 7.3 per-battle actor（`battle/actor` 的 `BattleActor`）

注册名 `def.BattleActorName(battleID)`，独立 goroutine、独立 mailbox（256）。生命周期全部消息驱动、幂等：

- `OnInit` → `begin`：推 `Created` →（run 模式）`battle.Run()` + **先 `Save(report)`** → 推 Action 流 → 推 `Finished` → 未确认则挂定时重试。（redeliver 模式复用库中结果，把投递目标覆盖为当前请求方；若库中结果**已确认**，则补发完成后直接退出，不再重试、不悬挂。）
- `BattleResultConfirmedNtf`（直达）→ `reports.Confirm`（幂等）→ `finish`。
- `retryTick` → 已达 `RetryAttempts` 上限则静默 `finish`（结果保留未确认）；否则重推 `Finished`。
- `retireBattle` → 让位 `finish`（rebuild 场景）。
- `finish`（`released` 幂等）：停定时器 → `system.Send(managerPID, &actorStopped{battleID,pid})` → `ctx.Stop()`（OnStop 停表、自动从注册名注销）。

> 无父子级联停止：进程退出时 `tree.Shutdown()` 会遍历停所有 actor。NATS 投往已停 actor 的重复确认仅 `Lookup` 失败并被丢弃（报告已 `Confirmed`，无害）。

---

## 8. 持久化（`service/internal/repository.go`）

`Report` 是"结算落库 → 结果投递"的持久化交接物：

```go
type Report struct {
    BattleID, UID, SourceServerID, SourceNodeID
    Result   engine.Result   // 含完整事件流与 Replay（Input 为 battlecfg.BattleInput）
    CreatedAt, UpdatedAt
    Confirmed bool           // Game 侧已确认
}
```

`Repository` 接口：`Save / Get / ListByPlayer / Confirm`。生产用 `MongoRepository`（库 `battle`、默认集合 `battle_reports`，`_id=battle_id` upsert，8s 超时）；`MemoryRepository` 供单测与 e2e（深拷贝、并发安全，不依赖 Mongo/NATS）。

---

## 9. 跨进程协议

### 9.1 寻址约定

- **创建**（Game→manager）：`natsrpc.Cast(natsrpc.Battle, def.BattleService, node.ServerID, node.NodeID, StartBattleReq)`。
- **确认/战斗内消息**（Game→per-battle actor）：`natsrpc.Cast(natsrpc.Battle, def.BattleActorName(msg.BattleId), node.ServerID, node.NodeID, msg)` —— 目标从 manager 换成该场战斗 actor 名，NATS 层在 battle 进程内按 `TaggerName` Lookup 直达。
- **结果通知**（actor→Game）：`natsrpc.Cast(natsrpc.Game, def.BattleClient, source.ServerID, source.NodeID, Created/Action/Finished)`，`source` 即发起请求的 Game 节点。

### 9.2 消息总表（`pb/cspb/pb_battle`）

| 消息 | 方向/层 | 说明 |
|---|---|---|
| `StartBattleReq` | Game→manager | 开始战斗；幂等回执为 `BattleCreatedNtf` |
| `QueryBattleReq` / `RebuildBattleReq` / `VerifyBattleReplayReq` | →manager | 查询 / 重建(重放) / 校验 |
| `BattleResultConfirmedNtf` | Game→**actor 名** | 确认已收 Finished → 落 Confirmed → actor 停 |
| `BattleCreatedNtf` | actor→Game | 创建回执 |
| `BattleActionNtf` ×N | actor→Game | 结算后逐事件推送（按 `sequence` 升序） |
| `BattleFinishedNtf` | actor→Game | 事件推送完毕后发出（含 outcome/units/checksum/replay） |

`battleclient`（Game 侧）状态机 `Idle→Creating→CreateUnknown→Running→Finished`；事件按 `sequence` 去重、重复 `Finished` 幂等；对 NATS `NATSSender.Confirm` 目标即为 per-battle actor 名（新架构）。最终经 `GameGate` 下推玩家会话。

---

## 10. 测试

`go test ./battle/... ./glconf/battlecfg/... ./game/battleclient/...`。**单测与 e2e 均无需 Mongo/NATS/etcd**：

| 文件 | 覆盖点 |
|---|---|
| `glconf/battlecfg/config_test.go` | 校验、稳定哈希、配置仓库（深拷贝/幂等/冲突/缺失）、default 加载、canonical 排序 |
| `engine/rng_damage_test.go` | 固定 RNG、伤害管线（表驱动锁表） |
| `engine/battle_test.go` / `replay_test.go` | ATB/targets/AI/状态/效果链/事件流；BattleID+checksum 确定性、Replay 校验 |
| `service/service_test.go` | Report 深拷贝/确认、生命周期+recent+rebuild、**多场 actor 隔离**（第一场 Save 阻塞时第二场独立完成）、Finished 重试至上限 |
| `e2e_test.go` | `localBridge`+`e2eGate` 单进程模拟 Game 侧：Begin → per-battle actor 推 Created/Action/Finished → client 按 actor 名回 Confirm → Report Confirmed 全链路 |

---

## 11. 扩展指引

纯配置可扩展（无需改代码）：新增**单位/技能/状态**只需在配置 JSON 追加并在 `active_skill_ids` / `effects[].status_id` 引用，`battlecfg.Validate` 把关。

需要改代码的扩展点：

- **新目标规则**：`battlecfg.TargetRule` + `engine.Battle.targets` switch + 校验。
- **新效果类型**：`battlecfg.EffectKind` + `engine.Battle.applyEffect` switch + 校验。
- **新状态类型**：`battlecfg.StatusKind` + `engine` 的 `execute/tickPeriodic`/`unit.modifier` 接入 + 校验。
- **新伤害结算环节**：往 `engine.NewDamagePipeline()` 追加 `DamageStage`（保持确定性：只消费 `RNG`，不依赖时间/并发）。

> ⚠️ 改任何结算逻辑都会改变事件流与 Checksum——**必须同步更新 engine 确定性锁表单测**，并让线上配置随代码一起版本化（同版本配置不可变，由 `battlecfg.MemoryConfigRepository` 强制）。
