package battle

import (
	"fmt"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"strconv"

	"github.com/gogu-x/gogs/def"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/natsrpc"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/cluster"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

type PlayerState uint8

const (
	Idle PlayerState = iota
	Creating
	CreateUnknown
	Running
	Finished
)

func (s PlayerState) String() string {
	switch s {
	case Idle:
		return "Idle"
	case Creating:
		return "Creating"
	case CreateUnknown:
		return "CreateUnknown"
	case Running:
		return "Running"
	case Finished:
		return "Finished"
	default:
		return "Unknown"
	}
}

type Node struct {
	ServerID int
	NodeID   int
}

type NodeSelector interface {
	Pick() (Node, error)
}

type Sender interface {
	Start(Node, *pb.StartBattleReq) error
	Confirm(Node, *pb.BattleResultConfirmedNtf) error
}

type Begin struct {
	Request *pb.StartBattleReq
}

type Reset struct{ UID uint64 }
type GetState struct{ UID uint64 }

type State struct {
	UID          uint64
	Status       PlayerState
	BattleID     string
	Node         Node
	LastSequence uint64
	Checksum     string
}

type record struct {
	State
	roomRequest   *pb.StartBattleReq // 本场共享战斗的权威入场快照，用于缺号时重同步。
	snapshotFloor uint64
	seen          map[uint64]struct{}
	playback      *playbackWindow // 服务端维护的演出准入窗口，不依赖客户端完成上报。
}

type completionKey struct {
	battleID string
	uid      uint64
}

type Client struct {
	selector      NodeSelector
	sender        Sender
	gameServerID  int
	gameNodeID    int
	players       map[uint64]*record
	completed     map[completionKey]Node
	serverOpened  bool
	serverClosing bool
}

func New(selector NodeSelector, sender Sender, gameServerID, gameNodeID int) *Client {
	if selector == nil {
		selector = &ClusterSelector{}
	}
	if sender == nil {
		sender = NATSSender{}
	}
	return &Client{
		selector: selector, sender: sender,
		gameServerID: gameServerID, gameNodeID: gameNodeID,
		players: make(map[uint64]*record), completed: make(map[completionKey]Node),
	}
}

func (c *Client) Name() string        { return def.BattleClient }
func (c *Client) OnInit(tree.Context) {}
func (c *Client) OnStop(tree.Context) {}
func (c *Client) MailboxSize() int    { return 1000000 }

func (c *Client) HandleMessage(ctx tree.Context, msg interface{}) {
	switch m := msg.(type) {
	case *sspb.Event:
		if gameevent.IsServerOpened(m) {
			c.serverOpened = true
			c.serverClosing = false
			tlog.Log.Info("[battle/Client.HandleMessage] 战斗客户端已处理开服事件")
			ctx.Response(nil, nil)
			return
		}
		if !gameevent.IsServerClosing(m) {
			eventName := m.GetName()
			err := fmt.Errorf("unsupported battle event %q", eventName)
			tlog.Log.Error("[battle/Client.HandleMessage] 收到不支持的战斗事件, event=%v err=%v", eventName, err)
			ctx.Response(nil, err)
			return
		}
		c.serverClosing = true
		tlog.Log.Info("[battle/Client.HandleMessage] 战斗客户端已处理关服事件")
		ctx.Response(nil, nil)
	case *Begin:
		c.begin(ctx, m)
	case *Reset:
		c.reset(ctx, m)
	case *GetState:
		c.getState(ctx, m)
	case *pb.BattleInputReq:
		c.forwardInput(ctx, m)
	case *pb.BattleInputAck:
		if r := c.players[m.GetUid()]; r != nil && r.Status == Running && r.BattleID == m.GetBattleId() {
			c.push(ctx, m.GetUid(), m)
		}
	case *pb.BattleAdmissionFailedNtf:
		c.admissionFailed(ctx, m)
	case *pb.BattleCreatedNtf:
		c.created(ctx, m)
	case *pb.BattleActionNtf:
		c.action(ctx, m)
	case *pb.BattleFinishedNtf:
		c.finished(ctx, m)
	default:
		ctx.Response(nil, fmt.Errorf("battle client: unsupported message %T", msg))
	}
}

func (c *Client) state(uid uint64) *record {
	r := c.players[uid]
	if r == nil {
		r = &record{State: State{UID: uid, Status: Idle}, seen: make(map[uint64]struct{})}
		c.players[uid] = r
	}
	return r
}

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
		r.playback = newPlaybackWindow(msg.GetTickDurationMs())
	}
	c.push(ctx, msg.Uid, msg)
}

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

// buildSettlement 将当前收件人的结果投递给其玩家 Actor，每名参战者独立去重和确认。
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

// ClusterSelector 从通用服务注册表轮询 Battle 节点。
type ClusterSelector struct{ next uint64 }

func (s *ClusterSelector) Pick() (Node, error) {
	instances, err := cluster.GetAllServices(def.ServiceBattle)
	if err != nil {
		return Node{}, err
	}
	if len(instances) == 0 {
		return Node{}, fmt.Errorf("battle client: no battle service nodes")
	}
	instance := instances[s.next%uint64(len(instances))]
	s.next++
	serverID, err := strconv.Atoi(instance.ServerID)
	if err != nil {
		return Node{}, fmt.Errorf("battle client: invalid server id %q", instance.ServerID)
	}
	nodeID, err := strconv.Atoi(instance.NodeID)
	if err != nil {
		return Node{}, fmt.Errorf("battle client: invalid node id %q", instance.NodeID)
	}
	return Node{ServerID: serverID, NodeID: nodeID}, nil
}

type NATSSender struct{}

func (NATSSender) Start(node Node, req *pb.StartBattleReq) error {
	if req.GetJoinBattleId() != "" {
		services, err := cluster.GetAllServices(def.ServiceBattle)
		if err != nil {
			return err
		}
		registered := false
		for _, instance := range services {
			if instance.ServerID == strconv.Itoa(node.ServerID) && instance.NodeID == strconv.Itoa(node.NodeID) {
				registered = true
				break
			}
		}
		if !registered {
			return ErrUnavailable
		}
	}
	return natsrpc.Cast(natsrpc.Battle, def.BattleService, node.ServerID, node.NodeID, req)
}

func (NATSSender) Confirm(node Node, msg *pb.BattleResultConfirmedNtf) error {
	return natsrpc.Cast(natsrpc.Battle, def.BattleActorName(msg.BattleId), node.ServerID, node.NodeID, msg)
}

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
