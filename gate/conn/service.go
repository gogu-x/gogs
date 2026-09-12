package conn

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/gogs/pb/pfpb/pb_pf"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

// onLogin 请求平台验证登录
func (c *Conn) onLogin(ctx tree.Context, msg interface{}) {
	req := msg.(*pb_gateway.LoginReq)
	platformPID := tree.MustLookup(def.PF)
	plReq := &pb_pf.AuthLoginReq{
		Account:  req.Account,
		Password: req.Password,
		ServerId: req.ServerId,
	}
	ctx.RequestCallback(platformPID, plReq, c.authLoginCb)
}

// LoginCb 平台回调
func (c *Conn) authLoginCb(ctx tree.Context, ret interface{}, err error) {
	if err != nil {
		c.state = StateAnon
		tlog.Log.Info("ConnActor[%d]: uid=%d AuthLoginReq err: %s", c.connID, c.uid, err.Error())
		c.WriteWsMsg(&pb_gateway.LoginAck{Code: pb_common.ErrCode_UNKNOWN, Msg: err.Error()})
		return
	}
	AuthAck := ret.(*pb_pf.AuthAck)
	if AuthAck.Code != 0 {
		return
	}
	c.uid = AuthAck.Uid
	c.token = AuthAck.Token
	c.serverID = AuthAck.ServerId
	c.nodeID = ""
	c.state = StateLoggIng
	//进入game gate 验证
	req := &pb_auth.LoginGameReq{
		UID:      AuthAck.Uid,
		ServerID: AuthAck.ServerId,
	}
	// 起来game gate流
	if err := c.OpenSteam(ctx.Self()); err != nil {
		c.state = StateAnon
		tlog.Log.Info("ConnActor[%d]: open game stream: %v", c.connID, err)
		c.WriteWsMsg(&pb_gateway.LoginAck{Code: pb_common.ErrCode_UNKNOWN, Msg: "game server unavailable"})
		return
	}
	c.sendGameSteam(ctx, req)
	tlog.Log.Info("ConnActor[%d]: uid=%d login forwarded -> server=%d node=%s", c.connID, c.uid, c.serverID, c.nodeID)
}

// LoginAck 游戏登录回调信息
func (c *Conn) LoginAck(_ tree.Context, msg interface{}) {
	req := msg.(*pb_auth.LoginGameAck)
	if req.GetCode() != pb_common.ErrCode_OK {
		c.WriteWsMsg(&pb_gateway.LoginAck{Code: pb_common.ErrCode_LOGIN_IN_PROGRESS})
		return
	}
	c.state = StateAuthed
	c.uid = req.GetUid()
	c.WriteWsMsg(&pb_gateway.LoginAck{Code: pb_common.ErrCode_OK})
}

func (c *Conn) onRegister(ctx tree.Context, msg interface{}) {
	req := msg.(*pb_gateway.RegisterReq)
	c.state = StateRegIng
	platformPID := tree.MustLookup(def.PF)
	plReq := &pb_pf.RegisterReq{
		Account:  req.Account,
		Password: req.Password,
		ServerId: req.ServerId,
	}
	ctx.RequestCallback(platformPID, plReq, c.regCb)
}

func (c *Conn) regCb(ctx tree.Context, ret interface{}, err error) {

	if err != nil {
		c.state = StateAnon
		c.WriteWsMsg(&pb_gateway.RegisterAck{Code: pb_common.ErrCode_UNKNOWN, Msg: err.Error()})
		return
	}

	AuthAck := ret.(*pb_pf.AuthAck)
	c.uid = AuthAck.Uid
	c.token = AuthAck.Token
	c.serverID = AuthAck.ServerId
	c.nodeID = ""
	tlog.Log.Info("ConnActor[%d]: uid=%d registered -> server=%d node=%s", c.connID, c.uid, c.serverID, c.nodeID)
	c.WriteWsMsg(&pb_gateway.RegisterAck{Code: pb_common.ErrCode_OK, Msg: "ok"})
}

func (c *Conn) onGetServerList(ctx tree.Context, msg interface{}) {
	req := msg.(*pb_gateway.GetServerListReq)
	platformPID := tree.MustLookup(def.PF)
	ctx.RequestCallback(platformPID, &pb_pf.GetServerListReq{Account: req.Account}, c.getServerListCb)
}

func (c *Conn) getServerListCb(_ tree.Context, ret interface{}, err error) {
	if err != nil {
		c.WriteWsMsg(&pb_gateway.ServerListAck{Code: pb_common.ErrCode_UNKNOWN})
		return
	}

	resp := ret.(*pb_pf.ServerListAck)
	accounts := make([]*pb_gateway.ServerAccount, 0, len(resp.Accounts))
	for _, account := range resp.Accounts {
		accounts = append(accounts, &pb_gateway.ServerAccount{ServerId: account.ServerId, Uid: account.Uid})
	}
	c.WriteWsMsg(&pb_gateway.ServerListAck{Code: pb_common.ErrCode_OK, Accounts: accounts})
}
