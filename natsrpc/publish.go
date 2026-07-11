package natsrpc

import (
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

const (
	subGameIn       = "game:%s:%s"
	subGateOut      = "gate.out.%s"
	subCross        = "cross.%s"
	subGameShutdown = "game.shutdown.%s.%s"
	subDeliver      = "platform.deliver.%s"
)

func GameInSubject(serverID, NodeID string) string { return fmt.Sprintf(subGameIn, serverID, NodeID) }
func GateOutSubject(gateID string) string          { return fmt.Sprintf(subGateOut, gateID) }

func publish(subject string, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("natsrpc.publish: %w", err)
	}
	return nc.Publish(subject, data)
}

// publishTo 按 (module, id, nodeID) 查表寻址并发送 Frame。
func publishTo(module, id, nodeID string, frame *Frame) error {
	subject, err := subjectFor(module, id, nodeID)
	if err != nil {
		return err
	}
	return publish(subject, frame)
}

func PublishShutdown(serverID, instID string) error {
	return nc.Publish(fmt.Sprintf(subGameShutdown, serverID, instID), []byte("shutdown"))
}

func PublishDeliver(serverID string, msg proto.Message) error {
	return publish(fmt.Sprintf(subDeliver, serverID), msg)
}

func newRequestId() string {
	return uuid.NewString()
}
