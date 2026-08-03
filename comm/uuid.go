// Package comm 提供跨进程共享的公共基础方法（唯一 ID 生成、常用工具函数等）。
package comm

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// 64 位唯一 ID（雪花算法变体）
//
// 位布局（共 64 bit，最高位始终为 0，保证结果为正数）：
//
//	 0                   1                   2                   3
//	 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 ...
//	+-+-----------------------------------------------------------+-----------+----------+
//	|0|                    timestamp (41 bit, ms)                 | node(10) | seq(12)  |
//	+-+-----------------------------------------------------------+-----------+----------+
//
//   - timestamp: 相对 epoch 的毫秒数，41 bit 可用 ~69 年
//   - node:      节点 ID，10 bit，取值 0~1023，用于区分不同进程/实例（如 gate-id、node-id）
//   - seq:       同一毫秒内的自增序号，12 bit，每毫秒最多 4096 个
//
// 单节点内保证严格单调递增；不同节点通过 nodeID 区分，避免冲突。
// ---------------------------------------------------------------------------

const (
	nodeBits = 10
	seqBits  = 12

	maxNodeID = -1 ^ (-1 << nodeBits) // 1023
	maxSeq    = -1 ^ (-1 << seqBits)  // 4095

	timeShift = nodeBits + seqBits
	nodeShift = seqBits
)

// epoch 起始时间：2025-01-01T00:00:00Z 的毫秒时间戳，用于压缩 timestamp 位宽。
var epoch = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

// IDGenerator 是一个可并发调用的 64 位 ID 生成器。
type IDGenerator struct {
	mu     sync.Mutex
	nodeID int64
	lastMs int64
	seq    int64
}

// NewIDGenerator 创建一个 ID 生成器，nodeID 取值范围 [0, 1023]。
// 分布式部署时，nodeID 应保证同时运行的进程之间互不相同
// （例如用 gate-id、game 的 server-id*节点数+node-id 等推导）。
func NewIDGenerator(nodeID int64) (*IDGenerator, error) {
	if nodeID < 0 || nodeID > maxNodeID {
		return nil, fmt.Errorf("comm: nodeID must be in [0, %d], got %d", maxNodeID, nodeID)
	}
	return &IDGenerator{nodeID: nodeID, lastMs: -1}, nil
}

// NextID 生成下一个全局唯一（同 nodeID 前提下严格单调递增）的 64 位 ID。
// 若单毫秒内序号耗尽，会自旋等待到下一毫秒。
func (g *IDGenerator) NextID() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UnixMilli() - epoch
	if now < g.lastMs {
		// 系统时钟回拨：直接复用上一次的时间戳，靠 seq 继续递增，
		// 避免生成重复或倒退的 ID；等真实时间追上后自然恢复正常。
		now = g.lastMs
	}

	if now == g.lastMs {
		g.seq = (g.seq + 1) & maxSeq
		if g.seq == 0 {
			// 当前毫秒序号用尽，忙等到下一毫秒。
			for now <= g.lastMs {
				now = time.Now().UnixMilli() - epoch
			}
		}
	} else {
		g.seq = 0
	}

	g.lastMs = now
	return (now << timeShift) | (g.nodeID << nodeShift) | g.seq
}

// NextUint64 与 NextID 等价，返回 uint64（便于直接赋值给 protobuf 的 uint64 字段）。
func (g *IDGenerator) NextUint64() uint64 {
	return uint64(g.NextID())
}

// ---------------------------------------------------------------------------
// 进程内简易唯一 ID（32 / 64 位自增计数器）
//
// 适用于不需要跨进程全局唯一、只需保证单进程内不重复的场景，
// 例如 Gate 进程内的连接 ID（connID）。
// 用当前 Unix 时间戳做起点，重启后也基本不会与历史值冲突。
// ---------------------------------------------------------------------------

// Counter32 是一个并发安全的 32 位自增 ID 生成器。
type Counter32 struct {
	mu  sync.Mutex
	cur uint32
}

// NewCounter32 创建一个从当前 Unix 时间戳（截断为 32 位）起步的计数器。
func NewCounter32() *Counter32 {
	return &Counter32{cur: uint32(time.Now().Unix())}
}

// Next 返回下一个 32 位唯一 ID。溢出后回绕到 0，调用方若长期运行且要求
// 绝对不重复，请改用 IDGenerator（64 位）。
func (c *Counter32) Next() uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cur++
	return c.cur
}

// Counter64 是一个并发安全的 64 位自增 ID 生成器。
type Counter64 struct {
	mu  sync.Mutex
	cur uint64
}

// NewCounter64 创建一个从当前 Unix 时间戳起步的计数器。
func NewCounter64() *Counter64 {
	return &Counter64{cur: uint64(time.Now().Unix())}
}

// Next 返回下一个 64 位唯一 ID。
func (c *Counter64) Next() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cur++
	return c.cur
}

// ---------------------------------------------------------------------------
// UUID 字符串 ID
// ---------------------------------------------------------------------------

// NewUUID 返回一个标准格式（带短横线）的 UUID v4 字符串，
// 适用于 NATS 请求 ID、trace ID 等不要求数值类型的场景。
func NewUUID() string {
	return uuid.NewString()
}
