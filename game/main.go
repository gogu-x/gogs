package main

import (
	"context"
	"fmt"
	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/game/model"
	"github.com/gogu-x/bigTree/log"
	"os"

	actor "github.com/gogu-x/bigTree"

	"github.com/urfave/cli/v3"
)

func main() {
	if err := cluster.Init(config.EtcdEndpoints); err != nil {
		log.Fatal("cluster init error: " + err.Error())
	}
	defer cluster.Close()

	cmd := &cli.Command{
		Name:  "game",
		Usage: "game server",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:     "server-id",
				Aliases:  []string{"id"},
				Usage:    "server ID (unique per game process, e.g. 1, 2, 3)",
				Required: true,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			config.ServerID = int(c.Int("server-id"))
			grpcAddr := config.GrpcAddr()

			fmt.Printf("game server [%d] starting, gRPC: %s\n", config.ServerID, grpcAddr)

			sys := actor.NewActorSystem()
			sys.Spawn("game", &model.GameActor{})
			sys.Spawn("gate", &model.GateActor{})

			sys.Start()
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err.Error())
	}
}
