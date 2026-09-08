package internal

import (
	"fmt"
	"time"

	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

// NatsMsgReq routes an RPC request to the target actor. The actor response is
// delivered back to this Nats actor as an ordinary message so MsgHandle can
// preserve the routing metadata carried in the context.
func (ns *Nats) NatsMsgReq(ctx tree.Context, m *sspb.NatsMsgReq) {
	if m == nil {
		return
	}
	pid, ok := ctx.Lookup(m.TaggerName)
	if !ok {
		tlog.Log.Error("NatsMsgReq actor no name=%v", m.TaggerName)
		return
	}
	msg, err := ns.codec.Unmarshal(m.Msg)
	if err != nil {
		tlog.Log.Error("natsrpc: decode request: %v", err)
		return
	}
	ctx.SetValue("sessionID", m.SessionID)
	ctx.SetValue("nodeID", m.NodeID)
	ctx.SetValue("id", m.Id)
	ctx.SetValue("taggerName", m.TaggerName)
	ctx.SetValue("sendName", m.SendName)
	ctx.SetValue("sendModule", m.SendModule)
	ctx.SetValue("taggerModule", m.TaggerModule)
	ctx.RequestAsMessage(pid, msg)
}

// NatsMsgNtf routes a fire-and-forget notification to its target actor.
func (ns *Nats) NatsMsgNtf(ctx tree.Context, m *sspb.NatsMsgNtf) {
	if m == nil {
		return
	}
	pid, ok := ctx.Lookup(m.TaggerName)
	if !ok {
		tlog.Log.Error("NatsMsgNtf actor no name=%v", m.TaggerName)
		return
	}
	msg, err := ns.codec.Unmarshal(m.Msg)
	if err != nil {
		tlog.Log.Error("natsrpc: decode notification: %v", err)
		return
	}
	if !ctx.Send(pid, msg) {
		tlog.Log.Error("natsrpc: route notification to actor %q failed", m.TaggerName)
	}
}

// NatsMsgAck resolves and removes a pending request. Decode failures are also
// returned to the original caller instead of leaving it pending until timeout.
func (ns *Nats) NatsMsgAck(_ tree.Context, m *sspb.NatsMsgAck) {
	if m == nil {
		return
	}
	if _, ok := ns.ReqMsgMap[m.SessionID]; !ok {
		tlog.Log.Debug("natsrpc: ack for unknown session %d", m.SessionID)
		return
	}
	msg, err := ns.codec.Unmarshal(m.Msg)
	ns.finishPending(m.SessionID, msg, err)
}

// MsgHandle publishes a target actor's response back to the request origin.
func (ns *Nats) MsgHandle(ctx tree.Context, m interface{}) {
	sessionID, ok := ctx.GetValue("sessionID").(int64)
	if !ok {
		tlog.Log.Error("natsrpc: response has no session routing metadata")
		return
	}
	id, idOK := ctx.GetValue("id").(int32)
	nodeID, nodeOK := ctx.GetValue("nodeID").(int32)
	taggerName, taggerOK := ctx.GetValue("taggerName").(string)
	sendName, sendOK := ctx.GetValue("sendName").(string)
	sendModule, moduleOK := ctx.GetValue("sendModule").(string)
	taggerModule, targetModuleOK := ctx.GetValue("taggerModule").(string)
	if !idOK || !nodeOK || !taggerOK || !sendOK || !moduleOK || !targetModuleOK {
		tlog.Log.Error("natsrpc: response has incomplete routing metadata")
		return
	}

	msgBytes, err := ns.codec.Marshal(m)
	if err != nil {
		tlog.Log.Error("natsrpc: encode response: %v", err)
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
	data, err := ns.codec.Marshal(ack)
	if err != nil {
		tlog.Log.Error("natsrpc: encode ack: %v", err)
		return
	}
	subject := Subject(ack.SendModule, int(ack.Id), int(ack.NodeID))
	if err := ns.publish(subject, data); err != nil {
		tlog.Log.Info("natsrpc: publish ack to %s: %v", subject, err)
		return
	}
	tlog.Log.Debug("natsrpc: publish %T to %s", ack, subject)
}

// catsMsg sends a fire-and-forget message wrapped in NatsMsgNtf. The wrapper
// carries the target actor name; the NATS subject addresses module/server/node.
func (ns *Nats) catsMsg(msg *NatsMsg) error {
	if msg == nil || msg.Msg == nil {
		return fmt.Errorf("natsrpc: cast nil message")
	}
	payload, err := ns.codec.Marshal(msg.Msg)
	if err != nil {
		return fmt.Errorf("natsrpc: marshal cast %T: %w", msg.Msg, err)
	}
	sessionID := ns.nextRequestID.Add(1)
	ntf := &sspb.NatsMsgNtf{
		SessionID:    sessionID,
		SendModule:   ns.typ,
		TaggerModule: msg.TaggerModule,
		SendName:     msg.SendName,
		TaggerName:   msg.TaggerName,
		Id:           int32(msg.ID),
		NodeID:       int32(msg.NodeId),
		Msg:          payload,
	}
	data, err := ns.codec.Marshal(ntf)
	if err != nil {
		return fmt.Errorf("natsrpc: marshal notification: %w", err)
	}
	subject := Subject(msg.TaggerModule, msg.ID, msg.NodeId)
	if err := ns.publish(subject, data); err != nil {
		return fmt.Errorf("natsrpc: publish cast to %s: %w", subject, err)
	}
	tlog.Log.Debug("natsrpc: cast %T to %s", msg.Msg, subject)
	return nil
}

// handleCall runs in the Nats actor goroutine.
func (ns *Nats) handleCall(ctx tree.Context, m *NatsMsg) {
	if m == nil {
		ctx.Response(nil, fmt.Errorf("natsrpc: nil call"))
		return
	}
	env := ctx.RequestEnvelope()
	if env == nil {
		tlog.Log.Error("natsrpc: call must be sent as a Tree request")
		return
	}
	ns.sendCall(m, env)
}

// sendCall contains the transport-facing request logic and is intentionally
// independent of tree.Context so encoding, publication and timeout behavior can
// be tested without a live NATS server.
func (ns *Nats) sendCall(m *NatsMsg, env *tree.Envelope) {
	if m == nil || env == nil {
		return
	}
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	payload, err := ns.codec.Marshal(m.Msg)
	if err != nil {
		env.Respond(nil, fmt.Errorf("natsrpc: marshal call payload: %w", err))
		return
	}

	sessionID := ns.nextRequestID.Add(1)
	originModule := m.SendModule
	if originModule == "" {
		originModule = ns.typ
	}
	req := &sspb.NatsMsgReq{
		SessionID:    sessionID,
		SendModule:   originModule,
		TaggerModule: m.TaggerModule,
		SendName:     m.SendName,
		TaggerName:   m.TaggerName,
		// Id and NodeID identify the request origin so the target can route ACK.
		Id:     int32(ns.serverId),
		NodeID: int32(ns.nodeId),
		Msg:    payload,
	}
	data, err := ns.codec.Marshal(req)
	if err != nil {
		env.Respond(nil, fmt.Errorf("natsrpc: marshal request: %w", err))
		return
	}

	ns.ReqMsgMap[sessionID] = env
	subject := Subject(m.TaggerModule, m.ID, m.NodeId)
	if err := ns.publish(subject, data); err != nil {
		delete(ns.ReqMsgMap, sessionID)
		env.Respond(nil, fmt.Errorf("natsrpc: publish call to %s: %w", subject, err))
		return
	}
	if ns.afterTimeout != nil {
		if h := ns.afterTimeout(timeout, sessionID); h != nil {
			ns.requestTimers[sessionID] = h
		}
	}
	tlog.Log.Debug("natsrpc: call %T to %s", m.Msg, subject)
}

func (ns *Nats) timeoutRequest(sessionID int64) {
	ns.finishPending(sessionID, nil, tree.ErrTimeout)
}

func (ns *Nats) finishPending(sessionID int64, value interface{}, err error) {
	env, ok := ns.ReqMsgMap[sessionID]
	if !ok {
		return
	}
	delete(ns.ReqMsgMap, sessionID)
	if h := ns.requestTimers[sessionID]; h != nil {
		h.Stop()
	}
	delete(ns.requestTimers, sessionID)
	env.Respond(value, err)
}
