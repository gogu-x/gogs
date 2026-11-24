package ipb

import "google.golang.org/protobuf/proto"

// ForwardedRequest 是玩家 Actor 原样转交给业务 Actor 的客户端请求，附带发起请求的会话。
//
// 玩家侧没有业务的请求（活动、联盟等）在玩家 Actor 完成 UID 和数据就绪校验后包装成该消息，
// 由业务 Actor 处理，并按 UID 和 Generation 用 ReplyToSession 直接回包给发起请求的会话。
type ForwardedRequest struct {
	UID        uint64        // 发起请求的玩家 ID。
	Generation uint64        // 发起请求的会话版本，回包时带上，旧会话的响应不会写到新连接。
	Request    proto.Message // 原始客户端请求。
}

// GetUID 返回发起请求的玩家 ID。
func (m *ForwardedRequest) GetUID() uint64 {
	if m == nil {
		return 0
	}
	return m.UID
}

// GetGeneration 返回发起请求的会话版本。
func (m *ForwardedRequest) GetGeneration() uint64 {
	if m == nil {
		return 0
	}
	return m.Generation
}

// GetRequest 返回原始客户端请求。
func (m *ForwardedRequest) GetRequest() proto.Message {
	if m == nil {
		return nil
	}
	return m.Request
}
