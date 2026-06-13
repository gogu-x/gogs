package main

import (
	"context"
	"fmt"
	"os"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/bigTree/log"
	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/gate/server"
	natsclient "github.com/gogu-x/gogs/nats"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "gate",
		Usage: "gate server",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:     "gate-id",
				Aliases:  []string{"id"},
				Usage:    "gate ID (unique per gate process, e.g. 1, 2, 3)",
				Required: true,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			config.GateID = c.Int("gate-id")
			addr := config.GateAddr()

			if err := cluster.Init(config.EtcdEndpoints); err != nil {
				log.Fatal("cluster init error: " + err.Error())
			}
			defer cluster.Close()

			if err := natsclient.Init(config.NatsURL); err != nil {
				log.Fatal("nats init error: " + err.Error())
			}
			defer natsclient.Close()

			fmt.Printf("gate server [%d] starting, listen: %s\n", config.GateID, addr)

			actor.Spawn(server.ActorRegistry, &server.RegistryActor{})
			actor.Spawn(server.ActorNats, server.NewNatsActor())

			srv := server.NewGateServer(addr)
			go actor.Default().Start()

			if err := srv.Start(); err != nil {
				log.Fatal("gate server error: " + err.Error())
			}
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err.Error())
	}
}
