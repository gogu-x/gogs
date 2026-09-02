package natsrpc

import (
	"context"
	"fmt"
	"time"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

// natsActorPID 返回本进程内 NatsActor 的 PID，Cast/Call 都通过它中转发送。
// 各进程（gate/game/platform）启动时须以 constant.ActorNats 名字 Spawn 一个 natsrpc.Actor。
func natsActorPID() (tree.PID, error) {
	pid, ok := tree.Lookup(def.Nats)
	if !ok {
		return tree.PID{}, fmt.Errorf("natsrpc: NatsActor(%q) not spawned in this process", def.Nats)
	}
	return pid, nil
}

// Cast 投递消息（fire-and-forget），不等待任何回复。
// module/id/nodeID 通过 RegisterModule 注册的规则寻址，nodeID 对无节点维度的模块可传空串。
func Cast(module, id, nodeID string, msg []byte) error {
	pid, err := natsActorPID()
	if err != nil {
		return err
	}
	return sendOrErr(pid, &castMsg{Module: module, ID: id, NodeId: nodeID, Msg: msg})
}

// Call 异步超时请求：发出后立即返回，回调在 callerPID 所属 Actor 的 goroutine 内执行。
// 适用于业务 Actor（PlayerActor/ConnActor 等）在自己的 HandleMessage 内发起跨节点/跨模块请求，
// 不阻塞当前 Actor，超时或收到回包都会触发且只触发一次 cb。
//
// 响应端需在处理完请求后调用 ReplyTo(reqFrame, respFrame) 回复。
func Call(module, id, nodeID string, msg []byte, timeout time.Duration, callerPID tree.PID, cb func(proto.Message, error)) error {
	pid, err := natsActorPID()
	if err != nil {
		return err
	}
	return sendOrErr(pid, &callMsg{
		Module: module, ID: id, NodeId: nodeID,
		Msg: msg, Timeout: timeout,
		CallerPID: callerPID, Callback: cb,
	})
}

// CallSync 同步请求：阻塞直到收到回包或超时。
func CallSync(module, id, nodeID string, msg proto.Message, timeout time.Duration) (proto.Message, error) {
	subject, err := subjectFor(module, id, nodeID)
	if err != nil {
		return nil, err
	}
	data, err := proto.Marshal(msg)
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
	var respFrame proto.Message
	if err := proto.Unmarshal(reply.Data, respFrame); err != nil {
		return nil, fmt.Errorf("natsrpc.CallSync: unmarshal reply: %w", err)
	}
	return respFrame, nil
}

func sendOrErr(pid tree.PID, msg interface{}) error {
	if !tree.Send(pid, msg) {
		return fmt.Errorf("natsrpc: send to NatsActor failed (mailbox full or actor stopped)")
	}
	return nil
}
