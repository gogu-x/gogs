package grpc

import (
	"context"
	"log"
	"net"
	"time"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/pb/protoPlatform"
	"github.com/gogu-x/gogs/platform/service"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc"
)

type Actor struct {
	router     actor.Router
	grpcServer *grpc.Server
	db         *mongo.Database
}

func NewActor(db *mongo.Database) *Actor { return &Actor{db: db} }

func (a *Actor) OnInit(ctx actor.ActorContext) {
	if err := service.EnsureIndexes(a.db); err != nil {
		log.Printf("GrpcActor: EnsureIndexes: %v", err)
	}

	a.router.Register(&protoPlatform.RegisterReq{}, a.onRegister)
	a.router.Register(&protoPlatform.AuthLoginReq{}, a.onLogin)
	a.router.Register(&protoPlatform.VerifyTokenReq{}, a.onVerify)
	a.router.Register(&protoPlatform.CreateOrderReq{}, a.onCreateOrder)
	a.router.Register(&protoPlatform.QueryOrderReq{}, a.onQueryOrder)
	a.router.Register(&deliverReq{}, a.onDeliver)

	lis, err := net.Listen("tcp", config.PlatformAddr)
	if err != nil {
		log.Fatalf("GrpcActor: listen %s: %v", config.PlatformAddr, err)
	}
	a.grpcServer = grpc.NewServer()
	svc := &svcHandler{pid: ctx.Self()}
	protoPlatform.RegisterAuthServiceServer(a.grpcServer, svc)
	protoPlatform.RegisterOrderServiceServer(a.grpcServer, svc)

	go func() {
		log.Printf("platform gRPC listening on %s", config.PlatformAddr)
		if err := a.grpcServer.Serve(lis); err != nil {
			log.Printf("GrpcActor: serve error: %v", err)
		}
	}()
}

func (a *Actor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	a.router.Route(ctx, msg)
}

func (a *Actor) OnStop(_ actor.ActorContext) {
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
	}
}

type svcHandler struct {
	protoPlatform.UnimplementedAuthServiceServer
	protoPlatform.UnimplementedOrderServiceServer
	pid actor.PID
}

func (s *svcHandler) call(msg interface{}) (interface{}, error) {
	return actor.Default().Request(s.pid, msg).AwaitTimeout(5 * time.Second)
}

func (s *svcHandler) Register(_ context.Context, req *protoPlatform.RegisterReq) (*protoPlatform.AuthAck, error) {
	v, err := s.call(req)
	if err != nil {
		return nil, err
	}
	return v.(*protoPlatform.AuthAck), nil
}

func (s *svcHandler) Login(_ context.Context, req *protoPlatform.AuthLoginReq) (*protoPlatform.AuthAck, error) {
	v, err := s.call(req)
	if err != nil {
		return nil, err
	}
	return v.(*protoPlatform.AuthAck), nil
}

func (s *svcHandler) VerifyToken(_ context.Context, req *protoPlatform.VerifyTokenReq) (*protoPlatform.VerifyAck, error) {
	v, err := s.call(req)
	if err != nil {
		return nil, err
	}
	return v.(*protoPlatform.VerifyAck), nil
}

func (s *svcHandler) CreateOrder(_ context.Context, req *protoPlatform.CreateOrderReq) (*protoPlatform.OrderAck, error) {
	v, err := s.call(req)
	if err != nil {
		return nil, err
	}
	return v.(*protoPlatform.OrderAck), nil
}

func (s *svcHandler) QueryOrder(_ context.Context, req *protoPlatform.QueryOrderReq) (*protoPlatform.OrderDetail, error) {
	v, err := s.call(req)
	if err != nil {
		return nil, err
	}
	return v.(*protoPlatform.OrderDetail), nil
}
