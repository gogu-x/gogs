package constant

import "fmt"

const (
	ActorNats       = "NATS"
	ActorRegistry   = "registry"
	ActorGateServer = "gate-server"
)

func ConnName(connID uint64) string { return fmt.Sprintf("conn-%d", connID) }

func StreamName(serverID uint64) string { return fmt.Sprintf("stream-%v", serverID) }
