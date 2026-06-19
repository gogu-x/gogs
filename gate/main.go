package main

import (
	"context"
	"fmt"
	"os"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/bigTree/log"
	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/gate/conn"
	"github.com/gogu-x/gogs/gate/constant"
	gatenats "github.com/gogu-x/gogs/gate/nats"
	"github.com/gogu-x/gogs/gate/registry"
	"github.com/gogu-x/gogs/gate/wsserver"
	natsclient "github.com/gogu-x/gogs/natsrpc"
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
				Required: true,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			config.GateID = c.Int("gate-id")

			if err := cluster.Init(config.EtcdEndpoints); err != nil {
				log.Fatal("cluster init error: " + err.Error())
			}
			defer cluster.Close()

			if err := natsclient.Init(config.NatsURL); err != nil {
				log.Fatal("nats init error: " + err.Error())
			}
			defer natsclient.Close()

			reg := &registry.Actor{}
			actor.Spawn(constant.ActorRegistry, reg)
			actor.Spawn(constant.ActorNats, gatenats.NewActor())
			actor.Spawn(constant.ActorGateServer, wsserver.New(config.GateAddr()))

			// 注入 registry 检查到 conn 路由，避免循环依赖
			conn.RegistryHasServer = reg.HasServer

			fmt.Printf("gate server [%d] starting, listen: %s\n", config.GateID, config.GateAddr())
			actor.Default().Start()
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err.Error())
	}
}
