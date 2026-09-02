package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	gatenats "github.com/gogu-x/gogs/gate/nats"
	"github.com/gogu-x/gogs/gate/ws"
	natsclient "github.com/gogu-x/gogs/natsrpc"
	_ "github.com/gogu-x/gogs/pb/pbregister"
	"github.com/gogu-x/gogs/rpc/platformrpc"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/cluster"
	"github.com/gogu-x/tree/log"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "gate",
		Usage: "gate server",
		Flags: conf.ConnectionFlags(),
		Action: func(ctx context.Context, c *cli.Command) error {
			if err := conf.LoadAndApply(c); err != nil {
				return err
			}

			def.NewLog(conf.LogPath, 0)
			if err := cluster.Init(conf.EtcdEndpoints); err != nil {
				log.Fatal("cluster init: " + err.Error())
			}
			defer cluster.Close()

			if err := natsclient.Init(conf.NatsURL); err != nil {
				log.Fatal("nats init: " + err.Error())
			}
			defer natsclient.Close()

			tree.Spawn(
				gatenats.NewActor(),
				ws.New(conf.GateAddr(), int64(conf.GateID)),
				platformrpc.NewActor(),
			)

			fmt.Printf("gate server [%d] starting, listen: %s\n", conf.GateID, conf.GateAddr())
			tree.Default().Start()
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err.Error())
	}
}
