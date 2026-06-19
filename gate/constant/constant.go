package constant

import "fmt"

const (
	ActorNats       = "nats"
	ActorRegistry   = "registry"
	ActorGateServer = "gate-server"
)

func ConnName(connID uint64) string { return fmt.Sprintf("conn-%d", connID) }
