package natsrpc

import (
	"context"
	"fmt"
	"time"

	"github.com/gogu-x/gogs/constant"
	actor "github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

// natsActorPID 返回本进程内 NatsActor 的 PID，Cast/Call 都通过它中转发送。
// 各进程（gate/game/platform）启动时须以 constant.ActorNats 名字 Spawn 一个 natsrpc.Actor。
func natsActorPID() (actor.PID, error) {
	pid, ok := actor.Lookup(constant.Nats)
	if !ok {
		return actor.PID{}, fmt.Errorf("natsrpc: NatsActor(%q) not spawned in this process", constant.Nats)
	}
	return pid, nil
}

// Cast 投递消息（fire-and-forget），不等待任何回复。
// module/id/nodeID 通过 RegisterModule 注册的规则寻址，nodeID 对无节点维度的模块可传空串。
func Cast(module, id, nodeID string, frame *Frame) error {
	pid, err := natsActorPID()
	if err != nil {
		return err
	}
	return sendOrErr(pid, &castMsg{Module: module, ID: id, NodeId: nodeID, Frame: frame})
}

// Call 异步超时请求：发出后立即返回，回调在 callerPID 所属 Actor 的 goroutine 内执行。
// 适用于业务 Actor（PlayerActor/ConnActor 等）在自己的 HandleMessage 内发起跨节点/跨模块请求，
// 不阻塞当前 Actor，超时或收到回包都会触发且只触发一次 cb。
//
// 响应端需在处理完请求后调用 ReplyTo(reqFrame, respFrame) 回复。
func Call(module, id, nodeID string, frame *Frame, timeout time.Duration, callerPID actor.PID, cb func(*Frame, error)) error {
	pid, err := natsActorPID()
	if err != nil {
		return err
	}
	return sendOrErr(pid, &callMsg{
		Module: module, ID: id, NodeId: nodeID,
		Frame: frame, Timeout: timeout,
		CallerPID: callerPID, Callback: cb,
	})
}

// CallSync 同步请求：阻塞直到收到回包或超时。
// 直接使用 NATS 原生 request-reply（临时 inbox + 自动清理），不占用任何 Actor goroutine，
// 可在非 Actor 上下文（如 HTTP handler、gRPC handler）中直接调用。
//
// 响应端可以是：
//   - 使用 natsrpc 订阅并调用 ReplyTo 回复（推荐，与 Call 模式共用响应端代码）；
//   - 任何符合 NATS request-reply 协议、直接 msg.Respond(data) 的订阅者。
func CallSync(module, id, nodeID string, frame *Frame, timeout time.Duration) (*Frame, error) {
	subject, err := subjectFor(module, id, nodeID)
	if err != nil {
		return nil, err
	}
	data, err := proto.Marshal(frame)
	if err != nil {
		return nil, fmt.Errorf("natsrpc.CallSync: marshal: %w", err)
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	reply, err := nc.RequestWithContext(ctx, subject, data)
	if err != nil {
		return nil, fmt.Errorf("natsrpc.CallSync [%s/%s/%s]: %w", module, id, nodeID, err)
	}
	var respFrame Frame
	if err := proto.Unmarshal(reply.Data, &respFrame); err != nil {
		return nil, fmt.Errorf("natsrpc.CallSync: unmarshal reply: %w", err)
	}
	return &respFrame, nil
}

// ReplyTo 响应端一行回复：将 respFrame 发回 reqFrame 携带的 reply inbox。
// 对 Call 发起的请求（reqFrame.RequestId 是 NatsActor 生成的私有 inbox subject）
// 和标准 NATS request-reply（reqFrame 来自 nc.Subscribe 的 msg.Reply）均适用，
// 只要 reqFrame.RequestId 非空即可。
func ReplyTo(reqFrame *Frame, respFrame *Frame) error {
	if reqFrame.GetRequestId() == "" {
		return fmt.Errorf("natsrpc.ReplyTo: request frame has no RequestId, not a request")
	}
	return publish(reqFrame.RequestId, respFrame)
}

func sendOrErr(pid actor.PID, msg interface{}) error {
	if !actor.Send(pid, msg) {
		return fmt.Errorf("natsrpc: send to NatsActor failed (mailbox full or actor stopped)")
	}
	return nil
}
