package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// testDatabase 连接本地 MongoDB 的临时库；未提供 PLATFORM_TEST_MONGO 时跳过。
func testDatabase(t *testing.T) *mongo.Database {
	t.Helper()
	uri := os.Getenv("PLATFORM_TEST_MONGO")
	if uri == "" {
		t.Skip("未设置 PLATFORM_TEST_MONGO，跳过 MongoDB 集成测试")
	}
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	db := client.Database(fmt.Sprintf("platform_uid_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})
	return db
}

// TestUIDCounterStartsAfterExistingAccounts 验证计数器从已有最大 uid 之后分配，重复种子不回退，并发分配不重复。
func TestUIDCounterStartsAfterExistingAccounts(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	if _, err := db.Collection(ColAccounts).InsertMany(ctx, []interface{}{
		Account{ID: bson.NewObjectID(), Account: "a", UID: 1000},
		Account{ID: bson.NewObjectID(), Account: "b", UID: 5000},
	}); err != nil {
		t.Fatal(err)
	}
	if seq, err := SeedUIDCounter(ctx, db); err != nil || seq != 5000 {
		t.Fatalf("seed = %v, %v; want 5000", seq, err)
	}
	first, err := NextUID(ctx, db)
	if err != nil || first != 5001 {
		t.Fatalf("first uid = %v, %v; want 5001", first, err)
	}
	if seq, err := SeedUIDCounter(ctx, db); err != nil || seq != 5001 {
		t.Fatalf("重复种子回退了计数器: %v, %v", seq, err)
	}

	const workers, each = 8, 50
	var mu sync.Mutex
	seen := map[uint64]bool{first: true}
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				uid, err := NextUID(ctx, db)
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				if seen[uid] {
					t.Errorf("重复 uid %d", uid)
				}
				seen[uid] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(seen) != workers*each+1 {
		t.Fatalf("分配数量 %d, want %d", len(seen), workers*each+1)
	}
}
