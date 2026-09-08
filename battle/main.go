package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gogu-x/gogs/battle/service"
	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
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
			configs, err := service.LoadConfigRepository(conf.BattleConfigPath)
			if err != nil {
				return err
			}
			if err := cluster.Init(conf.EtcdEndpoints); err != nil {
				tlog.Log.Error("battle cluster init: %v", err)
			} else {
				defer cluster.Close()
				if err := cluster.RegisterService(def.ServiceBattle, fmt.Sprint(conf.ServerID), fmt.Sprint(conf.NodeId), conf.BattleAddr()); err != nil {
					tlog.Log.Error("battle service register: %v", err)
				}
			}

			db := mongorpc.Connect(conf.MongoURL, conf.MongoUsername, conf.MongoPassword, "battle")
			reports := service.NewMongoRepository(db, "battle_reports")
			tree.Spawn(
				service.New(configs, reports, service.NATSNotifier{}, service.Options{Workers: conf.BattleWorkers, QueueSize: conf.BattleQueueSize, RetryAttempts: conf.BattleRetryCount, RetryDelay: time.Duration(conf.BattleRetryDelay) * time.Millisecond}),
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
