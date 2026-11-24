package conn

import (
	"encoding/binary"
	"reflect"

	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/tree/codec"
	"google.golang.org/protobuf/proto"
)

// 业务钩子触发特殊逻辑
func initHook(c *Conn) {
	c.RegHookHandle((*pb_gateway.LoginReq)(nil), c.onLogin)
	c.RegHookHandle((*pb_gateway.RegisterReq)(nil), c.onRegister)
	c.RegHookHandle((*pb_gateway.GetServerListReq)(nil), c.onGetServerList)

	c.RegHookHandle((*pb_auth.LoginGameAck)(nil), c.LoginAck)
}

// RegHookHandle 注册需要网关自身处理的消息，同时记录其消息编号，
// 让游戏服下行消息只读消息头就能判断是否命中钩子。
func (c *Conn) RegHookHandle(msg proto.Message, handler hookHandler) {
	msgType := reflect.TypeOf(msg)
	c.hook[msgType] = handler
	if id, ok := protoMsgID(msgType); ok {
		c.hookIDs[id] = struct{}{}
	}
}

// protoMsgID 返回消息类型在 ProtoCodec 中的 2 字节编号；类型未注册时返回 false。
func protoMsgID(msgType reflect.Type) (uint16, bool) {
	sample, ok := reflect.New(msgType.Elem()).Interface().(proto.Message)
	if !ok {
		return 0, false
	}
	data, err := codec.ProtoCodec.Marshal(sample)
	if err != nil || len(data) < 2 {
		return 0, false
	}
	return binary.BigEndian.Uint16(data[:2]), true
}

// frameMsgID 读取 ProtoCodec 编码数据开头的 2 字节消息编号。
func frameMsgID(payload []byte) (uint16, bool) {
	if len(payload) < 2 {
		return 0, false
	}
	return binary.BigEndian.Uint16(payload[:2]), true
}
