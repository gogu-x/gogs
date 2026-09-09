package internal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gogu-x/gogs/battle/battle/internal"
	engine2 "github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/def"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

// Options 是 manager 的装配参数。retry 项在创建 per-battle battle 时透传给
// battle（重试归属 battle）；recent 项供 manager 的进程内"玩家最近战斗"索引使用。
type Options struct {
	RecentTTL     time.Duration
	RecentLimit   int
	RetryDelay    time.Duration
	RetryAttempts int
}

func (o Options) normalized() Options {
	if o.RecentTTL <= 0 {
		o.RecentTTL = 10 * time.Minute
	}
	if o.RecentLimit <= 0 {
		o.RecentLimit = 20
	}
	if o.RetryDelay <= 0 {
		o.RetryDelay = time.Second
	}
	if o.RetryAttempts <= 0 {
		o.RetryAttempts = 3
	}
	return o
}

type recentBattle struct {
	BattleID string
	At       time.Time
}

// QueryPlayerRecent is an in-process query backed by the manager's short-lived
// per-player index. The durable repository remains the source of truth.
type QueryPlayerRecent struct {
	UID   uint64
	Limit int
}

type PlayerRecent struct{ BattleIDs []string }

// spawnSpec 描述 manager 需在旧 battle 停止后"让位替换"运行的一场战斗
// （rebuild-while-active）。复用同一 battle 保证确定性输出。
type spawnSpec struct {
	uid    uint64
	source internal.Source
	battle *engine2.Battle
}

// Service 是 battle 进程内的 manager battle：只负责 battle 编排、登记与只读
// 查询。每场战斗的"战报"（结算/落库/推送/确认/重试）全部在 manager 运行期
// SpawnOne 出的独立 battle battle（battle/battle）内自洽完成，manager 不持有、
// 不组织任何战报。manager 用 active/retiring 登记 battle 生命周期；reports
// 仅作只读用途：start 幂等预判、QueryBattle/RebuildBattle 读历史、旧客户端
// 确认兜底落库。
type Service struct {
	opts Options

	router tree.Router
	self   tree.PID
	system *tree.Tree
	active map[string]tree.PID
	// retiring 记录 rebuild-while-active 时等待"旧 battle 停止后替换运行"的战斗。
	retiring map[string]*spawnSpec
	recent   map[uint64][]recentBattle
}

func New() *Service {

	return &Service{
		active:   make(map[string]tree.PID),
		retiring: make(map[string]*spawnSpec),
		recent:   make(map[uint64][]recentBattle),
	}
}

func (s *Service) Name() string     { return def.BattleService }
func (s *Service) MailboxSize() int { return 1024 }

func (s *Service) OnInit(ctx tree.Context) {
	s.self, s.system = ctx.Self(), ctx.System()
	s.initRouter()
}

func (s *Service) OnStop(_ tree.Context) {}

func (s *Service) HandleMessage(ctx tree.Context, msg interface{}) {
	s.router.Route(ctx, msg)
}

// start 是创建入口：校验输入后按 battle_id 做幂等判定，需要现场结算/补发的
// 场景一律 spawn 一个 per-battle battle 处理，manager 自身不碰战报。
func (s *Service) start(ctx tree.Context, req *pb.StartBattleReq) {
	if req == nil || req.UID == 0 || req.ServerID == 0 || req.SourceNodeId == 0 {
		ctx.Response(nil, fmt.Errorf("battle service: uid, source server and source node are required"))
		return
	}
	input, err := internal.InputFromProto(req.Input)
	if err != nil {
		ctx.Response(nil, err)
		return
	}
	battle, err := engine2.NewBattle(s.configs, input)
	if err != nil {
		ctx.Response(nil, err)
		return
	}
	battleID := battle.ID()
	source := battle.Source{ServerID: int(req.ServerID), NodeID: int(req.SourceNodeId)}
	created := &pb.BattleCreatedNtf{BattleId: battleID, Uid: req.UID}

	if _, running := s.active[battleID]; running {
		// 幂等命中：该场战斗已有 battle 在跑/在等确认，Created 由那个 battle
		// 自洽推送，manager 只回受理回执。
		ctx.Response(created, nil)
		return
	}
	if report, getErr := s.reports.Get(context.Background(), battleID); getErr == nil {
		// 库中已有该战报（此前已结算）：交给 redeliver battle 自洽补发——
		// 已确认 → 推送 Created/Action/Finished 后自行退出；
		// 未确认 → 重投并重试等待确认。战报内容与投递一律不进 manager。
		s.spawnRedeliver(report, req.UID, source)
		ctx.Response(created, nil)
		return
	} else if !errors.Is(getErr, battle.ErrReportNotFound) {
		ctx.Response(nil, getErr)
		return
	}

	s.addRecent(req.UID, battleID)
	s.spawnRun(spawnSpec{uid: req.UID, source: source, battle: battle})
	ctx.Response(created, nil)
}

