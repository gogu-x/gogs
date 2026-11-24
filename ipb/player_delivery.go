package ipb

import "github.com/gogu-x/tree"

// DeliveryWithReply 是网关转交给玩家 Actor 的、需要回复调用方的业务载荷。
//
// 网关在排队、重投、唤醒加载和关服交接期间始终传递同一个对象，因此原调用方的回复目标
// 不会丢失；玩家 Actor 处理完成后通过 ReplyContext 把实际结果回给原调用方。
type DeliveryWithReply struct {
	payload interface{}
	reply   *tree.Envelope
}

// WrapWithReply 将业务载荷与原调用方的回复目标绑定在一起。
func WrapWithReply(payload interface{}, reply *tree.Envelope) *DeliveryWithReply {
	return &DeliveryWithReply{payload: payload, reply: reply}
}

// GetPayload 返回交给玩家业务路由处理的原始载荷。
func (d *DeliveryWithReply) GetPayload() interface{} { return d.payload }

// Reply 将结果回给原调用方；只有第一次调用生效。
func (d *DeliveryWithReply) Reply(value interface{}, err error) { d.reply.Respond(value, err) }

// ReplyContext 返回在玩家 Actor 中执行业务时使用的上下文：除回复外均沿用玩家 Actor
// 的上下文，ctx.Response 会回到原调用方而不是网关。
func (d *DeliveryWithReply) ReplyContext(ctx tree.Context) tree.Context {
	return deliveryReplyContext{Context: ctx, delivery: d}
}

type deliveryReplyContext struct {
	tree.Context
	delivery *DeliveryWithReply
}

func (c deliveryReplyContext) Message() interface{}            { return c.delivery.payload }
func (c deliveryReplyContext) Sender() tree.PID                { return c.delivery.reply.Sender }
func (c deliveryReplyContext) RequestEnvelope() *tree.Envelope { return c.delivery.reply }
func (c deliveryReplyContext) Response(value interface{}, err error) {
	c.delivery.Reply(value, err)
}

// RejectDelivery 在业务无法执行时向原调用方回复错误；不需要回复的单向载荷直接忽略。
//
// 网关和玩家 Actor 在丢弃任何已接收的投递前都必须调用它，否则调用方会一直等待结果。
func RejectDelivery(delivery interface{}, err error) {
	if withReply, ok := delivery.(*DeliveryWithReply); ok {
		withReply.Reply(nil, err)
	}
}

// RejectDeliveries 对一批投递逐条调用 RejectDelivery。
func RejectDeliveries(deliveries []interface{}, err error) {
	for _, delivery := range deliveries {
		RejectDelivery(delivery, err)
	}
}
