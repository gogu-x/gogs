package natsrpc

import "fmt"

// SubjectFunc 根据 (id, nodeID) 生成目标 NATS subject。
// nodeID 对不需要节点维度的模块（如 gate、platform）可忽略。
type SubjectFunc func(id, nodeID string) string

var moduleRegistry = make(map[string]SubjectFunc)

// RegisterModule 注册一个模块的 subject 生成规则。
// 各模块应在自己的 init() 或启动流程中调用一次，之后即可通过
// Cast/Call/CallSync 用 (module, id, nodeID) 三元组统一寻址。
//
// 例如：
//
//	natsrpc.RegisterModule(natsrpc.GameNats, func(id, nodeID string) string {
//	    return fmt.Sprintf("game:%s:%s", id, nodeID)
//	})
func RegisterModule(module string, fn SubjectFunc) {
	moduleRegistry[module] = fn
}

// subjectFor 查表生成目标 subject。
func subjectFor(module, id, nodeID string) (string, error) {
	fn, ok := moduleRegistry[module]
	if !ok {
		return "", fmt.Errorf("natsrpc: module %q not registered", module)
	}
	return fn(id, nodeID), nil
}

func init() {
	// 内置模块，保持与原 send() 分支一致的路由规则。
	RegisterModule(GameNats, func(id, nodeID string) string { return GameInSubject(id, nodeID) })
	RegisterModule(ModuleGate, func(id, _ string) string { return GateOutSubject(id) })
	RegisterModule(ModuleCross, func(id, _ string) string { return fmt.Sprintf(subCross, id) })
	RegisterModule(ModuleDeliver, func(id, _ string) string { return fmt.Sprintf(subDeliver, id) })
}
