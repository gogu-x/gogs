# KVK合服后自动分组 - 实现文档

- 关联策划案：`/data/h5/H5策划案/v1049/KVK合服后自动分组.xlsx`
- 关联模块：合服任务系统（door）/ 跨服入侵匹配（KVK）
- 编写日期：2026-07-29

## 1. 需求还原

### 1.1 背景问题
1. 已存在二次合服的服务器，一次合服和二次合服的KVK匹配组混在一起，无法区分。
2. 合服后跨服入侵（KVK）匹配组id需要人工手动改，效率低，容易改错。

### 1.2 方案
- 新增KVK匹配组id=3，代表"二次合服后的主服"。原id=2代表"一次合服后的主服"不变。
- 完整枚举：
  - 0 = 不参与匹配（通常为合服时被合掉的子服）
  - 1 = 未合服的服务器匹配组
  - 2 = 一次合服后的主服匹配组
  - 3 = 二次合服后的主服匹配组
- 合服完成后自动修改参与合服的各服务器KvkGroup：
  - 被合掉的子服 → 0
  - 一次合服后剩下的主服 → 2
  - 二次合服后剩下的主服 → 3
- 保留人工修改入口，自动修改后允许负责人再手动改。

## 2. 现状代码调研结论

### 2.1 核心字段：`KvkGroup`
落在 `utility.GSMeta`（`utility/gsmeta.go:104`）：
```go
KvkGroup int32 `json:"kvk_group" bson:"kvk_group"` // 跨服组id
```
该字段被以下几处消费：
- `igame/internal/receiver/ctl_match_king_war.go:107 crossInvadeMatchServerInfo()`：遍历所有服，`KvkGroup<=0`直接跳过，其余的服送入匹配。
- `igame/internal/receiver/match/kingwar/match_mgr.go`：`MatchServerInfo.KvkGroup`参与实际分组，`StartMatch()`里 `key = fmt.Sprintf("%v_%v", Group, KvkGroup)` 作为分组key，同一个key的服互相匹配。
- `igame/internal/receiver/handler/ctl_server.go OnSetServer`：devops后台人工改`kvk_group`的唯一入口（`req.KvkGroup *int32`），**本次需求不需要改这里，天然满足"保留手动修改"**。

### 2.2 一次/二次合服判定：复用现有字段 `MergeActvTags`
`utility.GSMeta.MergeActvTags`（"合服活动标签(合服次数)"）在创建合服任务时写入：
- `igame/internal/receiver/api/ctl_merge_game.go MergeGameTaskAddReq.MergeActvTags` → 任务创建时由GMT传入，写进`GSMeta.MergeActvTags`和`MergeGameTaskInfo.MergeActvTags`。
- `gdconf/def_merge.go GetMergeArgs()`已用该字段区分：`MergeActvTags==1`用一次合服的配置，其他值用二次合服(`_02`后缀)配置。

**结论：`MergeActvTags`已经是"这是第几次合服"的权威字段，本次需求直接复用它做判定，不需要新增字段来标记合服次数。**

判定规则（与 `GetMergeArgs` 保持一致的语义）：
- `task.MergeActvTags == 1` → 主服设为 KvkGroup = 2
- `task.MergeActvTags != 1`（即 >=2）→ 主服设为 KvkGroup = 3

### 2.3 合服任务与子任务机制（本次要挂载的框架）
合服任务主结构 `MergeGameTaskInfo`（`ctl_merge_game.go:60`），关键字段：
- `ServerId []int32`：**第一个元素为主服**，其余为被合并的子服。
- `MergeActvTags int32`：合服次数标签。
- `FinishAt int64`：合服完成时间，0表示未完成。
- `SubtaskStatus map[int32]*MergeSubTasks`：serverId → 该服要跑的子任务集合。
- `SubtaskId []int32`：任务包含的子任务id列表（创建任务时指定）。

子任务执行框架（已有，本次复用）：
```go
var checkMergeSubtaskHandle = make(map[int32]func(task *MergeGameTaskInfo, serverId int32, subtask *MergeSubTaskInfo) bool)

func init() {
    checkMergeSubtaskHandle[gdconf.MergeSubtask_MergeAdvanceMail] = checkMergeSubtask_MergeAdvanceMailStatus
    checkMergeSubtaskHandle[gdconf.MergeSubtask_ActvTypeCrossArenaKing] = checkMergeSubtask_ActvTypeCrossArenaKing
    checkMergeSubtaskHandle[gdconf.MergeSubtask_ActvTypeCrossInvade] = checkMergeSubtask_ActvTypeCrossInvade
}

func MergeTaskTick() {
    for _, task := range loadPendingMergeSubTasks() { // 未完成子任务的合服任务
        for serverId, subtasks := range task.SubtaskStatus {
            for subtaskId, subtaskInfo := range subtasks.Tasks {
                fn, ok := checkMergeSubtaskHandle[subtaskId]
                if ok && fn(task, serverId, subtaskInfo) {
                    // 标记该子任务finishAt
                }
            }
        }
    }
}
```
`MergeTaskTick` 由定时器循环调用（轮询），已有的两个KVK相关子任务（`MergeSubtask_ActvTypeCrossArenaKing`/`MergeSubtask_ActvTypeCrossInvade`）目前只做"提前N天屏蔽101/105活动，合服完成后解除屏蔽"，**没有触碰KvkGroup**，这是本次需求要新增的空白点。

