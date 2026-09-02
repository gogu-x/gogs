package platformrpc

import "github.com/gogu-x/gogs/pb/pb_pf"

func registerRoutes(a *Actor) {
	a.register(&pb_pf.RegisterReq{},
		pb_pf.AuthService_Register_FullMethodName,
		func() any { return &pb_pf.AuthAck{} })

	a.register(&pb_pf.AuthLoginReq{},
		pb_pf.AuthService_Login_FullMethodName,
		func() any { return &pb_pf.AuthAck{} })

	a.register(&pb_pf.VerifyTokenReq{},
		pb_pf.AuthService_VerifyToken_FullMethodName,
		func() any { return &pb_pf.VerifyAck{} })

	a.register(&pb_pf.GetServerListReq{},
		pb_pf.AuthService_GetServerList_FullMethodName,
		func() any { return &pb_pf.ServerListAck{} })

	a.register(&pb_pf.CreateOrderReq{},
		pb_pf.OrderService_CreateOrder_FullMethodName,
		func() any { return &pb_pf.OrderAck{} })

	a.register(&pb_pf.QueryOrderReq{},
		pb_pf.OrderService_QueryOrder_FullMethodName,
		func() any { return &pb_pf.OrderDetail{} })
}
