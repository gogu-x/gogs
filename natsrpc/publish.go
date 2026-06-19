package natsrpc

import (
	"fmt"

	"google.golang.org/protobuf/proto"
)

const (
	subGameIn       = "gate.in.%s"
	subGateOut      = "gate.out.%s.%d"
	subCross        = "cross.%s"
	subGameShutdown = "game.shutdown.%s.%s"
)

func PublishToGame(serverID string, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("natsrpc.PublishToGame: %w", err)
	}
	return nc.Publish(fmt.Sprintf(subGameIn, serverID), data)
}

func PublishRawToGate(gateID string, connID uint64, data []byte) error {
	return nc.Publish(fmt.Sprintf(subGateOut, gateID, connID), data)
}

func PublishCross(topic string, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("natsrpc.PublishCross: %w", err)
	}
	return nc.Publish(fmt.Sprintf(subCross, topic), data)
}

func PublishShutdown(serverID, instID string) error {
	return nc.Publish(fmt.Sprintf(subGameShutdown, serverID, instID), []byte("shutdown"))
}
