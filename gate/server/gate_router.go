package server

import (
	"fmt"
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/pb/protoGateway"
)

func initGateRouter(c *ConnActor) {
	c.router.Register(&protoGateway.LoginReq{}, func(ctx actor.ActorContext, msg interface{}) {
		req := msg.(*protoGateway.LoginReq)
		serverID := fmt.Sprintf("%d", req.ServerId)

		// 验证该区服是否在线
		if Registry == nil || !Registry.HasServer(serverID) {
			log.Printf("ConnActor[%d]: login failed, server %s not available", c.connID, serverID)
			c.Reply(&protoGateway.LoginReq{})
			return
		}

		// TODO: 验证 token（req.Token）

		c.uid = req.Uid
		c.serverID = serverID

		c.forward(ctx, req)
		log.Printf("ConnActor[%d]: uid=%d logged in -> server %s", c.connID, c.uid, c.serverID)
	})
}
