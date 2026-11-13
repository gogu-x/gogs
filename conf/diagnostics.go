package conf

import "fmt"

const (
	// gamePprofBasePort 是 game 诊断基础端口，实际端口加区服编号。
	gamePprofBasePort = 16000
	// gatePprofBasePort 是 Gate 诊断基础端口，实际端口加网关编号。
	gatePprofBasePort = 26000
)

// GamePprofAddr 根据区服编号生成本机诊断地址。
func GamePprofAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", gamePprofBasePort+ServerID)
}

// GatePprofAddr 根据网关编号生成本机诊断地址。
func GatePprofAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", gatePprofBasePort+GateID)
}
