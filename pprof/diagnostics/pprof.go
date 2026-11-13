// Package diagnostics 提供仅限本机访问的运行时诊断接口。
package diagnostics

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"runtime"
	"time"

	"github.com/gogu-x/tree/tlog"
)

// Start 启动诊断服务；空地址禁用，非回环地址拒绝启动。
// blockRate 为阻塞采样纳秒间隔，mutexFraction 为锁竞争采样比例。
func Start(addr string, blockRate, mutexFraction int) (func(), error) {
	if addr == "" {
		return func() {}, nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("pprof 地址必须使用回环 IP: %s", addr)
	}
	if blockRate < 0 || mutexFraction < 0 {
		return nil, fmt.Errorf("pprof 采样参数不能为负数")
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	runtime.SetBlockProfileRate(blockRate)
	previousMutex := runtime.SetMutexProfileFraction(mutexFraction)
	server := &http.Server{Handler: newMux(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			tlog.Log.Error("[diagnostics/Start] 诊断服务异常, addr=%v, err=%v", addr, err)
		}
	}()
	tlog.Log.Info("[diagnostics/Start] 诊断服务已启动, addr=%v, blockRate=%v, mutexFraction=%v", listener.Addr(), blockRate, mutexFraction)
	return func() {
		_ = server.Close()
		runtime.SetBlockProfileRate(0)
		runtime.SetMutexProfileFraction(previousMutex)
	}, nil
}

// newMux 使用独立路由，避免暴露业务默认 HTTP 路由。
func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.HandleFunc("/debug/runtime", runtimeStats)
	return mux
}

// runtimeStats 返回堆使用量、释放量、GC 暂停记录及协程数，不主动触发 GC。
func runtimeStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"time": time.Now().UTC(), "goroutines": runtime.NumGoroutine(), "memstats": stats,
	})
}
