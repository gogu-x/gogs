// 网关业务压测：独立注册登录、持续在线及响应延迟统计。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/gogu-x/gogs/pb"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_equipment"
	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/gogs/pb/cspb/pb_souljade"
	"github.com/gogu-x/gogs/pb/cspb/pb_talent"
	"github.com/gogu-x/tree/codec"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

var (
	addr     = flag.String("addr", "ws://127.0.0.1:8001/ws", "网关地址")
	users    = flag.Int("users", 10, "玩家数量")
	rate     = flag.Int("rate", 100, "每秒启动玩家数")
	hold     = flag.Duration("hold", 10*time.Minute, "升压完成后持续时间")
	interval = flag.Duration("interval", 5*time.Second, "每人请求间隔")
	timeout  = flag.Duration("timeout", 10*time.Second, "请求超时")
	server   = flag.Uint("server", 1, "服务器编号")
	mu       sync.Mutex
	hist     = map[string][]uint64{}
	errors   = map[string]uint64{}
	online   atomic.Int64
	peak     atomic.Int64
	ready    atomic.Int64
	full     atomic.Bool
	prefix   = fmt.Sprintf("load_%d", time.Now().Unix())
)

// record 汇总成功响应直方图及错误，不存储账号密码。
func record(name string, start time.Time, err error) {
	if full.Load() {
		name = "hold/" + name
	} else {
		name = "ramp/" + name
	}
	mu.Lock()
	defer mu.Unlock()
	if err != nil {
		detail := err.Error()
		if networkError, ok := err.(net.Error); ok && networkError.Timeout() {
			detail = "请求超时"
		}
		errors[name+": "+detail]++
		return
	}
	if hist[name] == nil {
		hist[name] = make([]uint64, 60002)
	}
	ms := min(time.Since(start).Milliseconds(), 60001)
	hist[name][ms]++
}

// request 顺序收发请求，忽略其他推送但不延长请求期限。
func request(c *websocket.Conn, req proto.Message, ack proto.Message) error {
	start := time.Now()
	name := reflect.TypeOf(req).Elem().Name()
	err := exchange(c, req, ack)
	record(name, start, err)
	return err
}

// exchange 校验响应类型和业务错误码。
func exchange(c *websocket.Conn, req proto.Message, ack proto.Message) error {
	data, err := codec.ProtoCodec.Marshal(req)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(*timeout)
	c.SetWriteDeadline(deadline)
	c.SetReadDeadline(deadline)
	if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return err
	}
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			return err
		}
		msg, err := codec.ProtoCodec.Unmarshal(data)
		if err != nil {
			return fmt.Errorf("响应解码失败")
		}
		if notification, ok := msg.(*pb_common.ErrorNtf); ok && notification.GetRequestType() == string(proto.MessageName(req)) {
			return fmt.Errorf("统一错误通知=%s", notification.GetCode())
		}
		if reflect.TypeOf(msg) != reflect.TypeOf(ack) {
			continue
		}
		result, ok := msg.(interface{ GetCode() pb_common.ErrCode })
		if !ok {
			return fmt.Errorf("响应缺少业务码")
		}
		if code := result.GetCode(); code != pb_common.ErrCode_OK {
			return fmt.Errorf("业务码=%s", code)
		}
		return nil
	}
}

// query 按轮次执行四类玩家查询。
func query(c *websocket.Conn, index int) error {
	switch index % 4 {
	case 0:
		return request(c, &pb_equipment.GetRoleEquipmentReq{}, &pb_equipment.GetRoleEquipmentAck{})
	case 1:
		return request(c, &pb_souljade.GetRoleSoulJadeReq{}, &pb_souljade.GetRoleSoulJadeAck{})
	case 2:
		return request(c, &pb_talent.GetRoleTalentReq{}, &pb_talent.GetRoleTalentAck{})
	default:
		return request(c, &pb_actv.GetActvListReq{}, &pb_actv.GetActvListAck{})
	}
}

