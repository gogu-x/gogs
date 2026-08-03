package base

import (
	"fmt"
	"reflect"
)

// PlayerHandler 处理来自 gate 的玩家请求，上下文携带连接信息与玩家数据。
type PlayerHandler func(*PlayContext, interface{})

// SysHandler 处理其他模块投递或内部异步产生的消息，没有 uid，也不做登录校验。
type SysHandler func(*SysContext, interface{})

type playerRoute struct {
	h PlayerHandler
	// anonymous 为 true 表示该请求在登录前即可处理（PlayContext.Player 可能为 nil）。
	anonymous bool
}

// Dispatcher 维护两张路由表：
//   - playerRoutes：gate 玩家请求（Frame 解码后的消息类型），默认要求玩家已登录；
//   - sysRoutes：其他模块投递 / 内部异步消息，不做登录校验。
//
// 两张表相互独立，同一消息类型可以只出现在其中一张里。
type Dispatcher struct {
	playerRoutes map[reflect.Type]playerRoute
	sysRoutes    map[reflect.Type]SysHandler
}

func (d *Dispatcher) registerPlayer(prototype interface{}, h PlayerHandler, anonymous bool) {
	if d.playerRoutes == nil {
		d.playerRoutes = make(map[reflect.Type]playerRoute)
	}
	t := reflect.TypeOf(prototype)
	if _, dup := d.playerRoutes[t]; dup {
		panic(fmt.Sprintf("base.Dispatcher: player handler already registered for %v", t))
	}
	d.playerRoutes[t] = playerRoute{h: h, anonymous: anonymous}
}

func (d *Dispatcher) registerSys(prototype interface{}, h SysHandler) {
	if d.sysRoutes == nil {
		d.sysRoutes = make(map[reflect.Type]SysHandler)
	}
	t := reflect.TypeOf(prototype)
	if _, dup := d.sysRoutes[t]; dup {
		panic(fmt.Sprintf("base.Dispatcher: system handler already registered for %v", t))
	}
	d.sysRoutes[t] = h
}

func (d *Dispatcher) lookupPlayer(t reflect.Type) (playerRoute, bool) {
	r, ok := d.playerRoutes[t]
	return r, ok
}

func (d *Dispatcher) lookupSys(t reflect.Type) (SysHandler, bool) {
	h, ok := d.sysRoutes[t]
	return h, ok
}

// PlayerRouteCount / SysRouteCount 供启动日志与测试断言使用。
func (d *Dispatcher) PlayerRouteCount() int { return len(d.playerRoutes) }
func (d *Dispatcher) SysRouteCount() int    { return len(d.sysRoutes) }

// IsAnonymous 返回该消息类型是否被注册为免登录路由，未注册时返回 false, false。
func (d *Dispatcher) IsAnonymous(prototype interface{}) (anonymous bool, registered bool) {
	r, ok := d.lookupPlayer(reflect.TypeOf(prototype))
	if !ok {
		return false, false
	}
	return r.anonymous, true
}

// RegisterPlayer 注册需要玩家已登录的 gate 请求，
// handler 拿到的 PlayContext.Player 一定非 nil。
//
//	base.RegisterPlayer(d, &protoChat.ChatReq{}, ChatService)
func RegisterPlayer[Req any](d *Dispatcher, prototype Req, fn func(*PlayContext, Req)) {
	d.registerPlayer(prototype, func(s *PlayContext, msg interface{}) {
		fn(s, msg.(Req))
	}, false)
}

// RegisterAnon 注册免登录的 gate 请求（登录、注册等白名单），
// handler 拿到的 PlayContext.Player 可能为 nil，但 ConnID/GateId 可用于回包。
func RegisterAnon[Req any](d *Dispatcher, prototype Req, fn func(*PlayContext, Req)) {
	d.registerPlayer(prototype, func(s *PlayContext, msg interface{}) {
		fn(s, msg.(Req))
	}, true)
}

// RegisterSys 注册其他模块投递 / 内部异步消息。
// 这类消息没有连接上下文，若需要作用于某个玩家，由 handler 自己从消息参数取 uid：
//
//	base.RegisterSys(d, &protoDeliver.RechargeMsg{}, func(s *base.SysContext, m *protoDeliver.RechargeMsg) {
//	    p := s.Players().Get(m.GetUid())
//	    ...
//	})
func RegisterSys[Msg any](d *Dispatcher, prototype Msg, fn func(*SysContext, Msg)) {
	d.registerSys(prototype, func(s *SysContext, msg interface{}) {
		fn(s, msg.(Msg))
	})
}
