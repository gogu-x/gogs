package battleclient

import (
	"fmt"
	"strconv"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/natsrpc"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/cluster"
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
	seen map[uint64]struct{}
}

type Client struct {
	selector     NodeSelector
	sender       Sender
	gameServerID int
	gameNodeID   int
	players      map[uint64]*record
	completed    map[string]string
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
		players: make(map[uint64]*record), completed: make(map[string]string),
	}
}

func (c *Client) Name() string        { return def.BattleClient }
func (c *Client) OnInit(tree.Context) {}
func (c *Client) OnStop(tree.Context) {}
func (c *Client) MailboxSize() int    { return 8 * 1024 }

func (c *Client) HandleMessage(ctx tree.Context, msg interface{}) {
	switch m := msg.(type) {
	case *Begin:
		c.begin(ctx, m)
	case *Reset:
		c.reset(ctx, m)
	case *GetState:
		c.getState(ctx, m)
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
	if msg == nil || msg.Request == nil || msg.Request.GetUID() == 0 {
		ctx.Response(nil, fmt.Errorf("battle client: request and uid are required"))
		return
	}
	request := proto.Clone(msg.Request).(*pb.StartBattleReq)
	uid := request.GetUID()
	r := c.state(uid)
	if r.Status == Creating || r.Status == CreateUnknown || r.Status == Running {
		ctx.Response(r.State, fmt.Errorf("battle client: uid=%d is %s", uid, r.Status))
		return
	}
	node, err := c.selector.Pick()
	if err != nil {
		r.Status = Idle
		ctx.Response(r.State, err)
		return
	}
	r.State = State{UID: uid, Status: Creating, Node: node}
	r.seen = make(map[uint64]struct{})
	request.ServerID = uint32(c.gameServerID)
	request.SourceNodeId = uint32(c.gameNodeID)
	if err := c.sender.Start(node, request); err != nil {
		r.Status = Idle
		ctx.Response(r.State, err)
		return
	}
	// NATS 已发布但尚未收到 Created；此时禁止盲目重发，避免生成两场战斗。
	r.Status = CreateUnknown
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
	r.BattleID = msg.BattleId
	r.Status = Running
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
	if sequence == 0 {
		return
	}
	if _, duplicate := r.seen[sequence]; duplicate {
		return
	}
	r.seen[sequence] = struct{}{}
	if sequence > r.LastSequence {
		r.LastSequence = sequence
	}
	c.push(ctx, msg.Uid, msg)
}

func (c *Client) finished(ctx tree.Context, msg *pb.BattleFinishedNtf) {
	if msg == nil || msg.Uid == 0 || msg.BattleId == "" {
		return
	}
	r := c.state(msg.Uid)
	if r.BattleID != "" && r.BattleID != msg.BattleId {
		return
	}
	// Finished 可恢复丢失的 Created；重复 Finished 只确认，不重复推给客户端。
	if _, alreadyApplied := c.completed[msg.BattleId]; !alreadyApplied {
		c.completed[msg.BattleId] = msg.Checksum
		r.BattleID = msg.BattleId
		r.Checksum = msg.Checksum
		r.Status = Finished
		c.push(ctx, msg.Uid, msg)
	}
	_ = c.sender.Confirm(r.Node, &pb.BattleResultConfirmedNtf{BattleId: msg.BattleId, Uid: msg.Uid})
}

func (c *Client) reset(ctx tree.Context, msg *Reset) {
	if msg == nil || msg.UID == 0 {
		ctx.Response(nil, fmt.Errorf("battle client: uid is required"))
		return
	}
	r := c.state(msg.UID)
	if r.Status == Creating || r.Status == CreateUnknown || r.Status == Running {
		ctx.Response(r.State, fmt.Errorf("battle client: cannot reset %s", r.Status))
		return
	}
	r.State = State{UID: msg.UID, Status: Idle}
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

func (c *Client) push(ctx tree.Context, uid uint64, msg proto.Message) {
	if gate, ok := ctx.Lookup(def.GameGate); ok {
		ctx.Send(gate, &ipb.PushToMsg{UID: uid, Msg: msg})
	}
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
	return natsrpc.Cast(natsrpc.Battle, def.BattleService, node.ServerID, node.NodeID, req)
}

func (NATSSender) Confirm(node Node, msg *pb.BattleResultConfirmedNtf) error {
	return natsrpc.Cast(natsrpc.Battle, def.BattleActorName(msg.BattleId), node.ServerID, node.NodeID, msg)
}
