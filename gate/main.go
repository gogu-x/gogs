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
	"github.com/gogu-x/gogs/gate/conn"
	gateconstant "github.com/gogu-x/gogs/gate/constant"
	gatenats "github.com/gogu-x/gogs/gate/nats"
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

			// 启动时从 etcd 加载所有节点，初始化 hash 路由缓存
			if all, err := cluster.GetAll(); err != nil {
				log.Fatal("cluster.GetAll: " + err.Error())
			} else {
				for serverID := range all {
					instances, _ := cluster.GetInstances(serverID)
					cluster.UpdateNodes(serverID, instances)
				}
			}

			// 监听 etcd 节点变化，动态维护 hash 路由缓存
			watchCtx, watchCancel := context.WithCancel(context.Background())
			defer watchCancel()
			go func() {
				for ev := range cluster.WatchInstances(watchCtx) {
					instances, _ := cluster.GetInstances(ev.ServerID)
					cluster.UpdateNodes(ev.ServerID, instances)
					// 节点下线：通知 GateServer 广播 failover，让受影响连接无感切换
					if ev.Type == "delete" {
						if pid, ok := actor.Lookup(gateconstant.ActorGateServer); ok {
							actor.Send(pid, &conn.NodeFailoverMsg{
								ServerID:   ev.ServerID,
								DeadNodeID: ev.NodeID,
							})
						}
					}
				}
			}()

			if err := natsclient.Init(config.NatsURL); err != nil {
				log.Fatal("nats init: " + err.Error())
			}
			defer natsclient.Close()

			//actor.Spawn(gateconstant.ActorRegistry, &registry.Actor{})
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
