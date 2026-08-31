package internal

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/constant"
	gamemsg "github.com/gogu-x/gogs/game/message"
	"github.com/gogu-x/gogs/pb/pb_auth"
	"github.com/gogu-x/gogs/pb/pb_gateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/cluster"
	"google.golang.org/grpc"
)

// GateActor serializes the lifecycle and routing state of all gateway streams.
type GateActor struct {
	grpcServer *grpc.Server
	router     tree.Router
	sessions   map[uint64]session
	bannedUIDs map[uint64]struct{}
	nextGen    uint64
}

func NewGateActor() *GateActor {
	return &GateActor{
		sessions:   make(map[uint64]session),
		bannedUIDs: make(map[uint64]struct{}),
	}
}

func (g *GateActor) Name() string { return constant.Gate }

func (g *GateActor) OnInit(ctx tree.Context) {
	g.router.Register(&openSession{}, g.onOpenSession)
	g.router.Register(&closeSession{}, g.onCloseSession)
	g.router.Register(&PushToMsg{}, g.onPushToMsg)
	g.router.Register(&BanUID{}, g.onBanUID)
	g.router.Register(&UnbanUID{}, g.onUnbanUID)

	lis, err := net.Listen("tcp", config.GameAddr())
	if err != nil {
		log.Fatalf("GateActor: listen error: %v", err)
	}

	g.grpcServer = grpc.NewServer()
	pb_gateway.RegisterGatewayServer(g.grpcServer, &gatewayService{
		actorPID: ctx.Self(),
		system:   ctx.System(),
	})

	go func() {
		log.Printf("GateActor: gRPC server listening on %s", config.GameAddr())
		if err := g.grpcServer.Serve(lis); err != nil {
			log.Printf("GateActor: grpc serve error: %v", err)
		}
	}()

	if err := cluster.Register(
		fmt.Sprintf("%d", config.ServerID),
		fmt.Sprintf("%d", os.Getpid()),
		config.GameAddr(),
	); err != nil {
		log.Printf("GateActor: cluster register error: %v", err)
	} else {
		log.Printf("GateActor: registered [%d] -> %s", config.ServerID, config.GameAddr())
	}
}

func (g *GateActor) HandleMessage(ctx tree.Context, msg interface{}) {
	g.router.Route(ctx, msg)
}

func (g *GateActor) OnStop(ctx tree.Context) {
	for uid, current := range g.sessions {
		delete(g.sessions, uid)
		ctx.Send(current.pid, &stopAgent{})
	}
	if g.grpcServer != nil {
		g.grpcServer.Stop()
	}
}

func (g *GateActor) onOpenSession(ctx tree.Context, msg interface{}) {
	open := msg.(*openSession)
	first, uid, err := decodeInboundFrame(open.frame)
	if err != nil {
		ctx.Response(nil, err)
		return
	}
	if _, ok := first.(*pb_auth.LoginGameReq); !ok {
		ctx.Response(nil, fmt.Errorf("first stream message must be LoginGameReq, got %T", first))
		return
	}
	if _, banned := g.bannedUIDs[uid]; banned {
		ctx.Response(nil, fmt.Errorf("uid %d is banned", uid))
		return
	}

	if previous, ok := g.sessions[uid]; ok {
		ctx.Send(previous.pid, &stopAgent{})
	}

	g.nextGen++
	generation := g.nextGen
	agent := newGsAgent(uid, generation, open.stream, ctx.Self())
	pid := ctx.System().SpawnOne(agent)
	g.sessions[uid] = session{pid: pid, generation: generation, uid: uid}

	ctx.Send(pid, &inboundFrame{frame: open.frame})
	ctx.Response(&openedSession{pid: pid, uid: uid, generation: generation}, nil)
}

func (g *GateActor) onCloseSession(ctx tree.Context, msg interface{}) {
	closed := msg.(*closeSession)
	current, ok := g.sessions[closed.uid]
	if !ok || current.pid != closed.pid || current.generation != closed.generation {
		return
	}

	delete(g.sessions, closed.uid)
	ctx.Send(current.pid, &stopAgent{})
	g.notifySessionClosed(ctx, closed.uid)
	log.Printf("GateActor: closed UID %d: %s", closed.uid, closed.reason)
}

func (g *GateActor) onPushToMsg(ctx tree.Context, msg interface{}) {
	push := msg.(*PushToMsg)
	if push.Frame == nil {
		return
	}
	current, ok := g.sessions[push.UID]
	if !ok {
		return
	}
	if !ctx.TrySend(current.pid, push.Frame) {
		g.closeCurrent(ctx, push.UID, current, "outbound mailbox is full")
	}
}

func (g *GateActor) onBanUID(ctx tree.Context, msg interface{}) {
	uid := msg.(*BanUID).UID
	if uid == 0 {
		return
	}
	g.bannedUIDs[uid] = struct{}{}
	if current, ok := g.sessions[uid]; ok {
		g.closeCurrent(ctx, uid, current, "user is banned")
	}
}

func (g *GateActor) onUnbanUID(_ tree.Context, msg interface{}) {
	delete(g.bannedUIDs, msg.(*UnbanUID).UID)
}

func (g *GateActor) closeCurrent(
	ctx tree.Context,
	uid uint64,
	current session,
	reason string,
) {
	delete(g.sessions, uid)
	ctx.Send(current.pid, &stopAgent{})
	g.notifySessionClosed(ctx, uid)
	log.Printf("GateActor: closed UID %d: %s", uid, reason)
}

func (g *GateActor) notifySessionClosed(ctx tree.Context, uid uint64) {
	if playPID, ok := ctx.Lookup(constant.PLAY); ok {
		ctx.Send(playPID, &gamemsg.SessionClosed{UID: uid})
	}
}

func validateFrame(frame *pb_gateway.Frame) error {
	if frame == nil {
		return fmt.Errorf("empty gateway frame")
	}
	if len(frame.GetPayload()) == 0 {
		return fmt.Errorf("empty gateway payload")
	}
	return nil
}
