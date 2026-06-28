package natsrpc

import (
	"fmt"

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

func send(m *SendMsg) error {
	switch m.Module {
	case GameNats:
		return publish(fmt.Sprintf(subGameIn, m.ID, m.NodeId), m.Frame)
	case ModuleGate:
		return publish(fmt.Sprintf(subGateOut, m.ID), m.Frame)
	case ModuleCross:
		return publish(fmt.Sprintf(subCross, m.ID), m.Frame)
	case ModuleDeliver:
		return publish(fmt.Sprintf(subDeliver, m.ID), m.Frame)
	default:
		return fmt.Errorf("unknown module: %s", m.Module)
	}
}

func PublishShutdown(serverID, instID string) error {
	return nc.Publish(fmt.Sprintf(subGameShutdown, serverID, instID), []byte("shutdown"))
}

func PublishDeliver(serverID string, msg proto.Message) error {
	return publish(fmt.Sprintf(subDeliver, serverID), msg)
}
