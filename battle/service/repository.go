package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/gogu-x/gogs/battle/engine"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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

type Repository interface {
	Save(context.Context, Report) error
	Get(context.Context, string) (Report, error)
	ListByPlayer(context.Context, uint64, int) ([]Report, error)
	Confirm(context.Context, string) error
}

func cloneReport(in Report) (Report, error) {
	data, err := json.Marshal(in)
	if err != nil {
		return Report{}, err
	}
	var out Report
	if err := json.Unmarshal(data, &out); err != nil {
		return Report{}, err
	}
	return out, nil
}

// MemoryRepository is concurrency-safe and intentionally used by unit and
// end-to-end tests so neither MongoDB nor NATS is required.
type MemoryRepository struct {
	mu      sync.RWMutex
	reports map[string]Report
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{reports: make(map[string]Report)}
}

func (r *MemoryRepository) Save(_ context.Context, report Report) error {
	if report.BattleID == "" || report.Result.BattleID != report.BattleID {
		return fmt.Errorf("battle repository: invalid report identity")
	}
	copy, err := cloneReport(report)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.reports[report.BattleID] = copy
	r.mu.Unlock()
	return nil
}

func (r *MemoryRepository) Get(_ context.Context, battleID string) (Report, error) {
	r.mu.RLock()
	report, ok := r.reports[battleID]
	r.mu.RUnlock()
	if !ok {
		return Report{}, ErrReportNotFound
	}
	return cloneReport(report)
}

func (r *MemoryRepository) ListByPlayer(_ context.Context, uid uint64, limit int) ([]Report, error) {
	if limit <= 0 {
		limit = 20
	}
	r.mu.RLock()
	items := make([]Report, 0)
	for _, report := range r.reports {
		if report.UID == uid {
			items = append(items, report)
		}
	}
	r.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]Report, 0, len(items))
	for _, report := range items {
		copy, err := cloneReport(report)
		if err != nil {
			return nil, err
		}
		out = append(out, copy)
	}
	return out, nil
}

func (r *MemoryRepository) Confirm(_ context.Context, battleID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	report, ok := r.reports[battleID]
	if !ok {
		return ErrReportNotFound
	}
	report.Confirmed = true
	report.UpdatedAt = time.Now().UTC()
	r.reports[battleID] = report
	return nil
}

type MongoRepository struct {
	collection *mongo.Collection
	timeout    time.Duration
}

func NewMongoRepository(db *mongo.Database, collection string) *MongoRepository {
	if collection == "" {
		collection = "battle_reports"
	}
	return &MongoRepository{collection: db.Collection(collection), timeout: 8 * time.Second}
}

func (r *MongoRepository) withTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, r.timeout)
}

func (r *MongoRepository) Save(parent context.Context, report Report) error {
	if report.BattleID == "" || report.Result.BattleID != report.BattleID {
		return fmt.Errorf("battle repository: invalid report identity")
	}
	ctx, cancel := r.withTimeout(parent)
	defer cancel()
	_, err := r.collection.ReplaceOne(ctx, bson.M{"_id": report.BattleID}, report, options.Replace().SetUpsert(true))
	return err
}

func (r *MongoRepository) Get(parent context.Context, battleID string) (Report, error) {
	ctx, cancel := r.withTimeout(parent)
	defer cancel()
	var report Report
	err := r.collection.FindOne(ctx, bson.M{"_id": battleID}).Decode(&report)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Report{}, ErrReportNotFound
	}
	return report, err
}

func (r *MongoRepository) ListByPlayer(parent context.Context, uid uint64, limit int) ([]Report, error) {
	if limit <= 0 {
		limit = 20
	}
	ctx, cancel := r.withTimeout(parent)
	defer cancel()
	cursor, err := r.collection.Find(ctx, bson.M{"uid": uid}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var reports []Report
	if err := cursor.All(ctx, &reports); err != nil {
		return nil, err
	}
	return reports, nil
}

func (r *MongoRepository) Confirm(parent context.Context, battleID string) error {
	ctx, cancel := r.withTimeout(parent)
	defer cancel()
	res, err := r.collection.UpdateOne(ctx, bson.M{"_id": battleID}, bson.M{"$set": bson.M{"confirmed": true, "updated_at": time.Now().UTC()}})
	if err == nil && res.MatchedCount == 0 {
		return ErrReportNotFound
	}
	return err
}
