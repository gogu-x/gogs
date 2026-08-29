package base

import (
	"log"
	"reflect"

	gamegate "github.com/gogu-x/gogs/game/gate"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"google.golang.org/protobuf/proto"
)

// SysContext 是所有 play 消息共享的上下文：模块 + 本次消息的 tree.Context。
// 其他模块投递或内部异步产生的消息只拿到它，因此在类型层面就不存在“玩家”概念，
// 也不需要做登录判断。
type SysContext struct {
	App *App
	ctx tree.Context
}

// Tree 返回本次消息的 Actor 上下文，供需要底层能力的场景使用。
func (s *SysContext) Tree() tree.Context { return s.ctx }

// Players 返回玩家管理器，系统消息若需要作用于某个玩家，自行按消息里的 uid 查找。
func (s *SysContext) Players() *player.PlayerMgr { return s.App.Players }

// Request 向目标 Actor 发请求，回调在本 Actor goroutine 内执行，
// 携带本 Actor 的 tree.Context。
func (s *SysContext) Request(pid tree.PID, msg interface{}, cb func(tree.Context, interface{}, error)) {
	s.ctx.RequestCallback(pid, msg, cb)
}

// Send 向目标 Actor 单向投递消息。
func (s *SysContext) Send(pid tree.PID, msg interface{}) bool { return s.ctx.Send(pid, msg) }

// Response 对通过 tree.Request 发来的消息进行应答（非请求时为 no-op）。
func (s *SysContext) Response(value interface{}, err error) { s.ctx.Response(value, err) }

// PlayContext 是 gate 玩家请求的上下文：在 SysContext 之上附带连接信息与玩家数据。
// 每个 Frame 新建一个实例（不做池化，因为 Request 回调会在之后异步持有它）。
type PlayContext struct {
	SysContext

	// Player 需登录路由下一定非 nil；免登录路由（登录/注册）可能为 nil。
	Player *player.Player

	PlayerId uint64
	ConnID   uint64
	GateId   string
	// RequestId 非空表示这是一次跨节点 request-reply，回包走 inbox 而不是 gate。
	RequestId string

	frame *protoGateway.Frame
}

// Frame 返回本次请求的原始帧（只读用途）。
func (s *PlayContext) Frame() *protoGateway.Frame { return s.frame }

// Reply 回包给客户端：
//   - RequestId 非空 → 回到发起方的 inbox（跨节点请求）；
//   - 否则 → Cast 到 gate.out.{GateId}，由 Gate 写回对应连接。
func (s *PlayContext) Reply(msg proto.Message) {
	body, err := codec.ProtoCodec.Marshal(msg)
	if err != nil {
		log.Printf("play: reply marshal error: %v", err)
		return
	}
	resp := &protoGateway.Frame{
		Uid:     s.PlayerId,
		ConnId:  s.ConnID,
		GateId:  s.GateId,
		Payload: body,
		MsgType: reflect.TypeOf(msg).Elem().Name(),
	}
	if s.RequestId != "" {
		resp.RequestId = s.RequestId
		if err := s.App.Sender.ReplyTo(s.frame, resp); err != nil {
			log.Printf("play: reply to inbox error: %v", err)
		}
		return
	}
	if err := s.App.Sender.CastGate(s.GateId, resp); err != nil {
		log.Printf("play: reply cast error: %v", err)
	}
}

// Sender 抽象 Frame 出口，便于单测注入替身。
type Sender interface {
	CastGate(gateID string, f *protoGateway.Frame) error
	ReplyTo(req *protoGateway.Frame, resp *protoGateway.Frame) error
}

type natsSender struct{}

func (natsSender) CastGate(gateID string, f *protoGateway.Frame) error {
	if gamegate.PushToConn(gateID, f.GetConnId(), f) {
		return nil
	}
	return natsrpc.Cast(natsrpc.ModuleGate, gateID, "", f)
}

func (natsSender) ReplyTo(req *protoGateway.Frame, resp *protoGateway.Frame) error {
	return natsrpc.ReplyTo(req, resp)
}
