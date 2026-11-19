package glconf

import "github.com/gogu-x/tree/tlog"

var afterLoadCallbacks = make(map[string][]func(*CsvConf))

// RegisterAfterLoad 按配置表的 CfgKey 注册加载回调，未知 key 或空回调返回 false。
// 服务启动时串行注册，首次加载配置前须完成所有注册。
// 每次该表成功加载并发布后，加载协程按注册顺序同步传入本次加载的表，供回调只读使用。
func RegisterAfterLoad(key string, callback func(*CsvConf)) bool {
	InitGameCfg()
	if callback == nil || getConfMeta(key) == nil {
		tlog.Error("[glconf/RegisterAfterLoad] 配置加载回调注册失败, key=%v, nilCallback=%v", key, callback == nil)
		return false
	}
	afterLoadCallbacks[key] = append(afterLoadCallbacks[key], callback)
	return true
}

func runAfterLoadCallbacks(key string, table *CsvConf) {
	callbacks := afterLoadCallbacks[key]
	if len(callbacks) == 0 {
		return
	}
	tlog.Info("[glconf/runAfterLoadCallbacks] 执行配置加载回调, key=%v, count=%v", key, len(callbacks))
	for _, callback := range callbacks {
		callback(table)
	}
}
