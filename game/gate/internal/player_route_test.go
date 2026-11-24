package internal

import (
	"testing"
	"time"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/db/mongorpc"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// routeContext 模拟网关投递上下文，记录实际交给玩家的消息。
type routeContext struct {
	tree.Context
	reply   *tree.Envelope
	payload interface{}
}

func (c *routeContext) RequestEnvelope() *tree.Envelope          { return c.reply }
func (c *routeContext) Response(value interface{}, err error)    { c.reply.Respond(value, err) }
func (c *routeContext) TrySend(_ tree.PID, msg interface{}) bool { c.payload = msg; return true }

// TestPlayerBusinessRoute 验证离线策略、正在回收时的排队和业务应答目标。
func TestPlayerBusinessRoute(t *testing.T) {
	for _, tc := range []struct {
		name                                      string
		online, resident, evicting, requireOnline bool
	}{
		{"在线", true, true, false, true},
		{"离线驻留拒绝在线操作", false, true, false, true},
		{"已回收拒绝在线操作", false, false, false, true},
		{"回收期间保留离线业务", false, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGateActor()
			state := &playerActorState{pid: tree.PID{ID: 1}, evicting: tc.evicting}
			if tc.resident {
				g.playerActors[7] = state
			}
			if tc.online {
				g.sessions[7] = session{}
			}
			req := &ipb.DeliverToPlayer{UID: 7, Payload: "业务", RequireOnline: tc.requireOnline}
			ctx := &routeContext{reply: tree.NewRequest(req, tree.PID{}, nil, nil)}
			g.onDeliverToPlayer(ctx, req)
			if tc.requireOnline && !tc.online {
				if _, err := ctx.reply.AwaitTimeout(time.Second); err == nil || err == tree.ErrTimeout || ctx.payload != nil || state.hasQueuedDeliveries() {
					t.Fatal("离线请求未立即拒绝")
				}
				return
			}
			payload := ctx.payload
			if tc.evicting {
				pending := state.takeQueuedDeliveries()
				if len(pending) != 1 {
					t.Fatal("回收期间未保留业务")
				}
				payload = pending[0]
			}
			delivery, ok := payload.(*ipb.DeliveryWithReply)
			if !ok || delivery.GetPayload() != "业务" {
				t.Fatal("未转交原业务")
			}
			delivery.ReplyContext(ctx).Response("处理完成", nil)
			if value, err := ctx.reply.AwaitTimeout(time.Second); err != nil || value != "处理完成" {
				t.Fatalf("原调用方收到 %v, %v", value, err)
			}
		})
	}
}

// emptyPlayerStore 模拟没有存档的玩家，验证首次加载期间保留原请求。
type emptyPlayerStore struct{}

func (emptyPlayerStore) Name() string        { return def.Mongo }
func (emptyPlayerStore) OnInit(tree.Context) {}
func (emptyPlayerStore) OnStop(tree.Context) {}
func (emptyPlayerStore) HandleMessage(ctx tree.Context, msg interface{}) {
	if _, ok := msg.(*mongorpc.FindOne); ok {
		ctx.Response(nil, mongo.ErrNoDocuments)
	}
}

// TestColdPlayerRoute 验证允许离线的请求可唤醒玩家，加载数据后回复实际业务结果。
func TestColdPlayerRoute(t *testing.T) {
	store := tree.SpawnOne(emptyPlayerStore{})
	t.Cleanup(func() { tree.Stop(store) })
	gate := NewGateActor()
	req := &ipb.DeliverToPlayer{UID: 8, Payload: &ipb.ActvAssetTrade{UID: 8}}
	ctx := &routeContext{reply: tree.NewRequest(req, tree.PID{}, nil, nil)}
	gate.onDeliverToPlayer(ctx, req)
	pid := gate.playerActors[8].pid
	t.Cleanup(func() { tree.Stop(pid) })
	if value, err := ctx.reply.AwaitTimeout(time.Second); err != nil || value != pb_common.ErrCode_PARAM {
		t.Fatalf("预期玩家业务校验结果，收到 %v, %v", value, err)
	}
}

// TestQueuedDeliveriesRejectedWhenDroppedDuringShutdown 验证关服期间玩家 Actor 停止时，
// 网关在丢弃排队投递前向原调用方回复错误。
func TestQueuedDeliveriesRejectedWhenDroppedDuringShutdown(t *testing.T) {
	g := NewGateActor()
	playerPID := tree.PID{ID: 1}
	state := &playerActorState{pid: playerPID}
	g.playerActors[7] = state
	reply := tree.NewRequest(nil, tree.PID{}, nil, nil)
	if !state.queueDelivery(ipb.WrapWithReply("业务", reply)) {
		t.Fatal("排队失败")
	}
	g.shuttingDown = true

	g.onPlayerActorStopped(&routeContext{}, &ipb.PlayerActorStopped{UID: 7, PID: playerPID})

	if _, err := reply.AwaitTimeout(time.Second); err == nil || err == tree.ErrTimeout {
		t.Fatalf("排队投递被丢弃但调用方未收到错误: %v", err)
	}
	if _, exists := g.playerActors[7]; exists {
		t.Fatal("关服后未清理玩家目录")
	}
}

// TestDeliveryRejectedWhenQueueFull 验证排队已满时网关立即向调用方回复错误。
func TestDeliveryRejectedWhenQueueFull(t *testing.T) {
	g := NewGateActor()
	state := &playerActorState{pid: tree.PID{ID: 1}, evicting: true}
	for i := 0; i < maxQueuedDeliveriesPerPlayer; i++ {
		state.queueDelivery(i)
	}
	g.playerActors[7] = state
	req := &ipb.DeliverToPlayer{UID: 7, Payload: "业务"}
	ctx := &routeContext{reply: tree.NewRequest(req, tree.PID{}, nil, nil)}

	g.onDeliverToPlayer(ctx, req)

	if _, err := ctx.reply.AwaitTimeout(time.Second); err == nil || err == tree.ErrTimeout {
		t.Fatalf("排队已满但调用方未收到错误: %v", err)
	}
}
