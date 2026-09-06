package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/gate/ws"
	"github.com/gogu-x/gogs/natsrpc"

	_ "github.com/gogu-x/gogs/pb"
	"github.com/gogu-x/gogs/rpc/platformrpc"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/cluster"
	"github.com/gogu-x/tree/tlog"
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

			tlog.NewLog(conf.LogPath, 0)
			if err := cluster.Init(conf.EtcdEndpoints); err != nil {
				tlog.Log.Error("cluster init: %v", err)
			}
			defer cluster.Close()

			tree.Spawn(
				ws.New(conf.GateAddr(), int64(conf.GateID)),
				platformrpc.NewActor(),
				natsrpc.NewNats(natsrpc.Gate, conf.GateID, conf.NodeId, conf.NatsURL),
			)

			fmt.Printf("gate server [%d] starting, listen: %s\n", conf.GateID, conf.GateAddr())
			tree.Default().Start()
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		tlog.Log.Error("%s", err)
	}
}
