package battle

import (
	"fmt"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"

	"github.com/gogu-x/gogs/def"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/natsrpc"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// registerMessages 在 Actor 初始化时注册所有支持的消息及未知消息的错误回包。
func (c *Client) registerMessages() {
	tree.RegisterFunc(&c.router, (*sspb.Event)(nil), c.handleEvent)
	tree.RegisterFunc(&c.router, (*pb.ClaimBattleGrantReq)(nil), c.forwardClaim)
	tree.RegisterFunc(&c.router, (*sspb.ApplyBattleGrantReq)(nil), c.forwardGrant)
	tree.RegisterFunc(&c.router, (*Begin)(nil), c.begin)
	tree.RegisterFunc(&c.router, (*Reset)(nil), c.reset)
	tree.RegisterFunc(&c.router, (*GetState)(nil), c.getState)
	tree.RegisterFunc(&c.router, (*pb.BattleInputReq)(nil), c.forwardInput)
	tree.RegisterFunc(&c.router, (*pb.BattleInputAck)(nil), c.inputAck)
	tree.RegisterFunc(&c.router, (*pb.BattleAdmissionFailedNtf)(nil), c.admissionFailed)
	tree.RegisterFunc(&c.router, (*pb.BattleCreatedNtf)(nil), c.created)
	tree.RegisterFunc(&c.router, (*pb.BattleActionNtf)(nil), c.action)
	tree.RegisterFunc(&c.router, (*pb.BattleFinishedNtf)(nil), c.finished)
	c.router.SetFallback(func(ctx tree.Context, msg interface{}) {
		ctx.Response(nil, fmt.Errorf("battle client: unsupported message %T", msg))
	})
}

// handleEvent 处理开关服通知，并向事件发送方确认处理结果。
func (c *Client) handleEvent(ctx tree.Context, m *sspb.Event) {
	if gameevent.IsServerOpened(m) {
		c.serverOpened = true
		c.serverClosing = false
		tlog.Log.Info("[battle/Client.handleEvent] 战斗客户端已处理开服事件")
		ctx.Response(nil, nil)
		return
	}
	if !gameevent.IsServerClosing(m) {
		eventName := m.GetName()
		err := fmt.Errorf("unsupported battle event %q", eventName)
		tlog.Log.Error("[battle/Client.handleEvent] 收到不支持的战斗事件, event=%v err=%v", eventName, err)
		ctx.Response(nil, err)
		return
	}
	c.serverClosing = true
	tlog.Log.Info("[battle/Client.handleEvent] 战斗客户端已处理关服事件")
	ctx.Response(nil, nil)
}

// inputAck 仅向当前仍在对应战斗中的玩家转发动作回执。
func (c *Client) inputAck(ctx tree.Context, m *pb.BattleInputAck) {
	uid := m.GetUid()
	if r := c.players[uid]; r != nil && r.Status == Running && r.BattleID == m.GetBattleId() {
		c.push(ctx, uid, m)
	}
}

// state 获取玩家战斗记录，首次访问时按空闲状态初始化。
func (c *Client) state(uid uint64) *record {
	r := c.players[uid]
	if r == nil {
		r = &record{State: State{UID: uid, Status: Idle}, seen: make(map[uint64]struct{})}
		c.players[uid] = r
	}
	return r
}

// begin 校验参战成员的当前状态，选择 Battle 节点并提交创建或加入请求。
func (c *Client) begin(ctx tree.Context, msg *Begin) {
	if !c.serverOpened || c.serverClosing {
		ctx.Response(nil, fmt.Errorf("battle client is unavailable during server lifecycle transition"))
		return
	}
	if msg == nil || msg.Request == nil || msg.Request.GetUID() == 0 {
		ctx.Response(nil, fmt.Errorf("battle client: request and uid are required"))
		return
	}
	request := proto.Clone(msg.Request).(*pb.StartBattleReq)
	request.ServerID = uint32(c.gameServerID)
	request.SourceNodeId = uint32(c.gameNodeID)
	uid := request.GetUID()
	r := c.state(uid)
	if r.playback.pending(comm.RealNow()) {
		ctx.Response(r.State, ErrInProgress)
		return
	}
	id := request.GetJoinBattleId()
	if (r.Status == Running || r.Status == CreateUnknown) && id != "" && id == r.BattleID {
		// 重复加入同一场用作主动同步，不占用第二个单位。
		err := c.sender.Start(r.Node, request)
		ctx.Response(r.State, err)
		return
	}
	if r.Status == Creating || r.Status == CreateUnknown || r.Status == Running {
		ctx.Response(r.State, ErrInProgress)
		return
	}
	var node Node
	if id != "" {
		server, nodeID, ok := def.BattleRoomRoute(id)
		if !ok || request.GetAllowJoin() || len(request.GetInitialMembers()) != 0 {
			ctx.Response(r.State, ErrInvalidRequest)
			return
		}
		node = Node{ServerID: server, NodeID: nodeID}
	} else {
		var err error
		node, err = c.selector.Pick()
		if err != nil {
			r.Status = Idle
			ctx.Response(r.State, err)
			return
		}
	}
	// Game 内部调用方提供已确定的同节点队友。先检查全员状态，发送成功后一起登记。
	seen := map[uint64]bool{uid: true}
	for _, m := range request.GetInitialMembers() {
		if m.GetUid() == 0 || m.GetRole() == nil || seen[m.GetUid()] {
			ctx.Response(r.State, ErrInvalidRequest)
			return
		}
		seen[m.GetUid()] = true
		other := c.state(m.GetUid())
		if (other.Status != Idle && other.Status != Finished) || other.playback.pending(comm.RealNow()) {
			ctx.Response(r.State, ErrInProgress)
			return
		}
		m.ServerId = request.GetServerID()
		m.NodeId = request.GetSourceNodeId()
	}
	r.roomRequest = nil
	if request.GetAllowJoin() || id != "" {
		r.roomRequest = request
	}
	r.State = State{UID: uid, Status: Creating, Node: node, BattleID: id}
	r.snapshotFloor = 0
	r.seen = make(map[uint64]struct{})
	r.playback = nil

	if err := c.sender.Start(node, request); err != nil {
		r.roomRequest = nil
		r.State = State{UID: uid, Status: Idle}
		ctx.Response(r.State, err)
		return
	}
	// NATS 已发布但尚未收到 Created；此时禁止盲目重发，避免生成两场战斗。
	r.Status = CreateUnknown
	for _, m := range request.GetInitialMembers() {
		other := c.state(m.GetUid())
		*other = record{State: State{UID: m.GetUid(), Status: CreateUnknown, Node: node}, seen: make(map[uint64]struct{})}
		if request.GetAllowJoin() {
			other.roomRequest = &pb.StartBattleReq{UID: m.GetUid(), ServerID: m.GetServerId(), SourceNodeId: m.GetNodeId(), BattleType: request.GetBattleType(), MonsterGroupConfigId: request.GetMonsterGroupConfigId(), AttackerRoles: []*pb.Role{m.GetRole()}}
		}
	}
	ctx.Response(r.State, nil)
}

// created 接收权威快照，更新玩家的战斗编号和事件序号基线，并向客户端推送。
func (c *Client) created(ctx tree.Context, msg *pb.BattleCreatedNtf) {
	if msg == nil || msg.Uid == 0 || msg.BattleId == "" {
		return
	}
	r := c.state(msg.Uid)
	if r.BattleID != "" && r.BattleID != msg.BattleId {
		return
	}
	if r.Status != CreateUnknown && r.Status != Creating && r.Status != Running {
		return
	}
	if msg.GetSnapshotSequence() < r.LastSequence {
		return
	}
	r.snapshotFloor = msg.GetSnapshotSequence()
	r.LastSequence = msg.GetSnapshotSequence()
	r.BattleID = msg.BattleId
	r.Status = Running
	if r.playback == nil && len(msg.GetControls()) == 0 {
		r.playback = newPlaybackWindow()
	}
	c.push(ctx, msg.Uid, msg)
}

// action 按玩家和战斗编号检查过程事件，过滤重复事件，共享战斗缺号时请求快照。
func (c *Client) action(ctx tree.Context, msg *pb.BattleActionNtf) {
	if msg == nil || msg.Event == nil || msg.Uid == 0 || msg.BattleId == "" {
		return
	}
	r := c.state(msg.Uid)
	if r.Status != Running || r.BattleID != msg.BattleId {
		return
	}
	sequence := msg.Event.Sequence
	if sequence == 0 || sequence <= r.snapshotFloor || (r.roomRequest != nil && sequence <= r.LastSequence) {
		return
	}
	if _, duplicate := r.seen[sequence]; duplicate {
		return
	}
	if req := r.roomRequest; req != nil && sequence > r.LastSequence+1 {
		// 请求权威快照；后续丢帧时仍可重试。旧快照由序号过滤。
		syncReq := proto.Clone(req).(*pb.StartBattleReq)
		syncReq.AllowJoin = false
		syncReq.InitialMembers = nil
		syncReq.JoinBattleId = r.BattleID
		if err := c.sender.Start(r.Node, syncReq); err != nil {
			tlog.Log.Warn("[战斗/同步] 快照请求失败, pid=%v err=%v", msg.GetUid(), err)
		}
	}
	r.seen[sequence] = struct{}{}
	r.playback.observe(msg.GetEvent(), comm.RealNow())
	if sequence > r.LastSequence {
		r.LastSequence = sequence
	}
	c.push(ctx, msg.Uid, msg)
}

// finished 按战斗和玩家去重，成功投递结算后向 Battle 确认。
func (c *Client) finished(ctx tree.Context, msg *pb.BattleFinishedNtf) {
	if msg == nil || msg.Uid == 0 || msg.BattleId == "" {
		return
	}
	key := completionKey{msg.BattleId, msg.Uid}
	if node, ok := c.completed[key]; ok {
		_ = c.sender.Confirm(node, &pb.BattleResultConfirmedNtf{BattleId: msg.GetBattleId(), Uid: msg.GetUid()})
		return
	}
	r := c.state(msg.Uid)
	if r.BattleID != "" && r.BattleID != msg.BattleId {
		return
	}
	// Finished 可恢复丢失的 Created；重复 Finished 只确认，不重复推给客户端或重复结算。
	settlement := c.buildSettlement(msg)
	if !c.deliverSettlement(ctx, settlement) {
		return
	}
	c.completed[key] = r.Node
	r.roomRequest = nil
	r.BattleID = msg.BattleId
	r.Checksum = msg.Checksum
	r.Status = Finished
	c.push(ctx, msg.Uid, msg)
	_ = c.sender.Confirm(r.Node, &pb.BattleResultConfirmedNtf{BattleId: msg.BattleId, Uid: msg.Uid})
}

// deliverSettlement 经网关把结算交给每名参战玩家；玩家在线时直达现有 Actor，离线时网关
// 唤醒临时 PlayerActor 加载、处理并存盘后回收。
//
// 使用非阻塞投递，网关队列满时返回 false，由调用方放弃本次确认等待 Battle 重发，
// 不让全区共用的战斗客户端被网关拖住。每个通知归属于一名参战者，按战斗编号和玩家编号去重。
func (c *Client) deliverSettlement(ctx tree.Context, settlement *Settlement) bool {
	gatePID, ok := ctx.Lookup(def.GameGate)
	if !ok {
		tlog.Log.Error("[battle/Client.deliverSettlement] 网关 Actor 不可用，等待 Battle 重发结束通知, battleID=%v", settlement.BattleID)
		return false
	}
	for _, participant := range settlement.Participants {
		if !ctx.TrySend(gatePID, &ipb.DeliverToPlayer{UID: participant.UID, Payload: settlement}) {
			tlog.Log.Error("[battle/Client.deliverSettlement] 网关队列已满，等待 Battle 重发结束通知, uid=%v battleID=%v", participant.UID, settlement.BattleID)
			return false
		}
	}
	return true
}

// buildSettlement 根据当前收件人的战斗结果组装玩家结算消息，每名参战者独立去重和确认。
func (c *Client) buildSettlement(msg *pb.BattleFinishedNtf) *Settlement {
	return &Settlement{
		BattleID:   msg.GetBattleId(),
		BattleType: msg.GetBattleType(),
		Outcome:    msg.GetOutcome(),
		Units:      msg.GetUnits(),
		Checksum:   msg.GetChecksum(),
		Participants: []Participant{{
			UID:      msg.GetUid(),
			ServerID: c.gameServerID,
			NodeID:   c.gameNodeID,
		}},
	}
}

// reset 检查战斗状态和演出窗口，允许重置时将玩家恢复为空闲状态。
func (c *Client) reset(ctx tree.Context, msg *Reset) {
	if msg == nil || msg.UID == 0 {
		ctx.Response(nil, fmt.Errorf("battle client: uid is required"))
		return
	}
	r := c.state(msg.UID)
	if r.playback.pending(comm.RealNow()) {
		ctx.Response(r.State, ErrInProgress)
		return
	}
	if r.Status == Creating || r.Status == CreateUnknown || r.Status == Running {
		ctx.Response(r.State, fmt.Errorf("battle client: cannot reset %s", r.Status))
		return
	}
	r.State = State{UID: msg.UID, Status: Idle}
	r.snapshotFloor = 0
	r.seen = make(map[uint64]struct{})
	ctx.Response(r.State, nil)
}

// getState 向调用方返回玩家当前的战斗状态。
func (c *Client) getState(ctx tree.Context, msg *GetState) {
	if msg == nil || msg.UID == 0 {
		ctx.Response(nil, fmt.Errorf("battle client: uid is required"))
		return
	}
	ctx.Response(c.state(msg.UID).State, nil)
}

// push 按命名规则查找玩家会话代理并直接推送战斗消息，不经过 Gate。
//
// 投递为非阻塞：玩家离线或代理队列已满时丢弃本条推送，避免全区共用的战斗客户端被拖住。
// 丢弃只影响客户端演出，结算仍由 deliverSettlement 投递。
func (c *Client) push(ctx tree.Context, uid uint64, msg proto.Message) {
	ipb.NotifyCurrentSession(ctx, uid, msg)
}

// forwardInput 将玩家输入转发到其正在参与的战斗，无法转发时返回失败回执。
func (c *Client) forwardInput(ctx tree.Context, req *pb.BattleInputReq) {
	uid := req.GetUID()
	if r := c.players[uid]; r != nil && r.Status == Running && r.BattleID == req.GetBattleId() {
		if err := natsrpc.Cast(natsrpc.Battle, def.BattleActorName(r.BattleID),
			r.Node.ServerID, r.Node.NodeID, req); err == nil {
			return
		}
	}
	c.push(ctx, uid, &pb.BattleInputAck{Code: pb_common.ErrCode_INVALID_BATTLE_REQUEST, BattleId: req.GetBattleId(), UnitId: req.GetUnitId(), Sequence: req.GetSequence()})
}

// admissionFailed 释放创建或加入失败的玩家状态。
func (c *Client) admissionFailed(ctx tree.Context, msg *pb.BattleAdmissionFailedNtf) {
	r := c.players[msg.GetUid()]
	if r == nil || (r.BattleID != "" && r.BattleID != msg.GetBattleId()) {
		return
	}
	if r.Status != CreateUnknown && r.Status != Creating {
		return
	}
	if msg.GetCode() == pb_common.ErrCode_OK {
		return
	}

	r.roomRequest = nil
	r.State = State{UID: msg.GetUid(), Status: Idle}
	c.push(ctx, msg.GetUid(), msg)
}

// forwardGrant 将玩法确认的增益送往参战节点，把 Battle 实际处理结果交回调用方。
func (c *Client) forwardGrant(ctx tree.Context, req *sspb.ApplyBattleGrantReq) {
	r := c.players[req.GetUid()]
	if r == nil || r.Status != Running || r.BattleID != req.GetBattleId() {
		ctx.Response(nil, ErrInvalidRequest)
		return
	}
	reply := ctx.RequestEnvelope()
	if reply == nil {
		return
	}
	err := natsrpc.CallSync(
		natsrpc.Game, natsrpc.Battle, def.BattleClient, def.BattleActorName(r.BattleID),
		r.Node.ServerID, r.Node.NodeID, req, ctx.Self(),
		func(_ tree.Context, value interface{}, err error) { reply.Respond(value, err) },
	)
	if err != nil {
		reply.Respond(nil, err)
	}
}

// forwardClaim 将领取请求路由至本人的战斗节点，并把处理结果推回当前会话。
func (c *Client) forwardClaim(ctx tree.Context, req *pb.ClaimBattleGrantReq) {
	uid, battleID := req.GetUID(), req.GetBattleId()
	failure := &pb.ClaimBattleGrantAck{BattleId: battleID, UnitId: req.GetUnitId(), GrantId: req.GetGrantId(), Code: pb_common.ErrCode_INVALID_BATTLE_REQUEST}
	if r := c.players[uid]; r != nil && r.Status == Running && r.BattleID == battleID {
		err := natsrpc.CallSync(
			natsrpc.Game, natsrpc.Battle, def.BattleClient, def.BattleActorName(battleID),
			r.Node.ServerID, r.Node.NodeID, req, ctx.Self(),
			func(cb tree.Context, value interface{}, err error) {
				if ack, ok := value.(*pb.ClaimBattleGrantAck); ok && err == nil {
					c.push(cb, uid, ack)
				} else {
					c.push(cb, uid, failure)
				}
			},
		)
		if err == nil {
			return
		}
	}
	c.push(ctx, uid, failure)
}
