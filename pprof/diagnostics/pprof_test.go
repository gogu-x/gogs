package diagnostics

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// TestRuntimeEndpoints 验证诊断路由和堆信息可读，且不暴露默认路由。
func TestRuntimeEndpoints(t *testing.T) {
	mux := newMux()
	for _, path := range []string{"/debug/runtime", "/debug/pprof/", "/debug/pprof/goroutine?debug=1", "/debug/pprof/heap", "/debug/pprof/block", "/debug/pprof/mutex"} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
		if recorder.Code != 200 || recorder.Body.Len() == 0 {
			t.Fatalf("%s: %d", path, recorder.Code)
		}
		if path == "/debug/runtime" {
			var result map[string]interface{}
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result["memstats"] == nil || result["goroutines"] == nil {
				t.Fatal("缺少运行时数据")
			}
		}
	}
}

// TestRejectPublicAddress 验证诊断端口拒绝公网绑定。
func TestRejectPublicAddress(t *testing.T) {
	for _, addr := range []string{":6060", "0.0.0.0:6060", "192.168.1.1:6060"} {
		if _, err := Start(addr, 0, 0); err == nil {
			t.Fatalf("未拒绝地址 %s", addr)
		}
	}
}
