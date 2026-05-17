package server

import (
	"fmt"
	pb "github.com/gogu-x/gogs/pb/game"
	"log"

	actor "github.com/gogu-x/bigTree"
)

func initGateRouter(c *ConnActor) {
	c.router.Register(&pb.LoginReq{}, func(ctx actor.ActorContext, msg interface{}) {
		handleLogin(c, ctx, msg.(*pb.LoginReq))
	})
}

func handleLogin(c *ConnActor, ctx actor.ActorContext, req *pb.LoginReq) {
	if req.Token == "" {
		log.Printf("ConnActor[%d]: invalid token", c.connID)
		ctx.Stop()
		return
	}
	c.uid = req.Uid
	c.serverID = fmt.Sprintf("%d", req.ServerId)

	log.Printf("ConnActor[%d]: login success uid=%d server=%s", c.connID, c.uid, c.serverID)
	c.Reply(&pb.LoginResp{Code: 0, Msg: "ok"})
}
