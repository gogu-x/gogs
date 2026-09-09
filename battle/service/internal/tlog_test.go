package internal

import "github.com/gogu-x/tree/tlog"

// init 在单测进程初始化日志器：manager / per-battle battle 在特定分支会打
// tlog，未初始化时 tlog.Log 为 nil。与 main.go 的 tlog.NewLog(conf.LogPath,0)
// 等价，path 为空只写 stdout，不会产生文件。
func init() {
	if tlog.Log == nil {
		tlog.NewLog("", 0)
	}
}
