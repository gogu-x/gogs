# Cross 进程概览

## 进程定位

Cross 是跨服地图节点，专门承载需要多个 game 服玩家共同参与的跨服地图实例。
每个 cross 进程可以同时托管多种类型的跨服地图副本。

---

## 模块组成（main.go 注册顺序）

```
db          → MongoDB 主库连接（conf.DBMOD）
db_slave    → MongoDB 从库连接（conf.DBSLAVEMOD）
natsrpc     → 消息总线，节点类型 = Cross
pf          → 平台模块
rankmgr     → Redis 排行榜
play        → cross 侧 play 模块（玩家缓存/联盟缓存/跨服传送）
serverstat  → 服务器状态心跳模块
duplicatemgr → 跨服副本管理器（GVG、远征、ZVZ 等所有地图实例）
```

---

## 各模块详解

### 1. play 模块（`cross/play/`）

**职责**：cross 进程中的玩家管理层，不处理地图玩法，只负责：

| 功能 | 消息 |
|------|------|
| 接收玩家 cache（含阵营 Horde） | `SyncPlayerCacheReq` → 写入本进程内存 `cache.PCD` |
| 接收联盟 cache | `SyncUnionCacheReq` → 写入 `cache.U` |
| 跨服传送检查 | `CrossChangeMapCheckReq` / `CrossChangeMapReq`（直接透传 Coord，校验在地图层） |
| GM 时间控制 | `GMSetServerTime` / `GMGetServerTime` |
| 广播 CrossEvent 到指定地图模块 | `CastCrossEvent(modName, key, ...)` |

源码：
- `cross/play/internal/handler.go` — 消息处理
- `cross/play/internal/module.go` — 模块主体
- `cross/play/internal/serverinfo.go` — 服务器信息存储

---

### 2. serverstat 模块（`cross/serverstat/`）

**职责**：周期性（每秒）向 global 上报本 cross 进程的运行状态，供 global 在分配新副本时做**负载均衡**决策。

上报内容（`ServerStateSyncNtf`）：
```
ServerId, ServerType=Cross, PlayerCount, MapCount, MapTypeCount, Cpu, Mem, Timestamp
```

接收消息：
- `ServerMapCountUpdataNtf` — duplicatemgr 变更地图数量时触发
- `ServerPlayerCountUpdataNtf` — 玩家数量变化时触发

源码：`cross/serverstat/serverstat.go`

---

### 3. duplicatemgr 模块（`game/wmap/duplicatemgr/`，nodeType=Cross）

**职责**：管理本 cross 进程上所有地图实例的生命周期。

| 功能 | 说明 |
|------|------|
| 创建副本 | 收到 `BatchCreateDuplicateMapReq`（来自 global），按 `MapType` 创建对应地图实例 |
| 加载副本 | 服务器重启时从 MongoDB 恢复所有地图实例 |
| 删除副本 | 副本超时/结束时销毁并从 MongoDB 删除 |
| 维护实例路由 | 所有 `cross_<instId>_<maptype>_*` 格式的 collection 都归属到具体实例 |

支持的地图类型：
- `MAP_EXPEDITION`（远征）
- `MAP_GVG`（联盟战争）
- `MAP_ZVZ`（区对区）
- 其他副本类型

源码：`game/wmap/duplicatemgr/`

---

### 4. 地图实例模块（WMap，动态创建）

每个副本实例是一个独立的 Go Module，注册到 `core.DefaultApp()`，有自己的 goroutine 和消息循环。

#### 远征地图（BattlefieldMap）

源码：`game/wmap/dupmaps/battlefield/`

| 文件 | 职责 |
|------|------|
| `map.go` | 地图结构定义、创建（`NewBattlefieldMap`）、Save/Load |
| `event.go` | 地图就绪事件（`OnMapReadyEvent` → `BootStartupBattlefield`） |
| `handler.go` | 消息路由注册（`OnLoadMapNtf`） |
| `timer.go` | 定时器注册 |

玩法逻辑（`game/wmap/internal/`）：

| 文件 | 职责 |
|------|------|
| `ctl_battlefield.go` | 远征地图启动（`BootStartupBattlefield`）、阶段定时器（`TimerTypeBattlefieldPhaseCheck`）、关卡开放（`openBarrier`） |
| `ctl_battlefield_melting_ice.go` | 融冰玩法 |
| `ctl_ice_contest.go` | 冰髓争夺（`IceContestPhaseChange`） |
| `ctl_island.go` | 岛屿系统 |
| `ctl_union_center.go` | 联盟中心建筑 |
| `ctl_ice_morrow.go` | 冰髓矿 |
| `ctl_area.go` | 领地争夺 |
| `ctl_king_war.go` | 王座战争（远征内） |
| `ctl_relic.go` | 遗迹 |
| `ctl_forgotten_land.go` | 遗忘之地 |
| `ctl_calling_altar.go` | 祭坛 |

