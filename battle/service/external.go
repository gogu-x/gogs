// Package service 提供 battle 进程内的 manager battle（BATTLE_SERVICE）。
// 这是模块的对外壳（external）：暴露 battle 构造器与装配/测试所需的类型与
// 工厂。具体实现分层如下：
//   - per-battle battle 与战报领域模型（Report/Repository/Source/Notifier）
//     在 battle/battle 独立包（manager 运行期 SpawnOne 出来执行每场战斗）；
//   - manager、持久化实现（内存/Mongo）、NATS 通知实现在本包 internal。
//
// 与 game/play、game/guild 等模块的 external+internal 组织一致。
package service

import (
	"github.com/gogu-x/gogs/battle/service/internal"
	"github.com/gogu-x/tree"
)

func New() tree.Actor {
	return internal.New()
}
