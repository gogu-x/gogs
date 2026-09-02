package conn

import (
	"reflect"

	"github.com/gogu-x/gogs/pb/pb_auth"
	"github.com/gogu-x/gogs/pb/pb_gateway"
	"google.golang.org/protobuf/proto"
)

// 业务钩子触发特殊逻辑
func initHook(c *Conn) {
	c.RegHookHandle((*pb_gateway.LoginReq)(nil), c.onLogin)
	c.RegHookHandle((*pb_gateway.RegisterReq)(nil), c.onRegister)
	c.RegHookHandle((*pb_gateway.GetServerListReq)(nil), c.onGetServerList)

	c.RegHookHandle((*pb_auth.LoginGameAck)(nil), c.LoginAck)
}

func (c *Conn) RegHookHandle(msg proto.Message, handler hookHandler) {
	msgType := reflect.TypeOf(msg)
	c.hook[msgType] = handler
}
