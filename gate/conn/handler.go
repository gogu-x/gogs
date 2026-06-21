package conn

import (
	"fmt"
	"log"
	"reflect"
	"time"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/gate/registry"
	"github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/gogs/pb/protoPlatform"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func (c *Actor) handleWsMsg(ctx actor.ActorContext, data []byte) {
	inner, err := c.codec.Unmarshal(data)
	if err != nil {
		log.Printf("ConnActor[%d]: unmarshal error: %v", c.connID, err)
		return
	}
	for _, mw := range c.middlewares {
		if !mw(ctx, inner) {
			return
		}
	}
	c.router.SetFallback(func(ctx actor.ActorContext, _ interface{}) {
		c.forward(ctx, inner)
	})
	c.router.Route(ctx, inner)
}

func (c *Actor) forward(ctx actor.ActorContext, inner interface{}) {
	protoMsg, ok := inner.(proto.Message)
	if !ok {
		return
	}
	body, _ := c.codec.Marshal(protoMsg)
	if c.serverID == "" {
		return
	}
	ctx.Send(actor.MustLookup(constant.ActorNats), &natsrpc.SendMsg{
		Module: natsrpc.ModuleGame,
		NodeID: c.serverID,
		Frame: &protoGateway.Frame{
			ConnId:   c.connID,
			Uid:      c.uid,
			ServerId: c.serverID,
			GateId:   fmt.Sprintf("%d", config.GateID),
			Payload:  body,
			MsgType:  reflect.TypeOf(inner).Elem().Name(),
		},
	})
}

func (c *Actor) Reply(msg proto.Message) {
	data, err := c.codec.Marshal(msg)
	if err != nil {
		return
	}
	_ = c.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *Actor) onLogin(ctx actor.ActorContext, msg interface{}) {
	req := msg.(*protoGateway.LoginReq)
	serverID := fmt.Sprintf("%d", req.ServerId)
	if !registry.Global.HasServer(serverID) {
		c.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_ERR_SERVER_NOT_FOUND, Msg: "server not available"})
		return
	}
	c.state = stateLogging
	platformPID := actor.MustLookup(constant.ActorRpcPlatform)
	ctx.Request(platformPID, &protoPlatform.AuthLoginReq{Account: req.Account, Password: req.Password, ServerId: req.ServerId}).
		Callback(ctx, func(ret interface{}, err error) {
			if err != nil {
				c.state = stateAnon
				c.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_ERR_UNKNOWN, Msg: err.Error()})
				return
			}
			AuthAck := ret.(*protoPlatform.AuthAck)
			c.uid = AuthAck.Uid
			c.token = AuthAck.Token
			c.serverID = serverID
			c.state = stateAuthed
			c.forward(ctx, req)
			log.Printf("ConnActor[%d]: uid=%d logged in -> server %s", c.connID, c.uid, c.serverID)
			c.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
		})
}

func (c *Actor) onRegister(ctx actor.ActorContext, msg interface{}) {
	req := msg.(*protoGateway.RegisterReq)
	serverID := fmt.Sprintf("%d", req.ServerId)
	if !registry.Global.HasServer(serverID) {
		c.Reply(&protoGateway.RegisterAck{Code: protoCommon.ErrCode_ERR_SERVER_NOT_FOUND, Msg: "server not available"})
		return
	}
	c.state = stateLogging
	start := time.Now()
	log.Printf("ConnActor[%d]: register start", c.connID)
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
			c.uid = AuthAck.Uid
			c.token = AuthAck.Token
			c.serverID = serverID
			c.state = stateAuthed
			c.forward(ctx, req)
			log.Printf("ConnActor[%d]: uid=%d registered -> server %s", c.connID, c.uid, c.serverID)
			c.Reply(&protoGateway.RegisterAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
		})
}