地图全局状态管理（`game/wmap/internal/expedition/`）：

| 文件 | 职责 |
|------|------|
| `mgr.go` | `Mgr` 结构（阵营/领地/行军/预警等），`InitAndLoad`，`LoadAfterInit` |
| `loader.go` | MongoDB 加载/保存，`GenUpdate`（脏标记增量 `$set`） |
| `ice_contest_mgr.go` | 冰髓争夺单独存储 |

#### GVG 地图（GVGMap）

源码：`game/wmap/dupmaps/gvgmap/`

#### ZVZ 地图（ZVZMap）

源码：`game/wmap/dupmaps/zvzmap/`

---

## 数据库

### MongoDB 库名
```
gs_cross_<ServerID>
（有 Namespace 时：<namespace>_gs_cross_<ServerID>）
```

### Collection 命名规则

- **世界地图数据**：直接用 `collName`（如 `player_city`）
- **副本数据**：自动拼接 `cross_<instId>_<mapType>_<collName>`

例：远征地图 instId=12345 的主 collection：
```
cross_12345_expedition_battlefieldmgr
cross_12345_expedition_icecontestmgr
cross_12345_expedition_player_city
cross_12345_expedition_player_troop
cross_12345_expedition_map_player
...
```

### 持久化的数据

| Collection 模式 | 内容 |
|----------------|------|
| `cross_<instId>_expedition_battlefieldmgr` | 远征全局状态（阵营/领地/行军/预警/日报/赐福等），`_id=crossServerId` |
| `cross_<instId>_expedition_icecontestmgr` | 冰髓争夺数据 |
| `cross_<instId>_expedition_player_city` | 玩家城池在地图上的坐标/状态 |
| `cross_<instId>_expedition_player_troop` | 玩家行军数据 |
| `cross_<instId>_expedition_map_player` | MapPlayer（召唤怪/采集记录等） |
| `cross_<instId>_expedition_*` | 领地建筑、任务建筑、采集物、遗迹、冰霜矿、融冰等 |
| `duplicate_map`（通用） | 地图实例元信息（instId/keepTime/ExtraInfo），重启恢复用 |

### 内存数据（不落地）

| 数据 | 位置 |
|------|------|
| `PlayerCache`（含 Horde 阵营） | `cache.PCD`（cross play 模块内存） |
| `UnionCache` | `cache.U`（cross play 模块内存） |

---

## 与其他节点的通信

| 方向 | 消息 | 说明 |
|------|------|------|
| global → cross | `BatchCreateDuplicateMapReq` | 创建副本实例 |
| game → cross play | `SyncPlayerCacheReq` | 推送玩家 cache（进图前） |
| game → cross play | `CrossChangeMapCheckReq/Req` | 跨服传送握手 |
| game → cross wmap | `SyncMapPlayerReq` | 推送 MapPlayer 数据（进图时） |
| game → cross wmap | `LogoutReq` | 玩家离开老地图通知 |
| game → cross wmap | `SetPlayerIDMapReq` | 告知地图玩家来源 serverID |
| cross wmap → game play | `NewMapCityAck`（回调链） | 建城完成通知 |
| cross serverstat → global | `ServerStateSyncNtf` | 心跳/负载上报（每秒） |

---

## 关键源码位置汇总

| 功能 | 文件路径 |
|------|---------|
| cross 进程入口 | `cross/main.go` |
| cross play 模块 | `cross/play/internal/` |
| 服务器状态心跳 | `cross/serverstat/serverstat.go` |
| 副本管理器 | `game/wmap/duplicatemgr/` |
| 远征地图结构 | `game/wmap/dupmaps/battlefield/` |
| 远征地图玩法（wmap层） | `game/wmap/internal/ctl_battlefield.go` |
| 远征全局状态 | `game/wmap/internal/expedition/` |
| GVG 地图 | `game/wmap/dupmaps/gvgmap/` |
| ZVZ 地图 | `game/wmap/dupmaps/zvzmap/` |
| wmap save 注册 | `game/wmap/internal/saver.go` |
| collection 命名规则 | `game/wmap/internal/saver.go:formatCollectionName` |
| 玩家 cache 推送 | `game/play/internal/mplayer/player_cache.go` |
