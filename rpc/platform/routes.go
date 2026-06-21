package platform

import "github.com/gogu-x/gogs/pb/protoPlatform"

func registerRoutes(a *Actor) {
	a.register(&protoPlatform.RegisterReq{},
		protoPlatform.AuthService_Register_FullMethodName,
		func() any { return &protoPlatform.AuthAck{} })

	a.register(&protoPlatform.AuthLoginReq{},
		protoPlatform.AuthService_Login_FullMethodName,
		func() any { return &protoPlatform.AuthAck{} })

	a.register(&protoPlatform.VerifyTokenReq{},
		protoPlatform.AuthService_VerifyToken_FullMethodName,
		func() any { return &protoPlatform.VerifyAck{} })

	a.register(&protoPlatform.CreateOrderReq{},
		protoPlatform.OrderService_CreateOrder_FullMethodName,
		func() any { return &protoPlatform.OrderAck{} })

	a.register(&protoPlatform.QueryOrderReq{},
		protoPlatform.OrderService_QueryOrder_FullMethodName,
		func() any { return &protoPlatform.OrderDetail{} })
}
