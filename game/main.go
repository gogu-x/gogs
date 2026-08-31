package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/activity"
	"github.com/gogu-x/gogs/game/gate"
	"github.com/gogu-x/gogs/game/guild"
	gamenats "github.com/gogu-x/gogs/game/nats"
	"github.com/gogu-x/gogs/game/play"
	natsclient "github.com/gogu-x/gogs/natsrpc"
	_ "github.com/gogu-x/gogs/pb/pbregister"
	rpcmongo "github.com/gogu-x/gogs/rpc/mongo"
	cluster2 "github.com/gogu-x/tree/cluster"
	"github.com/gogu-x/tree/log"

	"github.com/gogu-x/tree"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "game",
		Usage: "game server",
		Flags: config.ConnectionFlags(),
		Action: func(ctx context.Context, c *cli.Command) error {
			if err := config.LoadAndApply(c); err != nil {
				return err
			}

			if err := cluster2.Init(config.EtcdEndpoints); err != nil {
				log.Fatal("cluster init error: " + err.Error())
			}
			defer cluster2.Close()

			if err := natsclient.Init(config.NatsURL); err != nil {
				log.Fatal("NATS init error: " + err.Error())
			}
			defer natsclient.Close()

			serverID := fmt.Sprintf("%d", config.ServerID)
			NodeID := fmt.Sprintf("%d", config.NodeId)
			addr := config.GameAddr()

			if err := cluster2.Register(serverID, NodeID, addr); err != nil {
				log.Fatal("cluster register error: " + err.Error())
			}
			fmt.Printf("game server [%s] inst=%s registered at %s\n", serverID, NodeID, addr)

			db := rpcmongo.Connect(
				config.MongoURL,
				config.MongoUsername,
				config.MongoPassword,
				fmt.Sprintf("game_%v", serverID),
			)

			tree.Spawn(
				play.NewPlay(),
				gate.NewGate(),
				guild.NewGuild(),
				activity.NewActivity(),
				rpcmongo.NewActor(constant.Mongo, db),
				gamenats.NewNats(uint32(config.ServerID), uint32(config.NodeId)),
			)
			tree.Default().Start()

			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err.Error())
	}
}
