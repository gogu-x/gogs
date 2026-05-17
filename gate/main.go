package main

import (
	"log"

	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/gate/server"
	_ "github.com/gogu-x/gogs/pb/game"

	actor "github.com/gogu-x/bigTree"
)

func main() {
	if err := cluster.Init(config.EtcdEndpoints); err != nil {
		log.Fatalf("cluster init error: %v", err)
	}
	defer cluster.Close()

	sys := actor.NewActorSystem()
	sys.Spawn("registry", server.NewRegistryActor())

	srv := server.NewGateServer(sys, config.GateListenAddr)
	log.Printf("gate server listening on %s", config.GateListenAddr)

	go sys.Start()

	if err := srv.Start(); err != nil {
		log.Fatalf("gate server error: %v", err)
	}
}
