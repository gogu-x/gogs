package internal

import (
	"context"
	"sync"
)

// MemoryRepository 是用于本地运行和测试的线程安全战报实现。
type MemoryRepository struct {
	mu      sync.RWMutex
	reports map[string]Report
}

// NewMemoryRepository 创建一个空的内存战报仓储。
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{reports: make(map[string]Report)}
}

// Save 保存战报。
func (r *MemoryRepository) Save(_ context.Context, report Report) error {
	r.mu.Lock()
	r.reports[report.BattleID] = report
	r.mu.Unlock()
	return nil
}

// Get 获取战报。
func (r *MemoryRepository) Get(_ context.Context, battleID string) (Report, error) {
	r.mu.RLock()
	report, exists := r.reports[battleID]
	r.mu.RUnlock()
	if !exists {
		return Report{}, ErrReportNotFound
	}
	return report, nil
}

// ListByPlayer 获取玩家战报。
func (r *MemoryRepository) ListByPlayer(_ context.Context, uid uint64, limit int) ([]Report, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reports := make([]Report, 0)
	for _, report := range r.reports {
		if report.UID == uid {
			reports = append(reports, report)
			if limit > 0 && len(reports) >= limit {
				break
			}
		}
	}
	return reports, nil
}

// Confirm 标记战报已确认。
func (r *MemoryRepository) Confirm(_ context.Context, battleID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	report, exists := r.reports[battleID]
	if !exists {
		return ErrReportNotFound
	}
	report.Confirmed = true
	r.reports[battleID] = report
	return nil
}
