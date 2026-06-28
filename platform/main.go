package main

import (
	"context"
	"log"
	"os"

	actor "github.com/gogu-x/bigTree"
	"github.com/urfave/cli/v3"

	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/constant"
	rpcmongo "github.com/gogu-x/gogs/rpc/mongo"
	natsclient "github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/gogs/pb/protoPlatform"
	platformgrpc "github.com/gogu-x/gogs/platform/grpc"
	"github.com/gogu-x/gogs/platform/webhook"
)

func main() {
	cmd := &cli.Command{
		Name:  "platform",
		Usage: "platform server",
		Action: func(ctx context.Context, c *cli.Command) error {
			protoPlatform.RegisterJSONCodec()

			if err := natsclient.Init(config.NatsURL); err != nil {
				log.Fatalf("NATS init: %v", err)
			}
			defer natsclient.Close()

			db := rpcmongo.Connect(config.MongoURL, "platform")

			actor.Spawn(constant.ActorPlatformGrpc, platformgrpc.NewActor(db))
			actor.Spawn(constant.ActorPlatformWebhook, &webhook.Actor{})

			actor.Default().Start()
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err.Error())
	}
}
