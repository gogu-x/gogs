package comm

import (
	"sync"
	"testing"
)

func TestIDGenerator_Unique(t *testing.T) {
	g, err := NewIDGenerator(1)
	if err != nil {
		t.Fatalf("NewIDGenerator: %v", err)
	}

	const n = 100000
	seen := make(map[int64]struct{}, n)
	var last int64
	for i := 0; i < n; i++ {
		id := g.NextID()
		if id <= last {
			t.Fatalf("ID not strictly increasing: prev=%d cur=%d", last, id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate ID generated: %d", id)
		}
		seen[id] = struct{}{}
		last = id
	}
}

func TestIDGenerator_Concurrent(t *testing.T) {
	g, err := NewIDGenerator(2)
	if err != nil {
		t.Fatalf("NewIDGenerator: %v", err)
	}

	const goroutines = 50
	const perGoroutine = 2000

	var mu sync.Mutex
	seen := make(map[int64]struct{}, goroutines*perGoroutine)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				id := g.NextID()
				mu.Lock()
				if _, dup := seen[id]; dup {
					t.Errorf("duplicate ID generated under concurrency: %d", id)
				}
				seen[id] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != goroutines*perGoroutine {
		t.Fatalf("expected %d unique ids, got %d", goroutines*perGoroutine, len(seen))
	}
}

func TestIDGenerator_DifferentNodesNoCollision(t *testing.T) {
	g1, _ := NewIDGenerator(1)
	g2, _ := NewIDGenerator(2)

	seen := make(map[int64]struct{}, 2000)
	for i := 0; i < 1000; i++ {
		id1 := g1.NextID()
		id2 := g2.NextID()
		if _, dup := seen[id1]; dup {
			t.Fatalf("collision on node1 id: %d", id1)
		}
		seen[id1] = struct{}{}
		if _, dup := seen[id2]; dup {
			t.Fatalf("collision on node2 id: %d", id2)
		}
		seen[id2] = struct{}{}
	}
}

func TestNewIDGenerator_InvalidNodeID(t *testing.T) {
	if _, err := NewIDGenerator(-1); err == nil {
		t.Fatal("expected error for negative nodeID")
	}
	if _, err := NewIDGenerator(maxNodeID + 1); err == nil {
		t.Fatal("expected error for nodeID exceeding max")
	}
	if _, err := NewIDGenerator(0); err != nil {
		t.Fatalf("nodeID=0 should be valid: %v", err)
	}
	if _, err := NewIDGenerator(maxNodeID); err != nil {
		t.Fatalf("nodeID=maxNodeID should be valid: %v", err)
	}
}

func TestCounter32_Unique(t *testing.T) {
	c := NewCounter32()
	seen := make(map[uint32]struct{})
	for i := 0; i < 10000; i++ {
		id := c.Next()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate Counter32 id: %d", id)
		}
		seen[id] = struct{}{}
	}
}

func TestCounter32_Concurrent(t *testing.T) {
	c := NewCounter32()
	const goroutines = 50
	const perGoroutine = 1000

	var mu sync.Mutex
	seen := make(map[uint32]struct{}, goroutines*perGoroutine)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				id := c.Next()
				mu.Lock()
				if _, dup := seen[id]; dup {
					t.Errorf("duplicate Counter32 id under concurrency: %d", id)
				}
				seen[id] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
}

func TestCounter64_Unique(t *testing.T) {
	c := NewCounter64()
	seen := make(map[uint64]struct{})
	for i := 0; i < 10000; i++ {
		id := c.Next()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate Counter64 id: %d", id)
		}
		seen[id] = struct{}{}
	}
}

func TestNewUUID(t *testing.T) {
	a := NewUUID()
	b := NewUUID()
	if a == "" || b == "" {
		t.Fatal("NewUUID returned empty string")
	}
	if a == b {
		t.Fatal("two calls to NewUUID returned the same value")
	}
	if len(a) != 36 {
		t.Fatalf("expected standard UUID length 36, got %d (%s)", len(a), a)
	}
}
