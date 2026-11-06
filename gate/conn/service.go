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
	platformPID, available := ctx.Lookup(def.PF)
	if !available {
		c.state = StateAnon
		c.WriteWsMsg(&pb_gateway.LoginAck{Code: pb_common.ErrCode_INTERNAL})
		return
	}
	plReq := &pb_pf.AuthLoginReq{
		Account:  req.Account,
		Password: req.Password,
		ServerId: req.ServerId,
	}
	ctx.RequestCallback(platformPID, plReq, c.authLoginCb)
}

// authLoginCb preserves the platform result before opening a game session.
func (c *Conn) authLoginCb(ctx tree.Context, ret interface{}, err error) {
	ack, valid := ret.(*pb_pf.AuthAck)
	if err != nil || !valid || ack == nil {
		c.state = StateAnon
		tlog.Log.Warn("ConnActor[%d]: uid=%d AuthLoginReq err=%v response=%T", c.connID, c.uid, err, ret)
		c.WriteWsMsg(&pb_gateway.LoginAck{Code: pb_common.ErrCode_INTERNAL})
		return
	}
	if ack.GetCode() != pb_pf.ErrCode_OK {
		c.state = StateAnon
		c.WriteWsMsg(&pb_gateway.LoginAck{Code: platformCode(ack.GetCode())})
		return
	}
	c.uid = ack.GetUid()
	c.token = ack.GetToken()
	c.serverID = ack.GetServerId()
	c.nodeID = ""
	c.state = StateLoggIng
	//进入game gate 验证
	req := &pb_auth.LoginGameReq{
		UID:      ack.GetUid(),
		ServerID: ack.GetServerId(),
	}
	// 起来game gate流
	err = c.OpenSteam(ctx, req)
	if err != nil {
		c.state = StateAnon
		tlog.Log.Info("ConnActor[%d]: open game stream: %v", c.connID, err)
		c.WriteWsMsg(&pb_gateway.LoginAck{Code: pb_common.ErrCode_INTERNAL})
		return
	}
}

// LoginAck 游戏登录回调信息
func (c *Conn) LoginAck(_ tree.Context, msg interface{}) {
	req := msg.(*pb_auth.LoginGameAck)
	if req.GetCode() != pb_common.ErrCode_OK {
		c.state = StateAnon
		tlog.Log.Warn("LoginAck code err: %d", req.GetCode())
		c.WriteWsMsg(&pb_gateway.LoginAck{Code: req.GetCode()})
		return
	}
	c.state = StateAuthed
	c.uid = req.GetUid()
	tlog.Log.Info("LoginAck succeeded: uid=%d", c.uid)
	c.WriteWsMsg(&pb_gateway.LoginAck{Code: pb_common.ErrCode_OK})
}

func (c *Conn) onRegister(ctx tree.Context, msg interface{}) {
	req := msg.(*pb_gateway.RegisterReq)
	c.state = StateRegIng
	platformPID, available := ctx.Lookup(def.PF)
	if !available {
		c.state = StateAnon
		c.WriteWsMsg(&pb_gateway.RegisterAck{Code: pb_common.ErrCode_INTERNAL})
		return
	}
	plReq := &pb_pf.RegisterReq{
		Account:  req.Account,
		Password: req.Password,
		ServerId: req.ServerId,
	}
	ctx.RequestCallback(platformPID, plReq, c.regCb)
}

func (c *Conn) regCb(ctx tree.Context, ret interface{}, err error) {
	c.state = StateAnon
	ack, valid := ret.(*pb_pf.AuthAck)
	if err != nil || !valid || ack == nil {
		tlog.Log.Warn("ConnActor[%d]: regCb: %v", c.connID, err)
		c.WriteWsMsg(&pb_gateway.RegisterAck{Code: pb_common.ErrCode_INTERNAL})
		return
	}
	if ack.GetCode() != pb_pf.ErrCode_OK {
		c.WriteWsMsg(&pb_gateway.RegisterAck{Code: platformCode(ack.GetCode())})
		return
	}
	c.uid = ack.GetUid()
	c.token = ack.GetToken()
	c.serverID = ack.GetServerId()
	c.nodeID = ""
	tlog.Log.Info("ConnActor[%d]: uid=%d registered -> server=%d node=%s", c.connID, c.uid, c.serverID, c.nodeID)
	c.WriteWsMsg(&pb_gateway.RegisterAck{Code: pb_common.ErrCode_OK})
}

func (c *Conn) onGetServerList(ctx tree.Context, msg interface{}) {
	req := msg.(*pb_gateway.GetServerListReq)
	platformPID, available := ctx.Lookup(def.PF)
	if !available {
		c.WriteWsMsg(&pb_gateway.ServerListAck{Code: pb_common.ErrCode_INTERNAL})
		return
	}
	ctx.RequestCallback(platformPID, &pb_pf.GetServerListReq{Account: req.Account}, c.getServerListCb)
}

func (c *Conn) getServerListCb(_ tree.Context, ret interface{}, err error) {
	resp, valid := ret.(*pb_pf.ServerListAck)
	if err != nil || !valid || resp == nil {
		tlog.Log.Warn("ConnActor[%d]: getServerListCb: %v", c.connID, err)
		c.WriteWsMsg(&pb_gateway.ServerListAck{Code: pb_common.ErrCode_INTERNAL})
		return
	}
	if resp.GetCode() != pb_pf.ErrCode_OK {
		c.WriteWsMsg(&pb_gateway.ServerListAck{Code: platformCode(resp.GetCode())})
		return
	}
	accounts := make([]*pb_gateway.ServerAccount, 0, len(resp.Accounts))
	for _, account := range resp.Accounts {
		accounts = append(accounts, &pb_gateway.ServerAccount{ServerId: account.ServerId, Uid: account.Uid})
	}
	tlog.Log.Info("ConnActor[%d]: getServerListCb: accounts=%v", c.connID, accounts)
	c.WriteWsMsg(&pb_gateway.ServerListAck{Code: pb_common.ErrCode_OK, Accounts: accounts})
}

// platformCode 将平台错误码转换为客户端错误码。
func platformCode(code pb_pf.ErrCode) pb_common.ErrCode {
	switch code {
	case pb_pf.ErrCode_OK, pb_pf.ErrCode_UNKNOWN, pb_pf.ErrCode_PARAM, pb_pf.ErrCode_UNAUTHORIZED, pb_pf.ErrCode_LOGIN_IN_PROGRESS, pb_pf.ErrCode_SERVER_NOT_FOUND, pb_pf.ErrCode_USERNAME_EXISTS, pb_pf.ErrCode_WRONG_PASSWORD, pb_pf.ErrCode_INTERNAL, pb_pf.ErrCode_ACCOUNT_NOT_FOUND, pb_pf.ErrCode_GUILD_EXISTS, pb_pf.ErrCode_GUILD_NOT_FOUND, pb_pf.ErrCode_ALREADY_IN_GUILD, pb_pf.ErrCode_NOT_IN_GUILD:
		return code
	default:
		return pb_common.ErrCode_INTERNAL
	}
}
