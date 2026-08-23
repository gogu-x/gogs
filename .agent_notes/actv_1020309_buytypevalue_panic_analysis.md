# 活动 1020309 报错分析：`cfg.BuyTypeValue[0]` 数组越界 panic

## 1. 问题现象

- 报错位置：`actv/cactv/internal/activity/template/base/base.go:178`
- 报错方法：`ActvTmplBase.CheckOrderCreateCondtions`
- 触发代码：

```go
// 必须适合对应活动关联的礼包才能买
if c.CfgID != cfg.BuyTypeValue[0] {
    return cspb.ErrCodeOrderActvTmplCostumePassGift1, 0, 0
}
```

- 崩溃原因：`cfg.BuyTypeValue` 是空 slice（长度为 0），对其取下标 `[0]` 触发 `index out of range` panic。
- 影响活动：`1020309`，其关联的礼包（`D2GiftCfg`）配置中 `buy_type_value` 字段为空。

## 2. 根因定位

### 2.1 配置结构

`gdconf/conf_d2gift.go` 中 `D2GiftCfg.BuyTypeValue` 对应策划配置表字段 `buy_type_value`：

```go
BuyTypeValue []int32 `bson:"buy_type_value"` // 活动限购
```

`BuyType` 字段（`buy_type`）标识礼包的限购类型，枚举定义在 `gdconf/conf_d2gift_ext.go`：

```go
const (
    GiftLimitInterval     = iota // 0 默认
    GiftLimitForever             // 1 次数限购
    GiftTypeGiftTimeLimit        // 2 倒计时礼包
    GiftTypeTime                 // 3 客户端倒计时占位
    GiftLimitActv                // 4 活动限制的礼包
)
```

当 `BuyType == GiftLimitActv`（活动限购礼包）时，约定 `BuyTypeValue[0]` 存放该礼包**关联的活动配置 ID**（`ShowLimit`/`BuyTypeValue[2]` 等还有额外的时间窗口配置含义），代码里多处直接假设该 slice 非空。

### 2.2 已存在但不完整的防御

`gdconf/conf_d2gift_ext.go` 的 `afterLoadD2GiftCfg`（配置表加载后的处理钩子）中，**已经**对这个问题做了防御：

```go
if cfg.BuyType == GiftLimitActv {
    if len(cfg.BuyTypeValue) == 0 {
        tlog.Errorf("actv gift buy type value is empty. id:%d", cfg.CfgId)
        continue // 跳过，不加入 ActvGifts 映射
    }
    actvCfgID := cfg.BuyTypeValue[0]
    ...
}
```

但这个防御只影响 `ActvGifts`（活动ID -> 礼包ID列表）这张派生索引表的构建，**不会阻止**其他代码路径直接通过 `GetD2GiftCfg(giftId)` 拿到原始的 `*D2GiftCfg` 对象后再次访问 `BuyTypeValue[0]`。也就是说：错误配置的礼包在加载阶段已经被记了一条 `tlog.Errorf` 日志，但配置对象本身仍然被正常缓存（`csvD2GiftCfg`），后续任何直接用 giftId 查询、且不做长度校验的代码都会重复暴露同样的问题。

### 2.3 真正 panic 的代码路径

`CheckOrderCreateCondtions`（两处几乎相同的实现，分别在旧版 `game/actv` 和新版 `actv/cactv` 模板基类中）：

```go
func (c *ActvTmplBase) CheckOrderCreateCondtions(playerID int64, giftId int32) (errCode cspb.ErrCode, giftStart int64, giftEnd int64) {
    defer func() {
        if err := recover(); err != nil {
            tlog.Errorf("actv:%v actvCfg:%v CheckOrderCreateCondtions err:%v", c.ActvID, c.CfgID, err)
            errCode = cspb.ErrCodeOrderActvTmplCostumePassGift1
        }
    }()
    cfg := gdconf.GetD2GiftCfg(giftId)

    if cfg.BuyType != gdconf.GiftLimitActv {
        return cspb.ErrCodeOrderActvTmplCostumePassGift1, 0, 0
    }
    if !c.IsStart() {
        return cspb.ErrCodeOrderActvTmplCostumePassGift1, 0, 0
    }
    // panic 点：未检查 len(cfg.BuyTypeValue) 就直接取下标 0
    if c.CfgID != cfg.BuyTypeValue[0] {
        return cspb.ErrCodeOrderActvTmplCostumePassGift1, 0, 0
    }

    actvCfg := gdconf.GetActvOnlineCfgExt(c.CfgID)
    _, _, endTs, _ := actvCfg.GetTime(c.StartTs)
    // 若走到这里，还会访问 BuyTypeValue[2]，同样有越界风险
    if cfg.BuyTypeValue[2] == 0 {
        return cspb.ErrCodeSuccess, c.StartTs, endTs
    }
    ...
}
```