### 2.4 已有但语义不同的"跳过子服"逻辑
`crossInvadeMatchServerInfo()`目前用 `v.MergeTime > 0` 跳过被合并子服（而非`KvkGroup==0`）。两者是平行机制：
- `MergeTime`：子服被合并时间戳，>0表示已被合并。
- `KvkGroup`：分组归属，0表示不参与。
本次改动让`KvkGroup`显式归零，与`MergeTime`判断互相印证，不冲突，不需要改动`MergeTime`相关逻辑。

## 3. 实现方案

### 3.1 新增合服子任务类型
文件：`gdconf/def_merge.go`
```go
const (
    MergeSubtask_MergeAdvanceMail       = 1 // 合服预告邮件
    MergeSubtask_ActvTypeCrossArenaKing = 2 // 战区争霸活动关闭
    MergeSubtask_ActvTypeCrossInvade    = 3 // 101跨服战活动关闭
    MergeSubtask_KvkGroupAutoSet        = 4 // 合服后自动修改KVK匹配组id（新增）
)
```

### 3.2 新增子任务处理函数
文件：`igame/internal/receiver/api/ctl_merge_game.go`

```go
// checkMergeSubtask_KvkGroupAutoSet 合服完成后自动修改KVK匹配组id
// 规则：
//   被合并的子服（非主服） -> KvkGroup = 0
//   合服后剩下的主服：
//     task.MergeActvTags == 1（一次合服） -> KvkGroup = 2
//     task.MergeActvTags != 1（二次及以上合服） -> KvkGroup = 3
func checkMergeSubtask_KvkGroupAutoSet(task *MergeGameTaskInfo, serverId int32, subtask *MergeSubTaskInfo) (finished bool) {
    if subtask.FinishAt > 0 {
        return true
    }

    // 合服未完成，不处理
    if task.FinishAt == 0 {
        return false
    }

    if len(task.ServerId) == 0 {
        return false
    }
    mainServerId := task.ServerId[0]

    var kvkGroup int32
    if serverId == mainServerId {
        if task.MergeActvTags == 1 {
            kvkGroup = 2 // 一次合服主服
        } else {
            kvkGroup = 3 // 二次合服主服
        }
    } else {
        kvkGroup = 0 // 被合并子服
    }

    updateGSInfo := bson.D{{"$set", bson.M{fmt.Sprintf("infos.%v.kvk_group", serverId): kvkGroup}}}
    op := utility.GSMetas{}.GetUpdateOp(doorConf.DBName(), updateGSInfo)
    op.Selector = bson.M{fmt.Sprintf("infos.%v.serverid", serverId): serverId}
    if res := db.SaveCall(doorConf.DBMOD, op); res.Err != nil {
        tlog.Errorf("checkMergeSubtask_KvkGroupAutoSet serverId:%v kvkGroup:%v err:%v", serverId, kvkGroup, res.Err)
        return false
    }

    nowTs := util.NowTs()
    updateTask := bson.D{{"$set", bson.M{
        fmt.Sprintf("subtaskStatus.%v.tasks.%v.beginAt", serverId, subtask.TaskId):  nowTs,
        fmt.Sprintf("subtaskStatus.%v.tasks.%v.finishAt", serverId, subtask.TaskId): nowTs,
    }}}
    updateMergeTask(task.Id, updateTask)

    tlog.Infof("checkMergeSubtask_KvkGroupAutoSet mergeId:%v serverId:%v kvkGroup:%v set done", task.Id, serverId, kvkGroup)
    return true
}
```

触发时机：`task.FinishAt > 0`（合服彻底完成，即`OnG2DMergeGameProcessUpdateNtf`收到`req.MergeFinishAt > 0`那一刻，与`mergeGameTaskMarkStatus`同一时间点），一次性写入目标值，不需要提前N天检测窗口（不同于101/105活动屏蔽子任务需要在合服前后维护一段"屏蔽区间"，KvkGroup只需在合服彻底完成后改一次即可）。

### 3.3 注册子任务
文件：`igame/internal/receiver/api/ctl_merge_game.go`，`init()`中追加一行：
```go
func init() {
    checkMergeSubtaskHandle[gdconf.MergeSubtask_MergeAdvanceMail] = checkMergeSubtask_MergeAdvanceMailStatus
    checkMergeSubtaskHandle[gdconf.MergeSubtask_ActvTypeCrossArenaKing] = checkMergeSubtask_ActvTypeCrossArenaKing
    checkMergeSubtaskHandle[gdconf.MergeSubtask_ActvTypeCrossInvade] = checkMergeSubtask_ActvTypeCrossInvade
    checkMergeSubtaskHandle[gdconf.MergeSubtask_KvkGroupAutoSet] = checkMergeSubtask_KvkGroupAutoSet // 新增
}
```

