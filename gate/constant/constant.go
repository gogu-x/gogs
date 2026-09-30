package constant

import "fmt"

const (
	ActorRegistry   = "registry"
	ActorGateServer = "gate-ws"
)

func ConnName(connID uint64) string { return fmt.Sprintf("conn-%d", connID) }

func StreamName(serverID int32) string { return fmt.Sprintf("stream-%v", serverID) }
