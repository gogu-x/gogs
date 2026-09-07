package internal

import (
	"time"

	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// NatsMsgReq 将消息发送目标actor模块中
func (ns *Nats) NatsMsgReq(ctx tree.Context, m *sspb.NatsMsgReq) {
	//这是订阅者nats收到消息处理。解析消息数据，将消息请求发送给目标act
	pid, ok := tree.Default().Lookup(m.TaggerName)
	if !ok {
		tlog.Log.Error("NatsMsgReq actor no name=%v", m.TaggerName)
		return
	}
	//发送到目标actor，等待回复。回复不走 Await/cb，而是作为普通消息投递回
	//本 Nats actor 的 mailbox，统一在 HandleMessage(default 分支即 MsgHandle) 中处理，
	//这样才能读到下面设置的 sessionID/taggerName 等路由信息。
	ctx.SetValue("sessionID", m.SessionID)
	ctx.SetValue("nodeID", m.NodeID)
	ctx.SetValue("id", m.Id)
	ctx.SetValue("taggerName", m.TaggerName)
	ctx.SetValue("sendName", m.SendName)
	ctx.SetValue("sendModule", m.SendModule)
	ctx.SetValue("taggerModule", m.TaggerModule)
	msg, err := ns.codec.Unmarshal(m.Msg)
	if err != nil {
		tlog.Log.Error("%s", err)
		return
	}
	ctx.RequestAsMessage(pid, msg)

}

// NatsMsgAck 将消息发送目标actor模块中
func (ns *Nats) NatsMsgAck(ctx tree.Context, m *sspb.NatsMsgAck) {
	//这是订阅者nats收到消息处理。解析消息数据，将消息请求发送给目标act
	pid, ok := tree.Default().Lookup(m.SendName)
	if !ok {
		tlog.Log.Error("NatsMsgReq actor no name=%v", m.SendName)
		return
	}
	msg, err := ns.codec.Unmarshal(m.Msg)
	if err != nil {
		tlog.Log.Error("%s", err)
		return
	}
	ctx.Request(pid, msg)

}

// MsgHandle 处理所有消息回调，只处理带有sessionId的消息
func (ns *Nats) MsgHandle(ctx tree.Context, m interface{}) {
	//把消息处理成natsMsgAck 发给发送者的nats中
	sessionID := ctx.GetValue("sessionID").(int64)
	id := ctx.GetValue("id").(int32)
	nodeID := ctx.GetValue("nodeID").(int32)
	taggerName := ctx.GetValue("taggerName").(string)
	sendName := ctx.GetValue("sendName").(string)
	sendModule := ctx.GetValue("sendModule").(string)
	taggerModule := ctx.GetValue("taggerModule").(string)

	msgBytes, err := ns.codec.Marshal(m)
	if err != nil {
		tlog.Log.Error("codec MsgHandle marshal err=%v", err)
		return
	}
	ack := &sspb.NatsMsgAck{
		SessionID:    sessionID,
		SendModule:   sendModule,
		TaggerModule: taggerModule,
		SendName:     sendName,
		TaggerName:   taggerName,
		Id:           id,
		NodeID:       nodeID,
		Msg:          msgBytes,
	}

	data, err := proto.Marshal(ack)
	subject := Subject(ack.SendModule, int(ack.Id), int(ack.NodeID))
	if err := ns.conn.Publish(subject, data); err != nil {
		tlog.Log.Info("natsRpc: publish cast to %s: %v", subject, err)
		return
	}
	tlog.Log.Debug("natsRpc: cast %T to %s", ack, subject)
}

// catsMsg 发送 fire-and-forget 消息到 NATS。
func (ns *Nats) catsMsg(msg *CastMsg) {
	ns.nextRequestID.Add(1)

	if msg == nil || msg.Msg == nil {
		tlog.Log.Info("natsrpc: cast nil message")
		return
	}
	if ns.conn == nil {
		tlog.Log.Info("natsRpc: cast before Init")
		return
	}
	//把消息转换成

	data, err := ns.codec.Marshal(msg.Msg)
	if err != nil {
		tlog.Log.Info("natsRpc: marshal cast %T: %v", msg.Msg, err)
		return
	}
	subject := Subject(msg.Module, msg.ID, msg.NodeId)
	if err := ns.conn.Publish(subject, data); err != nil {
		tlog.Log.Info("natsRpc: publish cast to %s: %v", subject, err)
		return
	}
	tlog.Log.Debug("natsRpc: cast %T to %s", msg.Msg, subject)
}

// handleCall 在 NatsActor goroutine 内执行：生成 reply inbox subject，存 pending，发消息。
func (ns *Nats) handleCall(ctx tree.Context, m *CallMsg) {
	sessionID := ns.nextRequestID.Add(1)
	if m == nil {
		return
	}
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	//这里把消息转换一下发给nats 服务器， 比如一个natsReq 然后 订阅者收到以后应该也是一个req
	//这个req是NatS发给目标actor,给目标actor一定是投递一个带有回复的请求 tree.Request 这里不需要设置回调函数
	//回调函数在nats收到目标actor数据以后根据消息回复给你发送者的actor，这里必须给你每一条消息建立一个map，一遍回包使用

	ns.ReqMsgMap[sessionID] = ctx.RequestEnvelope()

	//编码如何无法编码直接返回err

	byteMsg, err := ns.codec.Marshal(m.Msg)
	if err != nil {
		ctx.Response(nil, err)
		tlog.Log.Error("natsRpc call msg marshal: %v", err)
		return
	}
	natsReqMsg := &sspb.NatsMsgReq{
		SessionID:    sessionID,
		SendModule:   m.SendModule,
		TaggerModule: m.TaggerModule,
		SendName:     m.SendName,
		TaggerName:   m.TaggerName,
		Id:           int32(m.ID),
		NodeID:       int32(m.NodeId),
		Msg:          byteMsg,
	}

	natsReqMsgBytes, err := proto.Marshal(natsReqMsg)
	if err != nil {
		ctx.Response(nil, err)
		tlog.Log.Error("natsRpc call msg marshal: %v", err)
		return
	}
	subject := Subject(m.TaggerModule, m.ID, m.NodeId)
	if err := ns.conn.Publish(subject, natsReqMsgBytes); err != nil {
		tlog.Log.Info("natsRpc: publish cast to %s: %v", subject, err)
		return
	}

	tlog.Log.Debug("natsRpc: cast call %T to %s", m.Msg, subject)
}
