# ctl_merge_game.go 合服流程文档分析

- 文件路径：`igame/internal/receiver/api/ctl_merge_game.go`
- 所属层：door侧（GMT/后台管理进程），HTTP handler + chanrpc回调 + 定时轮询三种入口并存
- 分析日期：2026-07-29

## 1. 模块定位

本文件是"合服任务"的door侧管理中心，职责：
1. 提供HTTP API给GMT后台，管理合服任务的生命周期（创建/发布/启动/查询/删除/发合服邮件）。
2. 通过`natsrpc`/`chanrpc`与game侧（play进程）交互，下发合服启动指令、接收合服进度回执。
3. 通过`MergeTaskTick`定时轮询，驱动一组"合服子任务"（活动屏蔽、邮件预告、KVK分组等）在合服前后按时间窗口自动执行。

实际的合服数据迁移逻辑（角色、联盟、排行榜等）不在本文件内，在`game/play/internal/ctl_merge.go`（play侧），本文件只负责**任务编排与调度**，不做业务数据合并。

## 2. 核心数据结构

### 2.1 MergeGameTaskInfo（合服任务，持久化于`unity.MergeGameTaskColName`）
```go
type MergeGameTaskInfo struct {
    Id              string                   // 任务id
    ServerId        []int32                  // 服务器列表，第一个元素固定为主服，其余为被合并子服
    ServerName      string                   // 合服后主服改名
    PublishAt       int64                    // 任务发布时间（GMT点击"发布"）
    BeginAt         int64                    // 任务启动时间（GMT点击"启动"，触发play侧真正合服）
    FinishAt        int64                    // 合服完成时间（play侧回执，0表示未完成）
    RuleFinishAt    int64                    // 规则检查完成时间
    DeleteAt        int64                    // 软删除时间
    BeginStatus     string                   // 启动状态（成功/错误信息）
    RuleStatus      string                   // 规则检查状态
    MergeStatus     string                   // 合服状态（play侧上报）
    MergeProcess    MergeProcess             // 合服执行进度明细
    MailAutoSend    int32                    // 是否自动发合服邮件
    SubtaskId       []int32                  // 该任务包含的子任务id列表
    MergeAt         int32                    // 预设合服日（秒级时间戳）
    SubtaskStatus   map[int32]*MergeSubTasks // serverId -> 该服要跑的子任务及其状态
    SubtaskFinishAt int64                    // 所有子任务全部完成的时间
    DoorId          int32                    // 归属的door进程id，避免多进程并发处理同一任务
    MergeActvTags   int32                    // 合服活动标签（合服次数：1=一次合服，其它=二次及以上）
}
```

### 2.2 MergeSubTaskInfo / MergeSubTasks（子任务）
```go
type MergeSubTaskInfo struct {
    TaskId   int32 // 子任务id
    BeginAt  int64 // 子任务开始处理时间
    FinishAt int64 // 子任务完成时间，>0表示完成
}
type MergeSubTasks struct {
    Tasks map[int32]*MergeSubTaskInfo // 子任务id -> 状态
}
```
`SubtaskStatus`是`map[serverId]*MergeSubTasks`，即**每个服务器独立维护自己的子任务完成状态**，同一个合服任务里主服和子服的子任务进度是分开跟踪的。

### 2.3 子任务类型注册表（`gdconf/def_merge.go`）
```go
const (
    MergeSubtask_MergeAdvanceMail       = 1 // 合服预告邮件
    MergeSubtask_ActvTypeCrossArenaKing = 2 // 战区争霸(105)活动关闭
    MergeSubtask_ActvTypeCrossInvade    = 3 // 跨服入侵(101/KVK)活动关闭
    MergeSubtask_KvkGroupAutoSet        = 4 // 合服后自动修改KVK匹配组id（新增）
)

var checkMergeSubtaskHandle = map[int32]func(task *MergeGameTaskInfo, serverId int32, subtask *MergeSubTaskInfo) bool{
    MergeSubtask_MergeAdvanceMail:       checkMergeSubtask_MergeAdvanceMailStatus,
    MergeSubtask_ActvTypeCrossArenaKing: checkMergeSubtask_ActvTypeCrossArenaKing,
    MergeSubtask_ActvTypeCrossInvade:    checkMergeSubtask_ActvTypeCrossInvade,
    MergeSubtask_KvkGroupAutoSet:        checkMergeSubtask_KvkGroupAutoSet,
}
```
所有子任务处理函数签名一致：`func(task, serverId, subtask) (finished bool)`，返回`true`代表该服的该子任务已完成。这是一个**策略表 + 定时轮询**的插件式设计，新增子任务只需实现同签名函数并注册，不用改动调度框架。

## 3. 完整生命周期（时间线）

