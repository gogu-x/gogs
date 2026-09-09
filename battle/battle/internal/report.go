// Package battle 承载"每场战斗一个执行 battle"的实现与它的战报领域模型。
// 战报（Report）与存取契约（Repository）归属在这里：每场战斗的战报在各自
// BattleActor 内自洽落库/推送/确认/重试，manager（battle/service）只负责
// 创建与回收这些 battle，不持有战报。
//
// battle 只依赖结算引擎、协议与 battle 框架，不反向依赖 service，从而与
// manager 解耦（manager import battle 完成创建）。
package internal

import (
	"context"
	"errors"
	"time"

	"github.com/gogu-x/gogs/battle/battle/internal/engine"
)

var ErrReportNotFound = errors.New("battle report not found")

// Report is the durable hand-off between battle execution and result delivery.
// A Finished notification must never be sent before this document is saved.
type Report struct {
	BattleID       string        `bson:"_id" json:"battle_id"`
	UID            uint64        `bson:"uid" json:"uid"`
	SourceServerID int           `bson:"source_server_id" json:"source_server_id"`
	SourceNodeID   int           `bson:"source_node_id" json:"source_node_id"`
	Result         engine.Result `bson:"result" json:"result"`
	CreatedAt      time.Time     `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time     `bson:"updated_at" json:"updated_at"`
	Confirmed      bool          `bson:"confirmed" json:"confirmed"`
}

// Repository 是战报的持久化存取契约，由各进程装配层提供实现（如 Mongo、
// 内存）。它属于战斗 battle 的战报模型，供 battle 落库/确认与 manager 只读查询共用。
type Repository interface {
	Save(context.Context, Report) error
	Get(context.Context, string) (Report, error)
	ListByPlayer(context.Context, uint64, int) ([]Report, error)
	Confirm(context.Context, string) error
}
