package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gogu-x/bigTree/log"
	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/activity"
	"github.com/gogu-x/gogs/game/gate"
	"github.com/gogu-x/gogs/game/guild"
	natsclient "github.com/gogu-x/gogs/natsrpc"
	_ "github.com/gogu-x/gogs/pb/pbregister"
	rpcmongo "github.com/gogu-x/gogs/rpc/mongo"

	actor "github.com/gogu-x/bigTree"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "game",
		Usage: "game server",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:     "server-id",
				Aliases:  []string{"id"},
				Usage:    "server ID (unique per game server group, e.g. 1, 2, 3)",
				Required: true,
			},
			&cli.IntFlag{
				Name:     "node-id",
				Aliases:  []string{"node-id"},
				Usage:    "node ID (unique per game server group, e.g. 1, 2, 3)",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "port",
				Usage: "gRPC listen port override (default: 9000+serverID)",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			config.ServerID = c.Int("server-id")
			config.NodeId = c.Int("node-id")

			if p := c.String("port"); p != "" {
				config.GrpcPortOverride = p
			}

			if err := cluster.Init(config.EtcdEndpoints); err != nil {
				log.Fatal("cluster init error: " + err.Error())
			}
			defer cluster.Close()

			if err := natsclient.Init(config.NatsURL); err != nil {
				log.Fatal("NATS init error: " + err.Error())
			}
			defer natsclient.Close()

			serverID := fmt.Sprintf("%d", config.ServerID)
			NodeID := fmt.Sprintf("%d", config.NodeId)
			addr := config.GameAddr()

			if err := cluster.Register(serverID, NodeID, addr); err != nil {
				log.Fatal("cluster register error: " + err.Error())
			}
			fmt.Printf("game server [%s] inst=%s registered at %s\n", serverID, NodeID, addr)

			db := rpcmongo.Connect(config.MongoURL, fmt.Sprintf("game_%v", serverID))

			actor.Spawn(constant.ActorNats, gate.NewNatsActor(NodeID))
			actor.Spawn(constant.ActorGuild, guild.NewGuildActor())
			actor.Spawn(constant.ActorActivity, activity.NewActivityActor())
			actor.Spawn(constant.ActorGameMongo, rpcmongo.NewActor(db))
			actor.Default().Start()

			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err.Error())
	}
}
