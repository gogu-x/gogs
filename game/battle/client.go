package battle

import (
	"fmt"
	"strconv"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/natsrpc"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/cluster"
)

type PlayerState uint8

const (
	Idle PlayerState = iota
	Creating
	CreateUnknown
	Running
	Finished
)

// String 返回玩家战斗状态的可读名称。
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
	router        tree.Router // 按消息类型分发到已注册的处理方法。
	selector      NodeSelector
	sender        Sender
	gameServerID  int
	gameNodeID    int
	players       map[uint64]*record
	completed     map[completionKey]Node
	serverOpened  bool
	serverClosing bool
}

// New 创建 Game 侧战斗客户端，默认使用服务发现选点并通过 NATS 发送请求。
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

// Name 返回用于 Actor 寻址的战斗客户端注册名。
func (c *Client) Name() string { return def.BattleClient }

// OnInit 在接收业务消息前完成路由注册。
func (c *Client) OnInit(tree.Context) { c.registerMessages() }

// OnStop 实现 Actor 的停止回调，当前无需释放独占资源。
func (c *Client) OnStop(tree.Context) {}

// MailboxSize 返回战斗客户端接收全区消息的邮箱容量。
func (c *Client) MailboxSize() int { return 1000000 }

// HandleMessage 将收到的消息交给已注册的类型路由。
func (c *Client) HandleMessage(ctx tree.Context, msg interface{}) {
	c.router.Route(ctx, msg)
}

// ClusterSelector 从通用服务注册表轮询 Battle 节点。
type ClusterSelector struct{ next uint64 }

// Pick 轮询已注册的 Battle 实例并返回目标节点地址。
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

// Start 向目标 Battle 管理器发送创建或加入请求。
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

// Confirm 将结算确认发回对应战斗实例。
func (NATSSender) Confirm(node Node, msg *pb.BattleResultConfirmedNtf) error {
	return natsrpc.Cast(natsrpc.Battle, def.BattleActorName(msg.BattleId), node.ServerID, node.NodeID, msg)
}
