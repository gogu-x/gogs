package natsrpc

import (
	"fmt"
	"time"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/natsrpc/internal"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// NewNats NATS 订阅 这里只是单个进程的订阅 module是一个进程，而非一个actor：

func NewNats(module string, serverID, nodeID int, url string) *internal.Nats {
	return internal.NewActor(module, serverID, nodeID, url)
}

// Cast 投递消息（fire-and-forget），不等待任何回复。
// module/id/nodeID 通过 RegisterModule 注册的规则寻址，nodeID 对无节点维度的模块可传空串。
func Cast(module string, id, nodeID int, msg interface{}) error {
	return sendOrErr(&internal.CastMsg{Module: module, ID: id, NodeId: nodeID, Msg: msg})
}

// Call 异步超时请求：发出后立即返回，回调在 callerPID 所属 Actor 的 goroutine 内执行。
// 适用于业务 Actor（PlayerActor/ConnActor 等）在自己的 HandleMessage 内发起跨节点/跨模块请求，
// 不阻塞当前 Actor，超时或收到回包都会触发且只触发一次 cb。
// 响应端需在业务 Actor 中调用 ctx.Response(resp, err) 回复。

func Call(sendModule, taggerModule string, sendActorName, taggerActorName string, id, nodeID int, msg interface{}, callerPID tree.PID, cb func(tree.Context, interface{}, error)) error {
	pid, ok := tree.Lookup(def.Nats)
	if !ok {
		err := fmt.Errorf("natsrpc: NatsActor(%q) not spawned in this process", def.Nats)
		tlog.Log.Debug("%s", err)
		return err
	}
	callMsg := &internal.CallMsg{
		SendModule:   sendModule,
		TaggerModule: taggerModule,
		ID:           id,
		NodeId:       nodeID,
		Msg:          msg,
		Timeout:      time.Second,
		SendName:     sendActorName,
		TaggerName:   taggerActorName,
	}
	tree.Default().RequestCallback(pid, callMsg, callerPID, cb)
	return nil
}

// CallSync
func CallSync(module string, id, nodeID int, msg proto.Message, timeout time.Duration) {
	data, err := codec.ProtoCodec.Marshal(msg)
	if err != nil {
		return
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	tlog.Log.Info("natsrpc: call sync %v %v %v %v", module, id, nodeID, data)
}

func sendOrErr(msg interface{}) error {
	pid, ok := tree.Lookup(def.Nats)
	if !ok {
		return fmt.Errorf("natsrpc: NatsActor(%q) not spawned in this process", def.Nats)
	}
	if !tree.Send(pid, msg) {
		return fmt.Errorf("natsrpc: send to NatsActor failed (mailbox full or actor stopped)")
	}
	return nil
}
