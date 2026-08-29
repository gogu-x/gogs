package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gogu-x/gogs/config"
	gatenats "github.com/gogu-x/gogs/gate/nats"
	"github.com/gogu-x/gogs/gate/ws"
	natsclient "github.com/gogu-x/gogs/natsrpc"
	_ "github.com/gogu-x/gogs/pb/pbregister"
	rpcplatform "github.com/gogu-x/gogs/rpc/platform"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/cluster"
	"github.com/gogu-x/tree/log"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "gate",
		Usage: "gate server",
		Flags: config.ConnectionFlags(),
		Action: func(ctx context.Context, c *cli.Command) error {
			if err := config.LoadAndApply(c); err != nil {
				return err
			}
			if err := cluster.Init(config.EtcdEndpoints); err != nil {
				log.Fatal("cluster init: " + err.Error())
			}
			defer cluster.Close()

			if err := natsclient.Init(config.NatsURL); err != nil {
				log.Fatal("nats init: " + err.Error())
			}
			defer natsclient.Close()

			tree.Spawn(
				gatenats.NewActor(),
				ws.New(config.GateAddr(), int64(config.GateID)),
				rpcplatform.NewActor(),
			)

			fmt.Printf("gate server [%d] starting, listen: %s\n", config.GateID, config.GateAddr())
			tree.Default().Start()
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err.Error())
	}
}
