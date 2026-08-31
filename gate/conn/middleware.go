package conn

import (
	"reflect"

	"github.com/gogu-x/gogs/pb/pb_common"
	"github.com/gogu-x/gogs/pb/pb_gateway"
	actor "github.com/gogu-x/tree"
)

var noAuthRequired = map[reflect.Type]bool{
	reflect.TypeOf(&pb_gateway.LoginReq{}):         true,
	reflect.TypeOf(&pb_gateway.RegisterReq{}):      true,
	reflect.TypeOf(&pb_gateway.GetServerListReq{}): true,
}

func (c *Conn) checkAuth(_ actor.Context, msg interface{}) bool {
	if noAuthRequired[reflect.TypeOf(msg)] {
		return true
	}
	if c.state == StateLoggIng {
		c.Reply(&pb_gateway.LoginAck{Code: pb_common.ErrCode_ERR_LOGIN_IN_PROGRESS, Msg: "login in progress"})
		return false
	}
	if c.state != StateAuthed {
		c.Reply(&pb_gateway.LoginAck{Code: pb_common.ErrCode_ERR_UNAUTHORIZED, Msg: "unauthorized"})
		return false
	}
	return true
}
