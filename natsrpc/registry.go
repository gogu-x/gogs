package natsrpc

import "fmt"

// SubjectFunc 根据 (id, nodeID) 生成目标 NATS subject。
// nodeID 对不需要节点维度的模块（如 gate、platform）可忽略。
type SubjectFunc func(id, nodeID string) string

var moduleRegistry = make(map[string]SubjectFunc)

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
}
