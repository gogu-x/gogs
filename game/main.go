package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gogu-x/bigTree/log"
	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/game/constant"
	"github.com/gogu-x/gogs/game/model"
	natsclient "github.com/gogu-x/gogs/nats"

	actor "github.com/gogu-x/bigTree"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "game",
		Usage: "game server",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:     "server-id",
				Aliases:  []string{"id"},
				Usage:    "server ID (unique per game server group, e.g. 1, 2, 3)",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "port",
				Usage: "gRPC listen port override (default: 9000+serverID)",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			config.ServerID = c.Int("server-id")
			if p := c.String("port"); p != "" {
				config.GrpcPortOverride = p
			}

			if err := cluster.Init(config.EtcdEndpoints); err != nil {
				log.Fatal("cluster init error: " + err.Error())
			}
			defer cluster.Close()

			if err := natsclient.Init(config.NatsURL); err != nil {
				log.Fatal("NATS init error: " + err.Error())
			}
			defer natsclient.Close()

			serverID := fmt.Sprintf("%d", config.ServerID)
			instID := strconv.Itoa(os.Getpid())
			addr := config.GameAddr()

			if err := cluster.Register(serverID, instID, addr); err != nil {
				log.Fatal("cluster register error: " + err.Error())
			}
			fmt.Printf("game server [%s] inst=%s registered at %s\n", serverID, instID, addr)

			actor.Spawn(constant.ActorSupervisor, &model.NatsActor{})
			actor.Spawn(constant.ActorGuild, model.NewGuildActor())
			actor.Spawn(constant.ActorActivity, model.NewActivityActor())

			// 系统 actor 数量（非 PlayerActor）：supervisor + guild + activity = 3
			const systemActorCount = 3

			quit := make(chan os.Signal, 1)
			signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
			go actor.Default().Start()
			<-quit

			fmt.Printf("game [%s] inst=%s: received signal, shutting down...\n", serverID, instID)

			// 1. 取消订阅，停止接收新消息
			if pid, ok := actor.Lookup(constant.ActorSupervisor); ok {
				actor.Send(pid, &model.DrainMsg{})
			}

			// 2. 等待所有 PlayerActor 退出（超时 30s）
			deadline := time.After(30 * time.Second)
		waitLoop:
			for {
				select {
				case <-deadline:
					fmt.Println("game: drain timeout, forcing shutdown")
					break waitLoop
				case <-time.After(200 * time.Millisecond):
					if actor.Default().SpawnCount() <= systemActorCount {
						fmt.Println("game: all players saved")
						break waitLoop
					}
				}
			}

			// 3. 有活跃玩家时写 drain 标记，通知 gate 开始缓冲并切换
			if actor.Default().SpawnCount() > systemActorCount {
				if err := cluster.SetDrain(serverID, instID); err != nil {
					fmt.Printf("game: SetDrain error: %v\n", err)
				}
			}

			// 4. 从 etcd 注销，gate 感知 delete 后切换到新实例
			if err := cluster.Deregister(serverID, instID); err != nil {
				fmt.Printf("game: Deregister error: %v\n", err)
			}
			fmt.Printf("game [%s] inst=%s: deregistered\n", serverID, instID)

			// 5. 关闭 actor 系统
			actor.Default().Shutdown()
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err.Error())
	}
}