// runUser 注册并登录独立玩家，通过业务请求维持在线。
func runUser(ctx context.Context, index int, admitted *sync.WaitGroup) {
	done := false
	defer func() {
		if !done {
			admitted.Done()
		}
	}()
	start := time.Now()
	dialer := websocket.Dialer{Subprotocols: []string{"protobuf"}, HandshakeTimeout: *timeout}
	c, _, err := dialer.DialContext(ctx, *addr, nil)
	record("connect", start, err)
	if err != nil {
		return
	}
	defer c.Close()
	account := fmt.Sprintf("%s_%d", prefix, index)
	if err := request(c, &pb_gateway.RegisterReq{Account: account, Password: "LoadTest@1234", ServerId: uint32(*server)}, &pb_gateway.RegisterAck{}); err != nil {
		return
	}
	if err := request(c, &pb_gateway.LoginReq{Account: account, Password: "LoadTest@1234", ServerId: uint32(*server)}, &pb_gateway.LoginAck{}); err != nil {
		return
	}
	for i := 0; i < 4; i++ {
		if err := query(c, i); err != nil {
			return
		}
	}
	n := online.Add(1)
	defer online.Add(-1)
	for old := peak.Load(); n > old; old = peak.Load() {
		if peak.CompareAndSwap(old, n) {
			break
		}
	}
	ready.Add(1)
	done = true
	admitted.Done()
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for i := index; ; i++ {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := query(c, i); err != nil {
				return
			}
		}
	}
}

// report 输出可重复核验的累计计数和毫秒分位数，按升压与稳态分开。
func report() {
	mu.Lock()
	defer mu.Unlock()
	stats := map[string]any{}
	for name, buckets := range hist {
		var count, sum uint64
		for ms, n := range buckets {
			count += n
			sum += uint64(ms) * n
		}
		quantiles := map[string]uint64{}
		for label, pct := range map[string]uint64{"p50_ms": 50, "p95_ms": 95, "p99_ms": 99, "max_ms": 100} {
			var cumulative uint64
			for ms, n := range buckets {
				cumulative += n
				if cumulative >= (count*pct+99)/100 {
					quantiles[label] = uint64(ms)
					break
				}
			}
		}
		stats[name] = map[string]any{"ok": count, "mean_ms": float64(sum) / float64(count), "latency": quantiles}
	}
	data, _ := json.Marshal(map[string]any{"time": time.Now().Format(time.RFC3339), "online": online.Load(), "peak": peak.Load(), "ready": ready.Load(), "hold": full.Load(), "stats": stats, "errors": errors})
	fmt.Println(string(data))
}

// main 按指定速率升压，所有用户完成首轮后进入稳态；中断时关闭测试连接。
func main() {
	flag.Parse()
	if *users < 1 || *rate < 1 || *rate > 10000 || *interval <= 0 || *hold <= 0 {
		log.Fatal("参数范围无效")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	log.Printf(
		"[gateload/main] 开始压测, prefix=%s users=%d rate=%d addr=%s hold=%s",
		prefix, *users, *rate, *addr, *hold,
	)
	var workers, admitted sync.WaitGroup
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				report()
			}
		}
	}()
	pacer := time.NewTicker(time.Second / time.Duration(*rate))
	for i := 0; i < *users && ctx.Err() == nil; i++ {
		<-pacer.C
		workers.Add(1)
		admitted.Add(1)
		go func(index int) { defer workers.Done(); runUser(ctx, index, &admitted) }(i)
	}
	pacer.Stop()
	admitted.Wait()
	full.Store(true)
	report()
	log.Printf("[gateload/main] 升压结束, online=%d target=%d", online.Load(), *users)
	if online.Load() > 0 {
		select {
		case <-ctx.Done():
		case <-time.After(*hold):
		}
	}
	report()
	stop()
	workers.Wait()
	log.Printf("[gateload/main] 测试连接已关闭, peak=%d", peak.Load())
}