func (s *Service) rebuild(ctx tree.Context, req *pb.RebuildBattleReq) {
	if req == nil || req.BattleId == "" {
		ctx.Response(&pb.RebuildBattleAck{Reason: "battle_id is required"}, nil)
		return
	}
	report, err := s.reports.Get(context.Background(), req.BattleId)
	if err != nil {
		ctx.Response(&pb.RebuildBattleAck{BattleId: req.BattleId, Reason: err.Error()}, nil)
		return
	}
	battle, err := engine2.NewBattle(s.configs, report.Result.Replay.Input)
	if err != nil || battle.ID() != req.BattleId {
		if err == nil {
			err = fmt.Errorf("stored replay generated a different battle_id")
		}
		ctx.Response(&pb.RebuildBattleAck{BattleId: req.BattleId, Reason: err.Error()}, nil)
		return
	}
	spec := spawnSpec{uid: report.UID, source: battle.Source{ServerID: report.SourceServerID, NodeID: report.SourceNodeID}, battle: battle}
	if pid, ok := s.active[req.BattleId]; ok {
		// 原 battle 仍在（可能等待确认）：让位队列登记后请它退出，由 ActorStopped 触发替换。
		s.retiring[req.BattleId] = &spec
		s.system.Send(pid, &battle.RetireBattle{BattleID: req.BattleId})
	} else {
		s.spawnRun(spec)
	}
	ctx.Response(&pb.RebuildBattleAck{Accepted: true, BattleId: req.BattleId}, nil)
}

func (s *Service) verify(ctx tree.Context, req *pb.VerifyBattleReplayReq) {
	if req == nil {
		ctx.Response(&pb.VerifyBattleReplayAck{Reason: "replay is required"}, nil)
		return
	}
	replay, err := internal.ReplayFromProto(req.Replay)
	if err == nil {
		err = engine2.VerifyReplay(s.configs, replay)
	}
	ack := &pb.VerifyBattleReplayAck{Valid: err == nil}
	if err != nil {
		ack.Reason = err.Error()
	}
	ctx.Response(ack, nil)
}

func (s *Service) query(ctx tree.Context, req *pb.QueryBattleReq) {
	if req == nil || req.BattleId == "" {
		ctx.Response(nil, fmt.Errorf("battle service: battle_id is required"))
		return
	}
	report, err := s.reports.Get(context.Background(), req.BattleId)
	if errors.Is(err, internal.ErrReportNotFound) {
		ctx.Response(&pb.QueryBattleAck{Found: false}, nil)
		return
	}
	if err != nil {
		ctx.Response(nil, err)
		return
	}
	ctx.Response(&pb.QueryBattleAck{Found: true, Result: internal.ResultToProto(report.Result)}, nil)
}

// confirm 兼容旧客户端把确认发到 BATTLE_SERVICE 名的情况：转发给对应活跃
// battle（确认已默认 NATS 直达 battle 名）；无活跃 battle 时直接幂等落库确认。
func (s *Service) confirm(ctx tree.Context, msg *pb.BattleResultConfirmedNtf) {
	if msg == nil || msg.BattleId == "" {
		return
	}
	if pid, ok := s.active[msg.BattleId]; ok {
		s.system.Send(pid, msg)
		return
	}
	if err := s.reports.Confirm(context.Background(), msg.BattleId); err != nil && !errors.Is(err, internal.ErrReportNotFound) {
		tlog.Log.Warn("battle service: confirm %s: %v", msg.BattleId, err)
	}
}

// release 处理 battle 退出：清理登记，若存在让位队列则 spawn 替换 battle。
func (s *Service) release(done *internal.ActorStopped) {
	if done == nil {
		return
	}
	if pid, ok := s.active[done.BattleID]; ok && pid == done.PID {
		delete(s.active, done.BattleID)
	}
	if spec, ok := s.retiring[done.BattleID]; ok {
		delete(s.retiring, done.BattleID)
		s.spawnRun(*spec)
	}
}

func (s *Service) spawnRun(spec spawnSpec) {
	pid := s.system.SpawnOne(internal.NewBattleActor(internal.Params{
		Mode: internal.ModeRun, BattleID: spec.battle.ID(), UID: spec.uid, Source: spec.source,
		Battle: spec.battle, Reports: s.reports, Notifier: s.notifier,
		RetryDelay: s.opts.RetryDelay, RetryAttempts: s.opts.RetryAttempts, ManagerPID: s.self,
	}))
	s.active[spec.battle.ID()] = pid
}

func (s *Service) spawnRedeliver(report internal.Report, uid uint64, source internal.Source) {
	pid := s.system.SpawnOne(internal.NewBattleActor(internal.Params{
		Mode: internal.ModeRedeliver, BattleID: report.BattleID, UID: uid, Source: source,
		Report: report, Reports: s.reports, Notifier: s.notifier,
		RetryDelay: s.opts.RetryDelay, RetryAttempts: s.opts.RetryAttempts, ManagerPID: s.self,
	}))
	s.active[report.BattleID] = pid
}

func (s *Service) addRecent(uid uint64, battleID string) {
	now := time.Now()
	items := s.recent[uid]
	kept := make([]recentBattle, 0, len(items)+1)
	kept = append(kept, recentBattle{BattleID: battleID, At: now})
	for _, item := range items {
		if item.BattleID != battleID && now.Sub(item.At) <= s.opts.RecentTTL && len(kept) < s.opts.RecentLimit {
			kept = append(kept, item)
		}
	}
	s.recent[uid] = kept
}

func (s *Service) queryRecent(ctx tree.Context, query *QueryPlayerRecent) {
	if query == nil || query.UID == 0 {
		ctx.Response(nil, fmt.Errorf("battle service: uid is required"))
		return
	}
	limit := query.Limit
	if limit <= 0 || limit > s.opts.RecentLimit {
		limit = s.opts.RecentLimit
	}
	now := time.Now()
	items := s.recent[query.UID]
	ids := make([]string, 0, limit)
	kept := items[:0]
	for _, item := range items {
		if now.Sub(item.At) <= s.opts.RecentTTL {
			kept = append(kept, item)
			if len(ids) < limit {
				ids = append(ids, item.BattleID)
			}
		}
	}
	s.recent[query.UID] = kept
	ctx.Response(&PlayerRecent{BattleIDs: ids}, nil)
}
