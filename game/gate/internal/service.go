package internal

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/tree"
	"google.golang.org/grpc"
)

// GateActor serializes the lifecycle and routing state of all gateway
// connection agents owned by this Game process.
type GateActor struct {
	grpcServer *grpc.Server
	router     tree.Router
	sessions   map[sessionKey]session
	bannedUIDs map[uint64]struct{}
	nextGen    uint64
}

func NewGateActor() *GateActor {
	return &GateActor{
		sessions:   make(map[sessionKey]session),
		bannedUIDs: make(map[uint64]struct{}),
	}
}

func (g *GateActor) Name() string { return constant.Gate }

func (g *GateActor) OnInit(ctx tree.Context) {
	g.router.Register(&openSession{}, g.onOpenSession)
	g.router.Register(&closeSession{}, g.onCloseSession)
	g.router.Register(&sessionUID{}, g.onSessionUID)
	g.router.Register(&PushToConn{}, g.onPushToConn)
	g.router.Register(&BanUID{}, g.onBanUID)
	g.router.Register(&UnbanUID{}, g.onUnbanUID)

	lis, err := net.Listen("tcp", config.GameAddr())
	if err != nil {
		log.Fatalf("GateActor: listen error: %v", err)
	}

	g.grpcServer = grpc.NewServer()
	protoGateway.RegisterGatewayServer(g.grpcServer, &gatewayService{
		actorPID: ctx.Self(),
		system:   ctx.System(),
	})

	go func() {
		log.Printf("GateActor: gRPC server listening on %s", config.GameAddr())
		if err := g.grpcServer.Serve(lis); err != nil {
			log.Printf("GateActor: grpc serve error: %v", err)
		}
	}()

	if err := cluster.Register(fmt.Sprintf("%d", config.ServerID), fmt.Sprintf("%d", os.Getpid()), config.GameAddr()); err != nil {
		log.Printf("GateActor: cluster register error: %v", err)
	} else {
		log.Printf("GateActor: registered [%d] -> %s", config.ServerID, config.GameAddr())
	}
}

func (g *GateActor) HandleMessage(ctx tree.Context, msg interface{}) {
	g.router.Route(ctx, msg)
}

func (g *GateActor) OnStop(ctx tree.Context) {
	for key, current := range g.sessions {
		delete(g.sessions, key)
		ctx.Send(current.pid, &stopAgent{})
	}
	if g.grpcServer != nil {
		g.grpcServer.Stop()
	}
}

func (g *GateActor) onOpenSession(ctx tree.Context, msg interface{}) {
	open := msg.(*openSession)
	if err := validateFrame(open.first); err != nil {
		ctx.Response(nil, err)
		return
	}

	key := sessionKey{gateID: open.first.GetGateId(), connID: open.first.GetConnId()}
	if previous, ok := g.sessions[key]; ok {
		// A reconnect replaces the old stream. Its delayed close event carries
		// the old generation and cannot remove this replacement.
		ctx.Send(previous.pid, &stopAgent{})
	}

	g.nextGen++
	generation := g.nextGen
	agent := newGsAgent(key, generation, open.stream, ctx.Self())
	pid := ctx.System().SpawnOne(agent)
	g.sessions[key] = session{pid: pid, generation: generation}

	ctx.Send(pid, &inboundFrame{frame: open.first})
	ctx.Response(&openedSession{key: key, pid: pid, generation: generation}, nil)
}

func (g *GateActor) onCloseSession(ctx tree.Context, msg interface{}) {
	c := msg.(*closeSession)
	current, ok := g.sessions[c.key]
	if !ok || current.pid != c.pid || current.generation != c.generation {
		return
	}

	delete(g.sessions, c.key)
	ctx.Send(current.pid, &stopAgent{})
	if current.uid != 0 {
		playPID, exists := ctx.Lookup(constant.PLAY)
		if exists {
			ctx.Send(playPID, &protoGateway.Frame{
				Uid:     current.uid,
				GateId:  c.key.gateID,
				ConnId:  c.key.connID,
				MsgType: natsrpc.MsgTypeDisconnect,
			})
		}
	}
	log.Printf("GateActor: closed connection %s: %s", c.key, c.reason)
}

func (g *GateActor) onSessionUID(ctx tree.Context, msg interface{}) {
	update := msg.(*sessionUID)
	current, ok := g.sessions[update.key]
	if !ok || current.pid != update.pid || current.generation != update.generation {
		ctx.Response(false, nil)
		return
	}
	if _, banned := g.bannedUIDs[update.uid]; banned {
		g.closeCurrent(ctx, update.key, current, "user is banned")
		ctx.Response(false, nil)
		return
	}
	current.uid = update.uid
	g.sessions[update.key] = current
	ctx.Response(true, nil)
}

func (g *GateActor) onPushToConn(ctx tree.Context, msg interface{}) {
	push := msg.(*PushToConn)
	if push.Frame == nil {
		return
	}
	key := sessionKey{gateID: push.GateID, connID: push.ConnID}
	current, ok := g.sessions[key]
	if !ok {
		return
	}
	if !ctx.TrySend(current.pid, &outboundFrame{frame: push.Frame}) {
		g.closeCurrent(ctx, key, current, "outbound mailbox is full")
	}
}

func (g *GateActor) onBanUID(ctx tree.Context, msg interface{}) {
	uid := msg.(*BanUID).UID
	if uid == 0 {
		return
	}
	g.bannedUIDs[uid] = struct{}{}
	for key, current := range g.sessions {
		if current.uid == uid {
			g.closeCurrent(ctx, key, current, "user is banned")
		}
	}
}

func (g *GateActor) onUnbanUID(_ tree.Context, msg interface{}) {
	delete(g.bannedUIDs, msg.(*UnbanUID).UID)
}

func (g *GateActor) closeCurrent(ctx tree.Context, key sessionKey, current session, reason string) {
	delete(g.sessions, key)
	ctx.Send(current.pid, &stopAgent{})
	if current.uid == 0 {
		return
	}
	if playPID, ok := ctx.Lookup(constant.PLAY); ok {
		ctx.Send(playPID, &protoGateway.Frame{
			Uid:     current.uid,
			GateId:  key.gateID,
			ConnId:  key.connID,
			MsgType: natsrpc.MsgTypeDisconnect,
		})
	}
	log.Printf("GateActor: closed connection %s: %s", key, reason)
}

func validateFrame(frame *protoGateway.Frame) error {
	if frame == nil {
		return fmt.Errorf("empty gateway frame")
	}
	if frame.GetGateId() == "" || frame.GetConnId() == 0 {
		return fmt.Errorf("missing gateway connection identity")
	}
	if frame.GetServerId() != int32(config.ServerID) {
		return fmt.Errorf("frame targets server %d, local server is %d", frame.GetServerId(), config.ServerID)
	}
	return nil
}
