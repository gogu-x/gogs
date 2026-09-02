package main

import (
	"context"
	"log"
	"os"

	"github.com/gogu-x/gogs/conf"
	actor "github.com/gogu-x/tree"
	"github.com/urfave/cli/v3"

	natsclient "github.com/gogu-x/gogs/natsrpc"
	_ "github.com/gogu-x/gogs/pb/pbregister"
	platformgrpc "github.com/gogu-x/gogs/platform/grpc"
	"github.com/gogu-x/gogs/platform/webhook"
	"github.com/gogu-x/gogs/rpc/mongorpc"
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

			if err := natsclient.Init(conf.NatsURL); err != nil {
				log.Fatalf("NATS init: %v", err)
			}
			defer natsclient.Close()

			db := mongorpc.Connect(
				conf.MongoURL,
				conf.MongoUsername,
				conf.MongoPassword,
				"platform",
			)

			actor.Spawn(
				natsclient.NewActor(natsclient.ActorConfig{}),
				platformgrpc.NewActor(db),
				&webhook.Actor{},
			)

			actor.Default().Start()
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err.Error())
	}
}
