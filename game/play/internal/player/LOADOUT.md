# Server-owned loadouts

The player actor owns inventory and progression. Each request controller explicitly
validates its request, calls its model rule, marks successful changes dirty and
returns a snapshot. `player/loadout.go` only builds views and battle modifiers;
there is no generic mutation callback or separate `game/play/internal/logic`
layer. Model rules live alongside the persisted soul jade and talent managers.

All game data comes from generated `glconf` tables:

- `LoadoutInitCfg`: initial soul jade instances, equipped definitions, level,
  materials and slot count.
- `SoulJadeCfg`: names, rarity, icon keys and initial modifiers.
- `SoulJadeLevelCfg`: available progression levels, costs, modifier count,
  upgrade increments and reroll pools/ranges.
- `SoulJadeEffectAffixCfg`: 冰爆、天雷词条的洗练权重、积攒阈值、命中增量、伤害系数和目标规则。
- `TalentRuleCfg`: initial points, level threshold, progression coefficients
  and the point cap.
- `TalentBranchCfg` and `TalentNodeCfg`: catalog, layout metadata, roots,
  prerequisites, costs, presentation and battle modifiers.

Missing or invalid required configuration does not create fallback inventory or
talents. Existing accounts receive missing modules only once during login loading.
Configured free roots are reconciled without resetting learned nodes; disabled or
duplicate slot references are removed while owned instances remain in inventory.

效果词条与基础属性词条一起参与洗练。效果词条的配置 ID 保存在
`SoulJadeAffix.Attribute`，`Permille` 固定为 0；基础属性继续使用 1～4 号属性
和原有千分比范围。装备魂玉后，Game 将有效效果词条 ID 放入权威战斗角色快照，
Battle 按配置为该角色建立独立的冰爆或天雷积攒值。

Every mutation checks the authenticated role and expected revision before changing
loaded memory. Successful mutations increment the module revision and mark the
player dirty. Existing lifecycle autosave, logout and shutdown paths persist the
data; request handlers do not query or write MongoDB directly.

Every response includes an explicitly present, predefined `ErrCode`, including
`OK` for success. Model operations return one `ErrCode`; no free-form error string
or string-code conversion participates in the request path. Failed requests still
return the current snapshot when available. Clients localize enum values. Names,
rarity and effect text remain presentation data from configuration.

Battle requests rebuild growth modifiers from server-owned modules and ignore
client-submitted progression. The battle compiler applies the combined modifiers
once, after adding equipment values to role base attributes. Attribute identifiers
and permille units are protocol semantics, not configurable gameplay formulas.

Controller tests invoke the actual handlers and capture replies through a test
gateway context. They check explicit status presence, replay protection, material
balances, snapshots and dirty flags. Tests load the actual source TSV files into
in-memory generated table installers.
Additional tests replace the tables with different identifiers, sparse levels,
costs, prerequisites and slot counts to detect hardcoded gameplay rules. No test
requires a database import or a service restart.
