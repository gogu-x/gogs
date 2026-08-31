package natsrpc

import (
	"fmt"
)

const (
	subIn = "sub:%v:%v:%v"
)

func GameSubject(serverID, NodeID uint32) string {
	return fmt.Sprintf(subIn, ModuleGame, serverID, NodeID)
}
func GateSubject(gateID, NodeID uint32) string { return fmt.Sprintf(subIn, ModuleGate, gateID, NodeID) }
func ActivitySubject(serverID, NodeID uint32) string {
	return fmt.Sprintf(subIn, ModuleActivity, serverID, NodeID)
}

// publishTo
func publishTo(module, id, nodeID string, msg []byte) error {
	subject, err := subjectFor(module, id, nodeID)
	if err != nil {
		return err
	}
	return nc.Publish(subject, msg)
}
