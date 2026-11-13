package conf

import "testing"

// TestPprofAddresses 验证诊断端口随各自编号变化，且区分 game 与 Gate。
func TestPprofAddresses(t *testing.T) {
	serverID, gateID := ServerID, GateID
	t.Cleanup(func() { ServerID, GateID = serverID, gateID })
	ServerID, GateID = 1, 2
	if GamePprofAddr() != "127.0.0.1:16001" || GatePprofAddr() != "127.0.0.1:26002" {
		t.Fatal("诊断端口未按服务编号生成")
	}
	ServerID, GateID = 974, 74
	if GamePprofAddr() != "127.0.0.1:16974" || GatePprofAddr() != "127.0.0.1:26074" {
		t.Fatal("诊断端口没有随编号变化")
	}
}