由于该礼包 `BuyType == GiftLimitActv` 且活动处于开启状态，代码会走到 `cfg.BuyTypeValue[0]` 这一行，`BuyTypeValue` 长度为 0，触发 panic。函数入口的 `defer recover()` 兜底捕获了 panic，返回 `ErrCodeOrderActvTmplCostumePassGift1`，避免了活动 goroutine 崩溃，但每次调用都会打一条 `tlog.Errorf`。

### 2.4 为什么错误量大

`CheckOrderCreateCondtions` 不止在真正下单时被调用一次，还被高频的批量查询接口调用：

- `OnP2AOrderCreateCheckReq`：玩家发起下单校验时调用一次。
- `OnP2AActvGiftCheckReq`：**批量**接口，对玩家所有进行中的活动、请求里的每个 `giftID` 都会调用一次 `CheckOrderCreateCondtions`（`actv_msg.go` 中的双重循环）。只要请求里携带了这个问题礼包 ID，且对应活动在开启状态，每次请求都会命中一次 panic-recover + Errorf 日志。

这个批量检查接口通常是客户端在打开活动/礼包相关界面时调用，调用频率和并发玩家数成正比，因此错误日志量会随着活动 1020309 的在线玩家数被迅速放大。

## 3. 影响范围

- 已确认新旧两套活动框架都存在同样代码模式：
  - `actv/cactv/internal/activity/template/base/base.go`（新）
  - `game/actv/internal/activity/template/base/base.go`（旧）
- 两处均有 `defer recover()` 兜底，**不会导致进程崩溃或阻塞其他活动**，影响被限定在：
  1. 该问题礼包无法被正常购买（下单校验永远返回 `ErrCodeOrderActvTmplCostumePassGift1`）；
  2. 产生大量 `CheckOrderCreateCondtions err:...` 错误日志，干扰日志监控、增加日志存储/检索成本，可能掩盖其他真实问题。

## 4. 修复建议

### 4.1 治本：修正活动 1020309 关联礼包的配置数据
在配置表中为该礼包补齐 `buy_type_value`（活动限购礼包必须至少配置 `[关联活动CfgID]`，若使用了限购时间窗口逻辑还需配置到下标 2）。这是最直接、影响面最小的修复方式，不需要发版。

### 4.2 治标：代码层加防御，避免同类配置问题继续产生 panic
在 `CheckOrderCreateCondtions` 中访问 `BuyTypeValue` 前增加长度校验，两处下标（`[0]` 和 `[2]`）都需要处理：

```go
cfg := gdconf.GetD2GiftCfg(giftId)

if cfg.BuyType != gdconf.GiftLimitActv {
    return cspb.ErrCodeOrderActvTmplCostumePassGift1, 0, 0
}
if len(cfg.BuyTypeValue) < 3 {
    tlog.Errorf("actv:%v actvCfg:%v giftId:%v BuyTypeValue配置不足(len=%d)，跳过校验",
        c.ActvID, c.CfgID, giftId, len(cfg.BuyTypeValue))
    return cspb.ErrCodeOrderActvTmplCostumePassGift1, 0, 0
}
if !c.IsStart() {
    return cspb.ErrCodeOrderActvTmplCostumePassGift1, 0, 0
}
if c.CfgID != cfg.BuyTypeValue[0] {
    return cspb.ErrCodeOrderActvTmplCostumePassGift1, 0, 0
}
...
if cfg.BuyTypeValue[2] == 0 {
    ...
}
```

好处：
- 把 panic 转成一条可控的、信息量更明确的错误日志（能直接看到 giftId 和当前配置长度，比 panic recover 打出的堆栈更容易定位）；
- 避免每次调用都触发 panic/recover（recover 本身有一定性能开销，高频调用场景下有意义）；
- 新旧两处模板基类（`actv/cactv` 与 `game/actv`）都需要同步修改。

### 4.3 加固配置加载阶段的校验（可选）
`afterLoadD2GiftCfg` 里已有的空值检查目前只是记日志然后 `continue`，建议评估是否需要：
- 在配置校验/发布工具链中加一条静态检查规则：`buy_type == GiftLimitActv` 时 `buy_type_value` 长度必须 >= 3，直接在配置提交/发布阶段拦截，而不是等到运行时才发现。

## 5. 结论

- 根因：活动 1020309 关联礼包的 `buy_type_value` 配置为空，与代码假定的"活动限购礼包必须有至少 1（甚至 3）个 `BuyTypeValue` 元素"这一隐含契约不符。
- 该问题在配置加载阶段已经有部分日志提示（`afterLoadD2GiftCfg` 中的 `tlog.Errorf`），但没有阻止错误配置被使用，也没有覆盖到 `CheckOrderCreateCondtions` 这条独立的读取路径。
- 当前 `recover()` 兜底保证了服务稳定性，不会引发活动/进程崩溃，但会在批量查询接口（`OnP2AActvGiftCheckReq`）高频调用下产生大量错误日志。
- 建议同时执行 4.1（改配置数据，立即止损）和 4.2（代码加防御，避免同类问题再次以 panic 形式出现）。
