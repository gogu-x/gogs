package conn

import (
	"reflect"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoGateway"
)

var noAuthRequired = map[reflect.Type]bool{
	reflect.TypeOf(&protoGateway.LoginReq{}):    true,
	reflect.TypeOf(&protoGateway.RegisterReq{}): true,
}

func (c *Actor) checkAuth(_ actor.ActorContext, msg interface{}) bool {
	if noAuthRequired[reflect.TypeOf(msg)] {
		return true
	}
	if c.state == stateLogging {
		c.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_ERR_LOGIN_IN_PROGRESS, Msg: "login in progress"})
		return false
	}
	if c.state != stateAuthed {
		c.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_ERR_UNAUTHORIZED, Msg: "unauthorized"})
		return false
	}
	return true
}