```
① OnMergeGameTaskAddReq        创建任务（GMT）
        │  写入 GSMeta.merge_id / mergeActvTags / mergeTaskDayAt
        │  服务器必须存在且 merge_id 为空，否则整体失败（原子性由mongo条件更新保证）
        ▼
② OnMergeGameTaskPublishReq    发布任务（GMT），仅设置 publishAt
        │  发布后，MergeTaskTick开始轮询该任务的子任务（loadPendingMergeSubTasks的过滤条件之一）
        ▼
③ MergeTaskTick（后台定时循环，非HTTP）
        │  对每个"已发布/未删除/子任务未全部完成"的任务：
        │  遍历 SubtaskStatus[serverId] 下的每个子任务，调用对应 checkMergeSubtaskHandle
        │  这一步在①②之后到④之前会持续运行，负责：
        │    - 提前N天发合服预告邮件（子任务1）
        │    - 提前N天屏蔽105/101活动，合服完成后解除屏蔽（子任务2、3）
        │    - 合服完成后自动设置KvkGroup（子任务4，新增）
        ▼
④ OnMergeGameTaskRunReq        启动任务（GMT点击"启动"）
        │  前置检查：任务未删除、未启动、所有服务器 merge_id 匹配、且都在维护时间窗口内
        │  通过 handler.GetAckFromGs 向主服(ServerId[0])所在game进程下发命令："merge init <serverIds> <mailAutoSend>"
        │  game侧收到后开始真正的数据合并流程（play侧 ctl_merge.go doMerge...）
        │  写入 beginAt / beginStatus
        ▼
⑤ play侧合服执行（不在本文件，game/play/internal/ctl_merge.go）
        │  按步骤机(MergeStep_*)逐步合并角色/联盟/排行榜/邮件等
        ▼
⑥ OnG2DMergeGameProcessUpdateNtf  play侧进度回执（chanrpc回调，非HTTP）
        │  每次进度变化都会回调，更新 updateAt/finishAt/rule_status/merge_status/merge_process/mailSendAt
        │  当 req.MergeFinishAt > 0 时，额外调用 mergeGameTaskMarkStatus 做收尾：
        │    - 主服(ServerId[0])：改名为task.ServerName，清空merge_id
        │    - 其余子服：写入 merge_time = now（标记"已被合并"）
        ▼
⑦ 后续 MergeTaskTick 继续轮询
        │  子任务2/3（活动屏蔽）等到 task.FinishAt>0 后的特定时间点（次日0点/下周日）解除屏蔽
        │  子任务4（KvkGroup自动设置）在 task.FinishAt>0 时立即一次性写入分组值
        ▼
⑧ OnMergeGameTaskSendMail       (可选) GMT手动触发向已完成任务的服务器发合服邮件
```

## 4. 关键分支/边界逻辑

### 4.1 任务创建的原子性保护（`OnMergeGameTaskAddReq`）
- 用mongo条件更新一次性锁定所有服务器的`merge_id`字段（要求为空才能设置），`MatchedCount==0 || ModifiedCount==0`时视为失败——避免同一批服务器被并发加入两个合服任务。
- 任务文档保存失败时会执行`rollbackServers`回滚已写入的`merge_id`/`mergeTaskDayAt`，防止"服务器被锁但任务不存在"的悬挂状态。
- **本次新增行为**：不管GMT传入的`SubTaskId`里是否包含KVK分组子任务，后端都会强制追加`MergeSubtask_KvkGroupAutoSet`（去重判断），确保GMT无需感知该子任务即可默认生效。

### 4.2 `mainServerId := task.ServerId[0]` 是隐含契约
整个文件里"谁是主服"完全依赖`ServerId`切片的第0个元素，没有独立字段显式标注。`checkMergeSubtask_KvkGroupAutoSet`、`mergeGameTaskMarkStatus`（改名逻辑）、`OnMergeGameTaskRunReq`（下发合服指令的目标进程）都依赖这个约定。改动/扩展这块逻辑时必须保持该顺序契约不被破坏。

### 4.3 子任务的"两阶段"模式（活动屏蔽类）
`checkMergeSubtask_ActvTypeCrossArenaKing`/`checkMergeSubtask_ActvTypeCrossInvade`都是两阶段状态机：
- 阶段1（`subtask.BeginAt == 0`）：判断合服时间是否落在活动开放区间内，若是则添加屏蔽（`act_shield`追加），并置`beginAt`。
- 阶段2（`subtask.BeginAt > 0`，等待`task.FinishAt > 0`）：合服彻底完成后，到达"次日0点"或"下周日"这类活动周期边界时，移除屏蔽并置`finishAt`。

**新增的`checkMergeSubtask_KvkGroupAutoSet`是单阶段模式**：只在`task.FinishAt > 0`时触发，一次性写入目标`KvkGroup`值并直接置`finishAt`，不需要维护中间的"合服进行中屏蔽"状态，因为分组id修改不需要像活动开关一样考虑"合服过程中是否处于活动开放期"的问题。

