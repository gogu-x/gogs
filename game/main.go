package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/activity"
	"github.com/gogu-x/gogs/game/gate"
	"github.com/gogu-x/gogs/game/guild"
	gamenats "github.com/gogu-x/gogs/game/nats"
	"github.com/gogu-x/gogs/game/play"
	natsclient "github.com/gogu-x/gogs/natsrpc"
	_ "github.com/gogu-x/gogs/pb/pbregister"
	"github.com/gogu-x/gogs/rpc/mongorpc"
	cluster2 "github.com/gogu-x/tree/cluster"
	"github.com/gogu-x/tree/log"

	"github.com/gogu-x/tree"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "game",
		Usage: "game server",
		Flags: conf.ConnectionFlags(),
		Action: func(ctx context.Context, c *cli.Command) error {
			if err := conf.LoadAndApply(c); err != nil {
				return err
			}
			def.NewLog(conf.LogPath, 0)

			if err := cluster2.Init(conf.EtcdEndpoints); err != nil {
				log.Fatal("cluster init error: " + err.Error())
			}
			defer cluster2.Close()

			if err := natsclient.Init(conf.NatsURL); err != nil {
				log.Fatal("NATS init error: " + err.Error())
			}
			defer natsclient.Close()

			serverID := fmt.Sprintf("%d", conf.ServerID)
			NodeID := fmt.Sprintf("%d", conf.NodeId)
			addr := conf.GameAddr()

			if err := cluster2.Register(serverID, NodeID, addr); err != nil {
				log.Fatal("cluster register error: " + err.Error())
			}
			fmt.Printf("game server [%s] inst=%s registered at %s\n", serverID, NodeID, addr)

			db := mongorpc.Connect(
				conf.MongoURL,
				conf.MongoUsername,
				conf.MongoPassword,
				fmt.Sprintf("game_%v", serverID),
			)

			tree.Spawn(
				play.NewPlay(),
				gate.NewGate(),
				guild.NewGuild(),
				activity.NewActivity(),
				mongorpc.NewActor(def.Mongo, db),
				gamenats.NewNats(uint32(conf.ServerID), uint32(conf.NodeId)),
			)
			tree.Default().Start()

			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err.Error())
	}
}
