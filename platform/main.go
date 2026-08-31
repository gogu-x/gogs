package main

import (
	"context"
	"log"
	"os"

	actor "github.com/gogu-x/tree"
	"github.com/urfave/cli/v3"

	"github.com/gogu-x/gogs/config"
	natsclient "github.com/gogu-x/gogs/natsrpc"
	_ "github.com/gogu-x/gogs/pb/pbregister"
	platformgrpc "github.com/gogu-x/gogs/platform/grpc"
	"github.com/gogu-x/gogs/platform/webhook"
	rpcmongo "github.com/gogu-x/gogs/rpc/mongo"
)

func main() {
	cmd := &cli.Command{
		Name:  "platform",
		Usage: "platform server",
		Flags: config.ConnectionFlags(),
		Action: func(ctx context.Context, c *cli.Command) error {
			if err := config.LoadAndApply(c); err != nil {
				return err
			}

			if err := natsclient.Init(config.NatsURL); err != nil {
				log.Fatalf("NATS init: %v", err)
			}
			defer natsclient.Close()

			db := rpcmongo.Connect(
				config.MongoURL,
				config.MongoUsername,
				config.MongoPassword,
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