### 4.4 `MergeTaskTick`的完成判定
```go
finishNum := 0
for subtaskId, subtaskInfo := range subtasks.Tasks {
    if fn(task, serverId, subtaskInfo) { finishNum++ }
}
if finishNum == len(subtasks.Tasks) {
    // 标记该服 subtaskFinishAt
}
```
注意这里是**按serverId粒度**判断"这个服的所有子任务是否全部完成"，而`loadPendingMergeSubTasks`的过滤条件`subtaskFinishAt: 0`是**任务级**字段——但代码里`subtaskFinishAt`只在循环内被无条件`updateMergeTask`覆盖写入，最终值以最后一次遍历到的serverId结果为准。多服场景下如果不同服务器完成时间不同，`subtaskFinishAt`会被多次覆盖直到所有服都完成的那一轮才能稳定为非0。这是现有实现的既有行为，非本次改动引入。

### 4.5 KvkGroup自动设置的分组规则（本次新增）
```go
if serverId == mainServerId {
    kvkGroup = (task.MergeActvTags == 1) ? 2 : 3   // 主服：一次合服=2，二次及以上=3
} else {
    kvkGroup = 0                                     // 子服：统一置0，不参与KVK匹配
}
```
`MergeActvTags`来自GMT创建任务时传入，是"合服次数"的既有标签字段（同时被`gdconf.GetMergeArgs`用于区分一次/二次合服的配置读取），本次复用而非新增。

## 5. 与其他模块的接口边界

| 交互对象 | 方式 | 用途 |
|---|---|---|
| GMT前端 | HTTP (gin) | 任务增删查、发布、启动、发邮件 |
| game侧(play进程) | `handler.GetAckFromGs` (同步命令) | 下发"merge init"启动真正合服 |
| game侧(play进程) | `chanrpc` (`OnG2DMergeGameProcessUpdateNtf`) | 接收合服进度/完成回执 |
| `utility.GSMetas`(door DB) | `db.SaveCall`/`GetUpdateOp` | 读写每个服务器的元数据（merge_id/mergeActvTags/act_shield/kvk_group等） |
| `gdconf` | 配置读取 | 合服提前天数、KVK/105活动周期常量、合服邮件配置 |
| `MergeTaskTick` | 定时器驱动（调用方在本文件外，通常是door主循环tick） | 驱动所有子任务状态机前进 |

## 6. 本次改动（KVK合服后自动分组）落点总结

| 改动文件 | 改动内容 |
|---|---|
| `gdconf/def_merge.go` | 新增常量`MergeSubtask_KvkGroupAutoSet = 4` |
| `ctl_merge_game.go OnMergeGameTaskAddReq` | 创建任务时强制追加子任务4到`req.SubTaskId`，GMT无需感知 |
| `ctl_merge_game.go` | 新增`checkMergeSubtask_KvkGroupAutoSet`函数 + 注册进`checkMergeSubtaskHandle` |

未改动：`OnSetServer`（devops人工改`kvk_group`的入口）、KVK匹配逻辑本身（`match_mgr.go`/`ctl_match_king_war.go`）——两者均按既有方式读取`KvkGroup`字段，感知不到值的来源是自动还是手动。

## 7. 潜在风险点（供代码评审参考）

1. **`MergeActvTags`语义耦合**：`gdconf.GetMergeArgs`用`==1`区分一次合服，本次KvkGroup规则沿用同一判定，若未来"合服次数"语义变更（如改为从1开始计数二次合服需要传2而不是"非1即算二次"），两处需要同步修改，建议后续如有变更统一收口成一个函数而非各处直接比较`==1`。
2. **`checkMergeSubtask_KvkGroupAutoSet`无重试上限**：与其他子任务一致，`db.SaveCall`失败时返回`false`，等待下一次`MergeTaskTick`重试，没有失败次数上限或告警，长期失败会静默阻塞`subtaskFinishAt`的达成（继承自现有子任务设计，非本次引入的新问题）。
3. **子服`KvkGroup`置0与`MergeTime`标记的关系**：`mergeGameTaskMarkStatus`在`req.MergeFinishAt > 0`时就会给子服写`merge_time`，比子任务4的`KvkGroup=0`写入更早发生（前者在`OnG2DMergeGameProcessUpdateNtf`同步执行，后者要等下一次`MergeTaskTick`轮询）。两者最终一致，但存在短暂的时间窗口内`merge_time>0`而`KvkGroup`还未归零，若匹配定时任务恰好在这个窗口内跑，仍会被`v.MergeTime > 0`的判断挡住（`crossInvadeMatchServerInfo`已有此check），不会造成错误匹配，但需要在测试时注意这个时序差不是bug。
