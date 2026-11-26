package robot

import (
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
)

// commandProbe 记录调度器发送的动作，供测试控制回执时机。
type commandProbe struct {
	router   tree.Router
	commands chan *pb.BattleInputReq
}

// Name 返回测试用动作接收器的注册名。
func (*commandProbe) Name() string { return "test-multi-unit-commands" }

// OnInit 注册需要记录的机器人动作消息。
func (p *commandProbe) OnInit(tree.Context) {
	tree.RegisterFunc(&p.router, (*Command)(nil), p.onCommand)
}

// OnStop 实现测试接收器的停止回调，无需额外清理。
func (*commandProbe) OnStop(tree.Context) {}

// HandleMessage 将收到的消息交给测试接收器的路由。
func (p *commandProbe) HandleMessage(ctx tree.Context, msg interface{}) { p.router.Route(ctx, msg) }

// onCommand 将实际投递的动作交给测试断言。
func (p *commandProbe) onCommand(_ tree.Context, msg *Command) { p.commands <- msg.Input }
