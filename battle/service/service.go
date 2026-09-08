package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gogu-x/gogs/battle/engine"
	"github.com/gogu-x/gogs/def"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

type Source struct {
	ServerID int
	NodeID   int
}

type Notifier interface {
	Notify(Source, proto.Message) error
}

type NotifyFunc func(Source, proto.Message) error

func (f NotifyFunc) Notify(source Source, msg proto.Message) error { return f(source, msg) }

type Options struct {
	Workers       int
	QueueSize     int
	RecentTTL     time.Duration
	RecentLimit   int
	RetryDelay    time.Duration
	RetryAttempts int
}

func (o Options) normalized() Options {
	if o.Workers <= 0 {
		o.Workers = 2
	}
	if o.QueueSize <= 0 {
		o.QueueSize = 64
	}
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

type job struct {
	battle  *engine.Battle
	uid     uint64
	source  Source
	rebuild bool
}

type jobDone struct {
	battleID string
	report   Report
	err      error
}

type retryResult struct{ battleID string }

type recentBattle struct {
	BattleID string
	At       time.Time
}

type pendingResult struct {
	report   Report
	attempts int
	timer    *time.Timer
}

// QueryPlayerRecent is an in-process query backed by the actor's short-lived
// per-player index. The durable repository remains the source of truth.
type QueryPlayerRecent struct {
	UID   uint64
	Limit int
}

type PlayerRecent struct{ BattleIDs []string }

type Service struct {
	configs  engine.ConfigRepository
	reports  Repository
	notifier Notifier
	opts     Options

	queue   chan job
	workers sync.WaitGroup
	self    tree.PID
	system  *tree.Tree
	active  map[string]struct{}
	recent  map[uint64][]recentBattle
	pending map[string]*pendingResult
}

func New(configs engine.ConfigRepository, reports Repository, notifier Notifier, opts Options) *Service {
	if configs == nil {
		panic("battle service: nil config repository")
	}
	if reports == nil {
		panic("battle service: nil report repository")
	}
	if notifier == nil {
		notifier = NotifyFunc(func(Source, proto.Message) error { return nil })
	}
	opts = opts.normalized()
	return &Service{configs: configs, reports: reports, notifier: notifier, opts: opts, queue: make(chan job, opts.QueueSize), active: make(map[string]struct{}), recent: make(map[uint64][]recentBattle), pending: make(map[string]*pendingResult)}
}

func (s *Service) Name() string     { return def.BattleService }
func (s *Service) MailboxSize() int { return s.opts.QueueSize * 2 }

func (s *Service) OnInit(ctx tree.Context) {
	s.self, s.system = ctx.Self(), ctx.System()
	for range s.opts.Workers {
		s.workers.Add(1)
		go s.worker()
	}
}

func (s *Service) OnStop(_ tree.Context) {
	for _, pending := range s.pending {
		if pending.timer != nil {
			pending.timer.Stop()
		}
	}
	close(s.queue)
	s.workers.Wait()
}

func (s *Service) HandleMessage(ctx tree.Context, msg interface{}) {
	switch m := msg.(type) {
	case *pb.StartBattleReq:
		s.start(ctx, m)
	case *pb.QueryBattleReq:
		s.query(ctx, m)
	case *pb.RebuildBattleReq:
		s.rebuild(ctx, m)
	case *pb.VerifyBattleReplayReq:
		s.verify(ctx, m)
	case *pb.BattleResultConfirmedNtf:
		s.confirm(m)
	case *QueryPlayerRecent:
		s.queryRecent(ctx, m)
	case *jobDone:
		s.complete(m)
	case *retryResult:
		s.retry(m.battleID)
	default:
		ctx.Response(nil, fmt.Errorf("battle service: unsupported message %T", msg))
	}
}

func (s *Service) start(ctx tree.Context, req *pb.StartBattleReq) {
	if req == nil || req.UID == 0 || req.ServerID == 0 || req.SourceNodeId == 0 {
		ctx.Response(nil, fmt.Errorf("battle service: uid, source server and source node are required"))
		return
	}
	input, err := InputFromProto(req.Input)
	if err != nil {
		ctx.Response(nil, err)
		return
	}
	battle, err := engine.NewBattle(s.configs, input)
	if err != nil {
		ctx.Response(nil, err)
		return
	}
	battleID := battle.ID()
	source := Source{ServerID: int(req.ServerID), NodeID: int(req.SourceNodeId)}

	if _, running := s.active[battleID]; running {
		created := &pb.BattleCreatedNtf{BattleId: battleID, Uid: req.UID}
		_ = s.notifier.Notify(source, created)
		ctx.Response(created, nil)
		return
	}
	if report, getErr := s.reports.Get(context.Background(), battleID); getErr == nil {
		delivery := report
		delivery.UID = req.UID
		delivery.SourceServerID = source.ServerID
		delivery.SourceNodeID = source.NodeID
		s.pushReport(delivery)
		ctx.Response(&pb.BattleCreatedNtf{BattleId: battleID, Uid: req.UID}, nil)
		return
	} else if !errors.Is(getErr, ErrReportNotFound) {
		ctx.Response(nil, getErr)
		return
	}

	select {
	case s.queue <- job{battle: battle, uid: req.UID, source: source}:
		s.active[battleID] = struct{}{}
		s.addRecent(req.UID, battleID)
		created := &pb.BattleCreatedNtf{BattleId: battleID, Uid: req.UID}
		if err := s.notifier.Notify(source, created); err != nil {
			tlog.Log.Warn("battle service: notify Created %s: %v", battleID, err)
		}
		ctx.Response(created, nil)
	default:
		ctx.Response(nil, fmt.Errorf("battle service: work queue is full"))
	}
}

func (s *Service) worker() {
	defer s.workers.Done()
	for work := range s.queue {
		result, err := work.battle.Run()
		report := Report{}
		if err == nil {
			now := time.Now().UTC()
			report = Report{BattleID: result.BattleID, UID: work.uid, SourceServerID: work.source.ServerID, SourceNodeID: work.source.NodeID, Result: result, CreatedAt: now, UpdatedAt: now}
			// Persistence is deliberately completed in the worker before the actor
			// can emit any Action or Finished notification.
			err = s.reports.Save(context.Background(), report)
		}
		s.system.Send(s.self, &jobDone{battleID: work.battle.ID(), report: report, err: err})
	}
}

func (s *Service) complete(done *jobDone) {
	if done == nil {
		return
	}
	delete(s.active, done.battleID)
	if done.err != nil {
		tlog.Log.Error("battle service: job failed: %v", done.err)
		return
	}
	s.pushActions(done.report)
	s.sendFinished(done.report, true)
}

func (s *Service) pushActions(report Report) {
	source := Source{ServerID: report.SourceServerID, NodeID: report.SourceNodeID}
	for _, event := range report.Result.Events {
		if err := s.notifier.Notify(source, &pb.BattleActionNtf{BattleId: report.BattleID, Uid: report.UID, Event: eventToProto(event)}); err != nil {
			tlog.Log.Warn("battle service: notify Action %s/%d: %v", report.BattleID, event.Sequence, err)
		}
	}
}

func (s *Service) sendFinished(report Report, track bool) {
	source := Source{ServerID: report.SourceServerID, NodeID: report.SourceNodeID}
	if err := s.notifier.Notify(source, finishedToProto(report)); err != nil {
		tlog.Log.Warn("battle service: notify Finished %s: %v", report.BattleID, err)
	}
	if !track {
		return
	}
	if old := s.pending[report.BattleID]; old != nil && old.timer != nil {
		old.timer.Stop()
	}
	pending := &pendingResult{report: report, attempts: 1}
	s.pending[report.BattleID] = pending
	s.scheduleRetry(report.BattleID, pending)
}

func (s *Service) scheduleRetry(battleID string, pending *pendingResult) {
	pending.timer = time.AfterFunc(s.opts.RetryDelay, func() {
		s.system.Send(s.self, &retryResult{battleID: battleID})
	})
}

func (s *Service) retry(battleID string) {
	pending, ok := s.pending[battleID]
	if !ok {
		return
	}
	if pending.attempts >= s.opts.RetryAttempts {
		delete(s.pending, battleID)
		return
	}
	pending.attempts++
	s.sendFinished(pending.report, false)
	s.scheduleRetry(battleID, pending)
}

func (s *Service) confirm(msg *pb.BattleResultConfirmedNtf) {
	if msg == nil || msg.BattleId == "" {
		return
	}
	if pending := s.pending[msg.BattleId]; pending != nil {
		if pending.timer != nil {
			pending.timer.Stop()
		}
		delete(s.pending, msg.BattleId)
	}
	if err := s.reports.Confirm(context.Background(), msg.BattleId); err != nil && !errors.Is(err, ErrReportNotFound) {
		tlog.Log.Warn("battle service: confirm %s: %v", msg.BattleId, err)
	}
}

func (s *Service) pushReport(report Report) {
	source := Source{ServerID: report.SourceServerID, NodeID: report.SourceNodeID}
	_ = s.notifier.Notify(source, &pb.BattleCreatedNtf{BattleId: report.BattleID, Uid: report.UID})
	s.pushActions(report)
	s.sendFinished(report, !report.Confirmed)
}

func (s *Service) query(ctx tree.Context, req *pb.QueryBattleReq) {
	if req == nil || req.BattleId == "" {
		ctx.Response(nil, fmt.Errorf("battle service: battle_id is required"))
		return
	}
	report, err := s.reports.Get(context.Background(), req.BattleId)
	if errors.Is(err, ErrReportNotFound) {
		ctx.Response(&pb.QueryBattleAck{Found: false}, nil)
		return
	}
	if err != nil {
		ctx.Response(nil, err)
		return
	}
	ctx.Response(&pb.QueryBattleAck{Found: true, Result: ResultToProto(report.Result)}, nil)
}

func (s *Service) rebuild(ctx tree.Context, req *pb.RebuildBattleReq) {
	if req == nil || req.BattleId == "" {
		ctx.Response(&pb.RebuildBattleAck{Reason: "battle_id is required"}, nil)
		return
	}
	report, err := s.reports.Get(context.Background(), req.BattleId)
	if err != nil {
		reason := err.Error()
		ctx.Response(&pb.RebuildBattleAck{BattleId: req.BattleId, Reason: reason}, nil)
		return
	}
	battle, err := engine.NewBattle(s.configs, report.Result.Replay.Input)
	if err != nil || battle.ID() != req.BattleId {
		if err == nil {
			err = fmt.Errorf("stored replay generated a different battle_id")
		}
		ctx.Response(&pb.RebuildBattleAck{BattleId: req.BattleId, Reason: err.Error()}, nil)
		return
	}
	select {
	case s.queue <- job{battle: battle, uid: report.UID, source: Source{ServerID: report.SourceServerID, NodeID: report.SourceNodeID}, rebuild: true}:
		s.active[req.BattleId] = struct{}{}
		created := &pb.BattleCreatedNtf{BattleId: req.BattleId, Uid: report.UID}
		if notifyErr := s.notifier.Notify(Source{ServerID: report.SourceServerID, NodeID: report.SourceNodeID}, created); notifyErr != nil {
			tlog.Log.Warn("battle service: notify rebuild Created %s: %v", req.BattleId, notifyErr)
		}
		ctx.Response(&pb.RebuildBattleAck{Accepted: true, BattleId: req.BattleId}, nil)
	default:
		ctx.Response(&pb.RebuildBattleAck{BattleId: req.BattleId, Reason: "work queue is full"}, nil)
	}
}

func (s *Service) verify(ctx tree.Context, req *pb.VerifyBattleReplayReq) {
	if req == nil {
		ctx.Response(&pb.VerifyBattleReplayAck{Reason: "replay is required"}, nil)
		return
	}
	replay, err := replayFromProto(req.Replay)
	if err == nil {
		err = engine.VerifyReplay(s.configs, replay)
	}
	ack := &pb.VerifyBattleReplayAck{Valid: err == nil}
	if err != nil {
		ack.Reason = err.Error()
	}
	ctx.Response(ack, nil)
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
