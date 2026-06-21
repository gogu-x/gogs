package main

import (
	"context"
	"fmt"
	"os"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/bigTree/log"
	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/constant"
	gateconstant "github.com/gogu-x/gogs/gate/constant"
	gatenats "github.com/gogu-x/gogs/gate/nats"
	"github.com/gogu-x/gogs/gate/registry"
	"github.com/gogu-x/gogs/gate/wsserver"
	natsclient "github.com/gogu-x/gogs/natsrpc"
	_ "github.com/gogu-x/gogs/pb/pbregister"
	rpcplatform "github.com/gogu-x/gogs/rpc/platform"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "gate",
		Usage: "gate server",
		Flags: []cli.Flag{
			&cli.IntFlag{Name: "gate-id", Aliases: []string{"id"}, Required: true},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			config.GateID = c.Int("gate-id")

			if err := cluster.Init(config.EtcdEndpoints); err != nil {
				log.Fatal("cluster init: " + err.Error())
			}
			defer cluster.Close()

			if err := natsclient.Init(config.NatsURL); err != nil {
				log.Fatal("nats init: " + err.Error())
			}
			defer natsclient.Close()

			actor.Spawn(gateconstant.ActorRegistry, &registry.Actor{})
			actor.Spawn(gateconstant.ActorNats, gatenats.NewActor())
			actor.Spawn(gateconstant.ActorGateServer, wsserver.New(config.GateAddr()))
			actor.Spawn(constant.ActorRpcPlatform, rpcplatform.NewActor())

			fmt.Printf("gate server [%d] starting, listen: %s\n", config.GateID, config.GateAddr())
			actor.Default().Start()
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err.Error())
	}
}
