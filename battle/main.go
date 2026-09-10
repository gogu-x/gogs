package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gogu-x/gogs/battle/service"
	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/natsrpc"
	_ "github.com/gogu-x/gogs/pb"
	"github.com/gogu-x/gogs/rpc/mongorpc"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/cluster"
	"github.com/gogu-x/tree/tlog"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "battle",
		Usage: "deterministic battle service",
		Flags: conf.ConnectionFlags(),
		Action: func(_ context.Context, c *cli.Command) error {
			if err := conf.LoadAndApply(c); err != nil {
				return err
			}
			tlog.NewLog(conf.LogPath, 0)

			if err := cluster.Init(conf.EtcdEndpoints); err != nil {
				tlog.Log.Error("battle cluster init: %v", err)
			} else {
				defer cluster.Close()
				if err := cluster.RegisterService(def.ServiceBattle, fmt.Sprint(conf.ServerID), fmt.Sprint(conf.NodeId), conf.BattleAddr()); err != nil {
					tlog.Log.Error("battle service register: %v", err)
				}
			}

			db := mongorpc.Connect(conf.MongoURL, conf.MongoUsername, conf.MongoPassword, "battle")

			//加载配置表
			err := glconf.LoadAllConfs(conf.GconfDbUri, conf.GconfDb, true)
			if err != nil {
				tlog.Log.Error("load confs error: %v", err)
				return err
			}

			tree.Spawn(
				mongorpc.NewActor(def.Mongo, db),
				service.New(),
				natsrpc.NewNats(natsrpc.Battle, conf.ServerID, conf.NodeId, conf.NatsURL),
			)
			fmt.Printf("battle service [%d/%d] registered at %s\n", conf.ServerID, conf.NodeId, conf.BattleAddr())
			tree.Default().Start()
			return nil
		},
	}
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		tlog.Log.Error("%s", err)
	}
}
