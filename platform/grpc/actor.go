package grpc

import (
	"context"
	"net"
	"time"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/pb/pfpb/pb_pf"
	"github.com/gogu-x/gogs/platform/service"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc"
)

type Platform struct {
	router     tree.Router
	grpcServer *grpc.Server
	db         *mongo.Database
}

func NewActor(db *mongo.Database) *Platform { return &Platform{db: db} }

func (a *Platform) Name() string { return def.PF }

func (a *Platform) OnInit(ctx tree.Context) {
	if err := service.EnsureIndexes(a.db); err != nil {
		tlog.Error("[grpc/Platform.OnInit] 初始化索引失败, err=%v", err)
	}

	a.router.Register(&pb_pf.RegisterReq{}, a.onRegister)
	a.router.Register(&pb_pf.AuthLoginReq{}, a.onLogin)
	a.router.Register(&pb_pf.VerifyTokenReq{}, a.onVerify)
	a.router.Register(&pb_pf.GetServerListReq{}, a.onGetServerList)
	a.router.Register(&pb_pf.CreateOrderReq{}, a.onCreateOrder)
	a.router.Register(&pb_pf.QueryOrderReq{}, a.onQueryOrder)
	a.router.Register(&deliverReq{}, a.onDeliver)

	lis, err := net.Listen("tcp", conf.PlatformAddr)
	if err != nil {
		tlog.Fatal("[grpc/Platform.OnInit] 监听失败, addr=%s err=%v", conf.PlatformAddr, err)
	}
	a.grpcServer = grpc.NewServer()
	svc := &svcHandler{pid: ctx.Self()}
	pb_pf.RegisterAuthServiceServer(a.grpcServer, svc)
	pb_pf.RegisterOrderServiceServer(a.grpcServer, svc)

	go func() {
		tlog.Info("[grpc/Platform.OnInit] 启动gRPC服务, addr=%s", conf.PlatformAddr)
		if err := a.grpcServer.Serve(lis); err != nil {
			tlog.Error("[grpc/Platform.OnInit] gRPC服务退出, err=%v", err)
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
	ack, valid := v.(*pb_pf.AuthAck)
	if err != nil || !valid || ack == nil {
		tlog.Error("[grpc/svcHandler.Register] 平台请求失败, err=%v response=%T", err, v)
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_INTERNAL}, nil
	}
	return ack, nil
}

func (s *svcHandler) Login(_ context.Context, req *pb_pf.AuthLoginReq) (*pb_pf.AuthAck, error) {
	v, err := s.call(req)
	ack, valid := v.(*pb_pf.AuthAck)
	if err != nil || !valid || ack == nil {
		tlog.Error("[grpc/svcHandler.Login] 平台请求失败, err=%v response=%T", err, v)
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_INTERNAL}, nil
	}
	return ack, nil
}

func (s *svcHandler) VerifyToken(_ context.Context, req *pb_pf.VerifyTokenReq) (*pb_pf.VerifyAck, error) {
	v, err := s.call(req)
	ack, valid := v.(*pb_pf.VerifyAck)
	if err != nil || !valid || ack == nil {
		tlog.Error("[grpc/svcHandler.VerifyToken] 平台请求失败, err=%v response=%T", err, v)
		return &pb_pf.VerifyAck{Code: pb_pf.ErrCode_INTERNAL}, nil
	}
	return ack, nil
}

func (s *svcHandler) GetServerList(_ context.Context, req *pb_pf.GetServerListReq) (*pb_pf.ServerListAck, error) {
	v, err := s.call(req)
	ack, valid := v.(*pb_pf.ServerListAck)
	if err != nil || !valid || ack == nil {
		tlog.Error("[grpc/svcHandler.GetServerList] 平台请求失败, err=%v response=%T", err, v)
		return &pb_pf.ServerListAck{Code: pb_pf.ErrCode_INTERNAL}, nil
	}
	return ack, nil
}

func (s *svcHandler) CreateOrder(_ context.Context, req *pb_pf.CreateOrderReq) (*pb_pf.OrderAck, error) {
	v, err := s.call(req)
	ack, valid := v.(*pb_pf.OrderAck)
	if err != nil || !valid || ack == nil {
		tlog.Error("[grpc/svcHandler.CreateOrder] 平台请求失败, err=%v response=%T", err, v)
		return &pb_pf.OrderAck{Code: pb_pf.ErrCode_INTERNAL}, nil
	}
	return ack, nil
}

func (s *svcHandler) QueryOrder(_ context.Context, req *pb_pf.QueryOrderReq) (*pb_pf.OrderDetail, error) {
	v, err := s.call(req)
	ack, valid := v.(*pb_pf.OrderDetail)
	if err != nil || !valid || ack == nil {
		tlog.Error("[grpc/svcHandler.QueryOrder] 平台请求失败, err=%v response=%T", err, v)
		return &pb_pf.OrderDetail{Code: pb_pf.ErrCode_INTERNAL}, nil
	}
	return ack, nil
}
