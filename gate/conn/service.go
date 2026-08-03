package conn

import (
	"log"

	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/gogs/pb/protoPlatform"
	"github.com/gogu-x/tree"
)

func (c *Conn) onLogin(ctx tree.Context, msg interface{}) {
	req := msg.(*protoGateway.LoginReq)
	c.state = stateLogging
	platformPID := tree.MustLookup(constant.PF)
	plReq := &protoPlatform.AuthLoginReq{
		Account:  req.Account,
		Password: req.Password,
		ServerId: req.ServerId,
	}
	ctx.RequestCallback(platformPID, plReq, c.LoginCb)
}

func (c *Conn) LoginCb(ctx tree.Context, ret interface{}, err error) {
	if err != nil {
		c.state = stateAnon
		log.Printf("ConnActor[%d]: uid=%d AuthLoginReq err: %s", c.connID, c.uid, err.Error())
		c.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_ERR_UNKNOWN, Msg: err.Error()})
		return
	}
	AuthAck := ret.(*protoPlatform.AuthAck)
	if AuthAck.Code != protoCommon.ErrCode_OK {
		return
	}
	c.uid = AuthAck.Uid
	c.token = AuthAck.Token
	c.serverID = AuthAck.ServerId
	c.nodeID = ""
	c.state = stateAuthed
	req := &protoGateway.LoginGameReq{
		Uid:      AuthAck.Uid,
		ServerId: AuthAck.ServerId,
	}
	c.forward(ctx, req)
	log.Printf("ConnActor[%d]: uid=%d logged in -> server=%s node=%s", c.connID, c.uid, c.serverID, c.nodeID)
	c.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
}

func (c *Conn) onRegister(ctx tree.Context, msg interface{}) {
	req := msg.(*protoGateway.RegisterReq)
	c.state = stateLogging
	platformPID := tree.MustLookup(constant.PF)
	plReq := &protoPlatform.RegisterReq{
		Account:  req.Account,
		Password: req.Password,
		ServerId: req.ServerId,
	}
	ctx.RequestCallback(platformPID, plReq, c.regCb)
}

func (c *Conn) regCb(ctx tree.Context, ret interface{}, err error) {

	if err != nil {
		c.state = stateAnon
		c.Reply(&protoGateway.RegisterAck{Code: protoCommon.ErrCode_ERR_UNKNOWN, Msg: err.Error()})
		return
	}

	AuthAck := ret.(*protoPlatform.AuthAck)
	c.uid = AuthAck.Uid
	c.token = AuthAck.Token
	c.serverID = AuthAck.ServerId
	c.nodeID = ""
	c.state = stateAuthed
	log.Printf("ConnActor[%d]: uid=%d registered -> server=%s node=%s", c.connID, c.uid, c.serverID, c.nodeID)
	c.Reply(&protoGateway.RegisterAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
}
