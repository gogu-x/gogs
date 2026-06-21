package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gogu-x/bigTree/log"
	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/activity"
	"github.com/gogu-x/gogs/game/gate"
	"github.com/gogu-x/gogs/game/guild"
	"github.com/gogu-x/gogs/game/player"
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
			&cli.StringFlag{
				Name:  "port",
				Usage: "gRPC listen port override (default: 9000+serverID)",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			config.ServerID = c.Int("server-id")
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
			instID := fmt.Sprintf("%v", time.Now().Unix())
			addr := config.GameAddr()

			fmt.Println(instID)
			if err := cluster.Register(serverID, instID, addr); err != nil {
				log.Fatal("cluster register error: " + err.Error())
			}
			fmt.Printf("game server [%s] inst=%s registered at %s\n", serverID, instID, addr)

			db := rpcmongo.Connect(config.MongoURL, "game")

			actor.Spawn(constant.ActorNats, player.NewNatsActor(instID))
			actor.Spawn(constant.ActorGate, gate.NewGateActor())
			actor.Spawn(constant.ActorGuild, guild.NewGuildActor())
			actor.Spawn(constant.ActorActivity, activity.NewActivityActor())
			actor.Spawn(constant.ActorPlatformMongo, rpcmongo.NewActor(db))
			actor.Default().Start()

			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err.Error())
	}
}
