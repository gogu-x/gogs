package conn

import (
	"fmt"
	"log"
	"time"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/gogs/pb/protoPlatform"
)

func (c *Actor) onLogin(ctx actor.ActorContext, msg interface{}) {
	req := msg.(*protoGateway.LoginReq)
	serverID := fmt.Sprintf("%d", req.ServerId)
	c.state = stateLogging
	platformPID := actor.MustLookup(constant.ActorRpcPlatform)
	ctx.Request(platformPID, &protoPlatform.AuthLoginReq{Account: req.Account, Password: req.Password, ServerId: req.ServerId}).
		Callback(ctx, func(ret interface{}, err error) {
			if err != nil {
				c.state = stateAnon
				log.Printf("ConnActor[%d]: uid=%d AuthLoginReq err: %s", c.connID, c.uid, err.Error())
				c.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_ERR_UNKNOWN, Msg: err.Error()})
				return
			}
			AuthAck := ret.(*protoPlatform.AuthAck)
			inst, ok := cluster.HashPick(serverID, AuthAck.Uid)
			if !ok {
				c.state = stateAnon
				c.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_ERR_SERVER_NOT_FOUND, Msg: "server not available"})
				return
			}
			c.uid = AuthAck.Uid
			c.token = AuthAck.Token
			c.serverID = serverID
			c.nodeID = inst.NodeID
			c.state = stateAuthed
			c.forward(ctx, req)
			log.Printf("ConnActor[%d]: uid=%d logged in -> server=%s node=%s", c.connID, c.uid, c.serverID, c.nodeID)
			c.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
		})
}

func (c *Actor) onRegister(ctx actor.ActorContext, msg interface{}) {
	req := msg.(*protoGateway.RegisterReq)
	serverID := fmt.Sprintf("%d", req.ServerId)
	c.state = stateLogging
	start := time.Now()
	platformPID := actor.MustLookup(constant.ActorRpcPlatform)
	ctx.Request(platformPID, &protoPlatform.RegisterReq{Account: req.Account, Password: req.Password, ServerId: req.ServerId}).
		Callback(ctx, func(ret interface{}, err error) {
			log.Printf("ConnActor[%d]: register platform cb elapsed=%v err=%v", c.connID, time.Since(start), err)
			if err != nil {
				c.state = stateAnon
				c.Reply(&protoGateway.RegisterAck{Code: protoCommon.ErrCode_ERR_UNKNOWN, Msg: err.Error()})
				return
			}
			AuthAck := ret.(*protoPlatform.AuthAck)
			inst, ok := cluster.HashPick(serverID, AuthAck.Uid)
			if !ok {
				c.state = stateAnon
				c.Reply(&protoGateway.RegisterAck{Code: protoCommon.ErrCode_ERR_SERVER_NOT_FOUND, Msg: "server not available"})
				return
			}
			c.uid = AuthAck.Uid
			c.token = AuthAck.Token
			c.serverID = serverID
			c.nodeID = inst.NodeID
			c.state = stateAuthed
			c.forward(ctx, req)
			log.Printf("ConnActor[%d]: uid=%d registered -> server=%s node=%s", c.connID, c.uid, c.serverID, c.nodeID)
			c.Reply(&protoGateway.RegisterAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
		})
}
