package grpc

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/pb/pb_pf"
	"github.com/gogu-x/gogs/platform/service"
	"github.com/gogu-x/tree"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc"
)

type Platform struct {
	router     tree.Router
	grpcServer *grpc.Server
	db         *mongo.Database
}

func NewActor(db *mongo.Database) *Platform { return &Platform{db: db} }

func (a *Platform) Name() string { return constant.PF }

func (a *Platform) OnInit(ctx tree.Context) {
	if err := service.EnsureIndexes(a.db); err != nil {
		log.Printf("GrpcActor: EnsureIndexes: %v", err)
	}

	a.router.Register(&pb_pf.RegisterReq{}, a.onRegister)
	a.router.Register(&pb_pf.AuthLoginReq{}, a.onLogin)
	a.router.Register(&pb_pf.VerifyTokenReq{}, a.onVerify)
	a.router.Register(&pb_pf.GetServerListReq{}, a.onGetServerList)
	a.router.Register(&pb_pf.CreateOrderReq{}, a.onCreateOrder)
	a.router.Register(&pb_pf.QueryOrderReq{}, a.onQueryOrder)

	lis, err := net.Listen("tcp", config.PlatformAddr)
	if err != nil {
		log.Fatalf("GrpcActor: listen %s: %v", config.PlatformAddr, err)
	}
	a.grpcServer = grpc.NewServer()
	svc := &svcHandler{pid: ctx.Self()}
	pb_pf.RegisterAuthServiceServer(a.grpcServer, svc)
	pb_pf.RegisterOrderServiceServer(a.grpcServer, svc)

	go func() {
		log.Printf("platform gRPC listening on %s", config.PlatformAddr)
		if err := a.grpcServer.Serve(lis); err != nil {
			log.Printf("GrpcActor: serve error: %v", err)
		}
	}()
}

func (a *Platform) HandleMessage(ctx tree.Context, msg interface{}) {
	a.router.Route(ctx, msg)
}

func (a *Platform) OnStop(_ tree.Context) {
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
	}
}

type svcHandler struct {
	pb_pf.UnimplementedAuthServiceServer
	pb_pf.UnimplementedOrderServiceServer
	pid tree.PID
}

func (s *svcHandler) call(msg interface{}) (interface{}, error) {
	return tree.Default().Request(s.pid, msg).AwaitTimeout(5 * time.Second)
}

func (s *svcHandler) Register(_ context.Context, req *pb_pf.RegisterReq) (*pb_pf.AuthAck, error) {
	v, err := s.call(req)
	if err != nil {
		return nil, err
	}
	return v.(*pb_pf.AuthAck), nil
}

func (s *svcHandler) Login(_ context.Context, req *pb_pf.AuthLoginReq) (*pb_pf.AuthAck, error) {
	v, err := s.call(req)
	if err != nil {
		return nil, err
	}
	return v.(*pb_pf.AuthAck), nil
}

func (s *svcHandler) VerifyToken(_ context.Context, req *pb_pf.VerifyTokenReq) (*pb_pf.VerifyAck, error) {
	v, err := s.call(req)
	if err != nil {
		return nil, err
	}
	return v.(*pb_pf.VerifyAck), nil
}

func (s *svcHandler) GetServerList(_ context.Context, req *pb_pf.GetServerListReq) (*pb_pf.ServerListAck, error) {
	v, err := s.call(req)
	if err != nil {
		return nil, err
	}
	return v.(*pb_pf.ServerListAck), nil
}

func (s *svcHandler) CreateOrder(_ context.Context, req *pb_pf.CreateOrderReq) (*pb_pf.OrderAck, error) {
	v, err := s.call(req)
	if err != nil {
		return nil, err
	}
	return v.(*pb_pf.OrderAck), nil
}

func (s *svcHandler) QueryOrder(_ context.Context, req *pb_pf.QueryOrderReq) (*pb_pf.OrderDetail, error) {
	v, err := s.call(req)
	if err != nil {
		return nil, err
	}
	return v.(*pb_pf.OrderDetail), nil
}
