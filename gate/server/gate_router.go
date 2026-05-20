package server

import (
	"fmt"
	"log"

	"github.com/gogu-x/gogs/pb/gateway"

	actor "github.com/gogu-x/bigTree"
)

func initGateRouter(c *ConnActor) {
	c.router.Register(&gateway.LoginReq{}, func(ctx actor.ActorContext, msg interface{}) {
		handleLogin(c, ctx, msg.(*gateway.LoginReq))
	})
}

func handleLogin(c *ConnActor, ctx actor.ActorContext, req *gateway.LoginReq) {
	if req.Token == "" {
		log.Printf("ConnActor[%d]: invalid token", c.connID)
		ctx.Stop()
		return
	}
	c.uid = req.Uid
	c.serverID = fmt.Sprintf("%d", req.ServerId)

	log.Printf("ConnActor[%d]: login success uid=%d server=%s", c.connID, c.uid, c.serverID)
	c.Reply(&gateway.LoginResp{Code: 0, Msg: "ok"})
}
