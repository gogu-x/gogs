package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/activity"
	"github.com/gogu-x/gogs/game/battle"
	"github.com/gogu-x/gogs/game/gate"
	"github.com/gogu-x/gogs/game/guild"
	"github.com/gogu-x/gogs/game/play"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/natsrpc"
	_ "github.com/gogu-x/gogs/pb"
	"github.com/gogu-x/tree/cluster"
	"github.com/gogu-x/tree/db/mongorpc"
	"github.com/gogu-x/tree/tlog"

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
			tlog.NewLog(conf.LogPath, 0)

			if err := cluster.Init(conf.EtcdEndpoints); err != nil {
				tlog.Log.Error("cluster init error: %v", err)
			}
			defer cluster.Close()

			serverID := fmt.Sprintf("%d", conf.ServerID)
			NodeID := fmt.Sprintf("%d", conf.NodeId)
			addr := conf.GameAddr()

			if err := cluster.Register(serverID, NodeID, addr); err != nil {
				tlog.Log.Error("cluster register error: %v", err)
			}
			fmt.Printf("game server [%s] inst=%s registered at %s\n", serverID, NodeID, addr)

			db := mongorpc.Connect(
				conf.MongoURL,
				conf.MongoUsername,
				conf.MongoPassword,
				fmt.Sprintf("game_%v", serverID),
			)

			//加载配置表
			err := glconf.LoadAllConfs(conf.GconfDbUri, conf.GconfDb, true)
			if err != nil {
				tlog.Log.Error("load confs error: %v", err)
				return err
			}
			tree.Spawn(
				play.NewPlay(),
				battle.New(nil, nil, conf.ServerID, conf.NodeId),
				gate.NewGate(),
				guild.NewGuild(),
				activity.NewActivity(),
				mongorpc.NewActor(def.Mongo, db),
				natsrpc.NewNats(natsrpc.Game, conf.ServerID, conf.NodeId, conf.NatsURL),
			)
			tree.Default().Start()

			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		tlog.Log.Error("%s", err)
	}
}
