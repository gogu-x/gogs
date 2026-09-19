package main

import (
	"context"
	"log"
	"os"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/db/mongorpc"
	"github.com/gogu-x/tree/tlog"
	"github.com/urfave/cli/v3"

	"github.com/gogu-x/gogs/natsrpc"
	_ "github.com/gogu-x/gogs/pb"
	platformgrpc "github.com/gogu-x/gogs/platform/grpc"
	"github.com/gogu-x/gogs/platform/webhook"
)

func main() {
	cmd := &cli.Command{
		Name:  "platform",
		Usage: "platform server",
		Flags: conf.ConnectionFlags(),
		Action: func(ctx context.Context, c *cli.Command) error {
			if err := conf.LoadAndApply(c); err != nil {
				return err
			}

			tlog.NewLog(conf.LogPath, 0)

			db := mongorpc.Connect(
				conf.MongoURL,
				conf.MongoUsername,
				conf.MongoPassword,
				"platform",
			)

			tree.Spawn(
				platformgrpc.NewActor(db),
				&webhook.Actor{},
				natsrpc.NewNats(natsrpc.PF, conf.ServerID, conf.NodeId, conf.NatsURL),
			)

			tree.Default().Start()
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err.Error())
	}
}