### 3.4 GMT创建合服任务时默认携带新子任务
创建任务的入口在`OnMergeGameTaskAddReq`（`ctl_merge_game.go:142`），子任务列表由前端传入的`req.SubTaskId`决定。有两种落地方式，需与前端/策划确认：

- 方案A（推荐）：后端在`OnMergeGameTaskAddReq`中固定追加`MergeSubtask_KvkGroupAutoSet`到`req.SubTaskId`，不依赖GMT页面勾选，保证"自动修改"是强制发生的默认行为，符合策划案"自动化"的诉求，同时不影响"保留手动修改"（`OnSetServer`人工改`kvk_group`不受影响，且允许在自动设置之后再次手动覆盖）。
- 方案B：GMT前端页面新增一个可勾选的子任务项，运营人员需要手动选中才生效——不符合策划"自动化，减少人工"的初衷，不建议。

本文档采用方案A。具体改动位置：`OnMergeGameTaskAddReq`函数体，在构造`mergeSubtasks`前，确保`req.SubTaskId`包含`gdconf.MergeSubtask_KvkGroupAutoSet`（去重追加）。

### 3.5 人工修改入口（保持不变）
`igame/internal/receiver/handler/ctl_server.go OnSetServer`中已支持：
```go
if req.KvkGroup != nil {
    update = append(update, bson.E{fmt.Sprintf("infos.%v.kvk_group", req.ServerId), *req.KvkGroup})
}
```
自动修改完成后，该入口依然可用，满足"2.2.b 保留手动修改"，本次无需改动。

## 4. 数据流示意

```
GMT创建合服任务(OnMergeGameTaskAddReq)
    ServerId=[主服, 子服1, 子服2, ...]
    MergeActvTags=1或2+  (合服次数)
    SubTaskId 追加 MergeSubtask_KvkGroupAutoSet
        ↓
合服任务发布+启动 (OnMergeGameTaskPublishReq / OnMergeGameTaskRunReq)
        ↓
play侧执行实际合服流程 (game/play/internal/ctl_merge.go doMerge...)
        ↓
合服全部完成 → G2DMergeGameProcessUpdateNtf(MergeFinishAt>0)
        ↓
door侧 task.FinishAt 被置位 (mergeGameTaskMarkStatus)
        ↓
MergeTaskTick 轮询 → checkMergeSubtask_KvkGroupAutoSet 命中 task.FinishAt>0
        ↓
    主服(ServerId[0])：MergeActvTags==1 ? KvkGroup=2 : KvkGroup=3
    其余子服：KvkGroup=0
        ↓
写入 utility.GSMeta.KvkGroup (door DB)
        ↓
下次 crossInvadeMatchServerInfo() 匹配时读取到新KvkGroup，自动生效
```

## 5. 待确认问题（需策划/前端确认后再定稿）

1. **子任务默认下发方式**：方案A（后端强制追加，GMT不需要勾选）还是方案B（GMT页面新增勾选项）？本文档默认按方案A实现，如需方案B需前端配合改动合服任务创建页面。
2. **三次及以上合服**：文档只定义到KvkGroup=3（二次合服），若未来出现三次合服，是否统一归为3，还是需要继续新增4、5...？当前实现按`MergeActvTags != 1`统一归为3处理，等价于"只要不是首次合服都算3"，需要策划确认这个兼容规则是否符合预期。
3. **子任务列表是否需要在合服任务详情页（`OnMergeGameTaskInfoReq`）里展示KvkGroup自动修改的结果**，便于运营复核。当前方案只落库，不额外加展示逻辑，如需要需前端配合。

## 6. 影响范围/回归点

- 新增：`gdconf/def_merge.go`（常量）、`ctl_merge_game.go`（新函数+init注册+AddReq追加子任务id）。
- 不改动：`OnSetServer`人工改分组入口、`crossInvadeMatchServerInfo`/`match_mgr.go`匹配逻辑本身（继续读`KvkGroup`，无需感知新增的"3"这个值的来源）。
- 回归测试点：
  - 一次合服任务完成后，主服KvkGroup=2，子服KvkGroup=0。
  - 二次合服任务完成后，主服KvkGroup=3，子服KvkGroup=0。
  - 合服未完成时（`task.FinishAt==0`）不触发修改。
  - 自动修改后，GMT人工再次修改`kvk_group`仍然生效（不被覆盖回退）。
  - 修改后下一次KVK匹配（`crossInvadeMatchServerInfo`/`StartMatch`）能按新分组正确分组，主服进入正确的匹配池，被合子服不再进入匹配。
