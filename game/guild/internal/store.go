//go:generate go run ../../../tools/deepcopy-gen -dir ../..

package internal

import (
	"fmt"
	"sort"

	"github.com/gogu-x/gogs/game/activity"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_guild"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Member is one player's persisted membership in a guild.
// +deepcopy-gen=true
type Member struct {
	UID   uint64             `bson:"uid"`   // 玩家唯一 ID。
	Name  string             `bson:"name"`  // 玩家名称快照。
	Level uint32             `bson:"level"` // 玩家等级快照。
	Role  pb_guild.GuildRole `bson:"role"`  // 联盟职位。
}

// GetUID returns the member's player ID.
func (member *Member) GetUID() uint64 {
	if member == nil {
		return 0
	}
	return member.UID
}

// GetName returns the member's display name.
func (member *Member) GetName() string {
	if member == nil {
		return ""
	}
	return member.Name
}

// GetLevel returns the member's cached player level.
func (member *Member) GetLevel() uint32 {
	if member == nil {
		return 0
	}
	return member.Level
}

// GetRole returns the member's current guild role.
func (member *Member) GetRole() pb_guild.GuildRole {
	if member == nil {
		return pb_guild.GuildRole_MEMBER
	}
	return member.Role
}

// ToProto creates a client-facing member snapshot.
func (member *Member) ToProto() *pb_guild.GuildMember {
	if member == nil {
		return nil
	}
	return &pb_guild.GuildMember{
		Uid:   member.GetUID(),
		Name:  member.GetName(),
		Level: member.GetLevel(),
		Role:  member.GetRole(),
	}
}

// Guild is a persisted guild aggregate owned by the global Guild Actor.
// +deepcopy-gen=true
type Guild struct {
	ID               uint64             `bson:"_id"`        // 联盟唯一 ID。
	Name             string             `bson:"name"`       // 联盟名称。
	Notice           string             `bson:"notice"`     // 联盟公告。
	Leader           uint64             `bson:"leader"`     // 当前会长玩家 ID。
	Members          map[uint64]*Member `bson:"members"`    // 按玩家 ID 索引的成员。
	LegacyActivities bson.Raw           `bson:"activities"` // 旧版联盟文档中的活动数据，仅用于迁移。
	Deleted          bool               `bson:"deleted"`    // 联盟是否已解散。
	Dirty            bool               `bson:"-"`          // 是否需要存盘。
	revision         uint64             `bson:"-"`          // 内存版本号，不写入联盟文档。
}

// GetID returns the guild ID.
func (guild *Guild) GetID() uint64 {
	if guild == nil {
		return 0
	}
	return guild.ID
}

// GetName returns the guild name.
func (guild *Guild) GetName() string {
	if guild == nil {
		return ""
	}
	return guild.Name
}

// GetNotice returns the guild notice.
func (guild *Guild) GetNotice() string {
	if guild == nil {
		return ""
	}
	return guild.Notice
}

// GetLeader returns the leader's player ID.
func (guild *Guild) GetLeader() uint64 {
	if guild == nil {
		return 0
	}
	return guild.Leader
}

// IsDeleted reports whether the guild has been disbanded.
func (guild *Guild) IsDeleted() bool {
	return guild == nil || guild.Deleted
}

// GetMembers returns the guild's member index.
func (guild *Guild) GetMembers() map[uint64]*Member {
	if guild == nil {
		return nil
	}
	return guild.Members
}

// GetLegacyActivities returns the old embedded activity data for migration.
func (guild *Guild) GetLegacyActivities() bson.Raw {
	if guild == nil {
		return nil
	}
	return guild.LegacyActivities
}

// ClearLegacyActivities clears the old embedded activity data after migration.
func (guild *Guild) ClearLegacyActivities() {
	if guild != nil {
		guild.LegacyActivities = nil
	}
}

// MarkDirty marks the guild document as changed and increments its version.
func (guild *Guild) MarkDirty() {
	if guild == nil {
		return
	}
	guild.SetDirty(true)
}

// IsDirty reports whether the guild needs persistence.
func (guild *Guild) IsDirty() bool {
	return guild != nil && guild.Dirty
}

// Revision returns the in-memory version used to protect concurrent saves.
func (guild *Guild) Revision() uint64 {
	if guild == nil {
		return 0
	}
	return guild.revision
}

// SetDirty updates the persistence marker.
func (guild *Guild) SetDirty(dirty bool) {
	if guild == nil {
		return
	}
	if dirty {
		guild.revision++
	}
	guild.Dirty = dirty
}

// ToProto creates a deterministic client-facing guild snapshot.
func (guild *Guild) ToProto() *pb_guild.GuildInfo {
	if guild == nil || guild.IsDeleted() {
		return nil
	}
	membersByID := guild.GetMembers()
	memberIDs := make([]uint64, 0, len(membersByID))
	for uid := range membersByID {
		memberIDs = append(memberIDs, uid)
	}
	sort.Slice(memberIDs, func(i, j int) bool { return memberIDs[i] < memberIDs[j] })
	members := make([]*pb_guild.GuildMember, 0, len(memberIDs))
	for _, uid := range memberIDs {
		if member := membersByID[uid]; member != nil {
			members = append(members, member.ToProto())
		}
	}
	return &pb_guild.GuildInfo{
		GuildId: guild.GetID(),
		Name:    guild.GetName(),
		Notice:  guild.GetNotice(),
		Leader:  guild.GetLeader(),
		Members: members,
	}
}

// Store owns all guild aggregates and the player-to-guild membership index.
type Store struct {
	guilds          map[uint64]*Guild
	activityRecords map[uint64]*activity.Record
	memberGuild     map[uint64]uint64
	nextID          uint64
}

// NewStore creates an empty guild store.
func NewStore() *Store {
	return &Store{
		guilds:          make(map[uint64]*Guild),
		activityRecords: make(map[uint64]*activity.Record),
		memberGuild:     make(map[uint64]uint64),
	}
}

// Create handles a guild creation request.
func (store *Store) Create(ctx tree.Context, req *pb_guild.CreateGuildReq) {
	ctx.Response(store.create(req), nil)
}

// Join handles a guild join request.
func (store *Store) Join(ctx tree.Context, req *pb_guild.JoinGuildReq) {
	ctx.Response(store.join(req), nil)
}

// Leave handles a guild leave request.
func (store *Store) Leave(ctx tree.Context, req *pb_guild.LeaveGuildReq) {
	ctx.Response(store.leave(req), nil)
}

// Get handles a guild lookup request.
func (store *Store) Get(ctx tree.Context, req *pb_guild.GetGuildReq) {
	ctx.Response(store.get(req), nil)
}

// HandlePending executes a queued request after guild documents finish loading.
func (store *Store) HandlePending(message interface{}) (interface{}, bool) {
	switch request := message.(type) {
	case *pb_guild.CreateGuildReq:
		return store.create(request), true
	case *pb_guild.JoinGuildReq:
		return store.join(request), true
	case *pb_guild.LeaveGuildReq:
		return store.leave(request), true
	case *pb_guild.GetGuildReq:
		return store.get(request), true
	case *ipb.GetGuildActivities:
		return store.activitiesForPlayer(request.GetUID()), true
	default:
		return nil, false
	}
}

func (store *Store) activitiesForPlayer(uid uint64) []*pb_activity.ActivityInfo {
	if store == nil || uid == 0 {
		return []*pb_activity.ActivityInfo{}
	}
	guildID, exists := store.memberGuild[uid]
	if !exists {
		return []*pb_activity.ActivityInfo{}
	}
	guild := store.getGuild(guildID)
	record := store.activityRecords[guildID]
	if guild == nil || guild.IsDeleted() || record == nil || record.GetManager() == nil {
		return []*pb_activity.ActivityInfo{}
	}
	instances := record.GetManager().List()
	activities := make([]*pb_activity.ActivityInfo, 0, len(instances))
	for _, instance := range instances {
		if instance == nil {
			continue
		}
		activities = append(activities, activity.ToInfo(instance))
	}
	return activities
}

func (store *Store) create(req *pb_guild.CreateGuildReq) *pb_guild.CreateGuildAck {
	ack := &pb_guild.CreateGuildAck{Code: pb_common.ErrCode_PARAM}
	if store == nil || req == nil || req.GetUID() == 0 || req.GetName() == "" {
		uid := uint64(0)
		if req != nil {
			uid = req.GetUID()
		}
		tlog.Log.Error("[guild/Store.create] 创建联盟参数无效, pid=%v", uid)
		return ack
	}
	uid := req.GetUID()
	if _, exists := store.memberGuild[uid]; exists {
		ack.Code = pb_common.ErrCode_ALREADY_IN_GUILD
		tlog.Log.Info("[guild/Store.create] 玩家已有联盟，拒绝重复创建, pid=%v", uid)
		return ack
	}
	store.nextID++
	if store.nextID == 0 {
		store.nextID++
	}
	guildID := store.nextID
	guild := &Guild{
		ID:      guildID,
		Name:    req.GetName(),
		Leader:  uid,
		Members: make(map[uint64]*Member),
	}
	store.activityRecords[guildID] = activity.NewRecord(activity.ScopeGuild, guildID, activity.NewManager())
	guild.GetMembers()[uid] = &Member{
		UID:   uid,
		Name:  req.GetLeaderName(),
		Level: req.GetLeaderLevel(),
		Role:  pb_guild.GuildRole_LEADER,
	}
	guild.MarkDirty()
	store.guilds[guildID] = guild
	store.memberGuild[uid] = guildID
	ack.Code = pb_common.ErrCode_OK
	ack.Guild = guild.ToProto()
	tlog.Log.Info("[guild/Store.create] 已创建联盟, guild=%v pid=%v", guildID, uid)
	return ack
}

func (store *Store) join(req *pb_guild.JoinGuildReq) *pb_guild.JoinGuildAck {
	ack := &pb_guild.JoinGuildAck{Code: pb_common.ErrCode_PARAM}
	if store == nil || req == nil || req.GetUID() == 0 || req.GetGuildId() == 0 {
		uid := uint64(0)
		guildID := uint64(0)
		if req != nil {
			uid = req.GetUID()
			guildID = req.GetGuildId()
		}
		tlog.Log.Error("[guild/Store.join] 加入联盟参数无效, pid=%v guild=%v", uid, guildID)
		return ack
	}
	uid := req.GetUID()
	if _, exists := store.memberGuild[uid]; exists {
		ack.Code = pb_common.ErrCode_ALREADY_IN_GUILD
		tlog.Log.Info("[guild/Store.join] 玩家已有联盟，拒绝重复加入, pid=%v guild=%v", uid, req.GetGuildId())
		return ack
	}
	guild, exists := store.guilds[req.GetGuildId()]
	if !exists || guild == nil || guild.IsDeleted() {
		ack.Code = pb_common.ErrCode_GUILD_NOT_FOUND
		tlog.Log.Info("[guild/Store.join] 联盟不存在, pid=%v guild=%v", uid, req.GetGuildId())
		return ack
	}
	members := guild.GetMembers()
	if members == nil {
		members = make(map[uint64]*Member)
		guild.Members = members
	}
	beforeCount := len(members)
	members[uid] = &Member{
		UID:   uid,
		Name:  req.GetMemberName(),
		Level: req.GetMemberLevel(),
		Role:  pb_guild.GuildRole_MEMBER,
	}
	store.memberGuild[uid] = guild.GetID()
	guild.MarkDirty()
	ack.Code = pb_common.ErrCode_OK
	tlog.Log.Info("[guild/Store.join] 玩家加入联盟, guild=%v pid=%v beforeMembers=%v afterMembers=%v",
		guild.GetID(), uid, beforeCount, len(members))
	return ack
}

func (store *Store) leave(req *pb_guild.LeaveGuildReq) *pb_guild.LeaveGuildAck {
	ack := &pb_guild.LeaveGuildAck{Code: pb_common.ErrCode_PARAM}
	if store == nil || req == nil || req.GetUID() == 0 {
		uid := uint64(0)
		if req != nil {
			uid = req.GetUID()
		}
		tlog.Log.Error("[guild/Store.leave] 离开联盟参数无效, pid=%v", uid)
		return ack
	}
	uid := req.GetUID()
	guildID, exists := store.memberGuild[uid]
	if !exists {
		ack.Code = pb_common.ErrCode_NOT_IN_GUILD
		tlog.Log.Info("[guild/Store.leave] 玩家不属于任何联盟, pid=%v", uid)
		return ack
	}
	guild := store.guilds[guildID]
	if guild == nil || guild.IsDeleted() {
		delete(store.memberGuild, uid)
		ack.Code = pb_common.ErrCode_NOT_IN_GUILD
		return ack
	}
	members := guild.GetMembers()
	beforeCount := len(members)
	delete(members, uid)
	delete(store.memberGuild, uid)
	if uid == guild.GetLeader() {
		store.transferOrDisband(guild, members)
	}
	guild.MarkDirty()
	ack.Code = pb_common.ErrCode_OK
	tlog.Log.Info("[guild/Store.leave] 玩家离开联盟, guild=%v pid=%v beforeMembers=%v afterMembers=%v",
		guildID, uid, beforeCount, len(members))
	return ack
}

func (store *Store) get(req *pb_guild.GetGuildReq) *pb_guild.GetGuildAck {
	ack := &pb_guild.GetGuildAck{Code: pb_common.ErrCode_PARAM}
	if store == nil || req == nil || req.GetGuildId() == 0 {
		guildID := uint64(0)
		if req != nil {
			guildID = req.GetGuildId()
		}
		tlog.Log.Error("[guild/Store.get] 查询联盟参数无效, guild=%v", guildID)
		return ack
	}
	guild, exists := store.guilds[req.GetGuildId()]
	if !exists || guild == nil || guild.IsDeleted() {
		ack.Code = pb_common.ErrCode_GUILD_NOT_FOUND
		return ack
	}
	ack.Code = pb_common.ErrCode_OK
	ack.Guild = guild.ToProto()
	return ack
}

func (store *Store) transferOrDisband(guild *Guild, remaining map[uint64]*Member) {
	if guild == nil {
		return
	}
	if len(remaining) == 0 {
		guild.Leader = 0
		guild.Deleted = true
		tlog.Log.Info("[guild/Store.transferOrDisband] 联盟因最后一名成员离开而解散, guild=%v", guild.GetID())
		return
	}
	memberIDs := make([]uint64, 0, len(remaining))
	for uid := range remaining {
		memberIDs = append(memberIDs, uid)
	}
	sort.Slice(memberIDs, func(i, j int) bool { return memberIDs[i] < memberIDs[j] })
	newLeaderID := memberIDs[0]
	newLeader := remaining[newLeaderID]
	if newLeader == nil {
		return
	}
	newLeader.Role = pb_guild.GuildRole_LEADER
	guild.Leader = newLeaderID
	tlog.Log.Info("[guild/Store.transferOrDisband] 会长离开，已转让会长职位, guild=%v pid=%v", guild.GetID(), newLeaderID)
}

// ReplaceAll restores guild documents and rebuilds the member index after startup.
func (store *Store) ReplaceAll(guilds []*Guild) error {
	if store == nil {
		return nil
	}
	store.reset()
	for _, guild := range guilds {
		if guild == nil || guild.GetID() == 0 {
			continue
		}
		if guild.GetMembers() == nil {
			guild.Members = make(map[uint64]*Member)
		}
		legacyManager := activity.NewManager()
		legacyData := guild.GetLegacyActivities()
		if len(legacyData) > 0 {
			if err := bson.Unmarshal(legacyData, legacyManager); err != nil {
				return fmt.Errorf("decode legacy guild activity for guild %d: %w", guild.GetID(), err)
			}
		}
		record := activity.NewRecord(activity.ScopeGuild, guild.GetID(), legacyManager)
		if len(legacyData) > 0 {
			record.MarkDirty()
			guild.ClearLegacyActivities()
			guild.MarkDirty()
		}
		store.activityRecords[guild.GetID()] = record
		store.guilds[guild.GetID()] = guild
		if guild.GetID() > store.nextID {
			store.nextID = guild.GetID()
		}
		if guild.IsDeleted() {
			continue
		}
		for _, member := range guild.GetMembers() {
			if member != nil && member.GetUID() != 0 {
				store.memberGuild[member.GetUID()] = guild.GetID()
			}
		}
	}
	return nil
}

// ReplaceActivityRecords attaches loaded activity documents to guild aggregates.
func (store *Store) ReplaceActivityRecords(records []*activity.Record) {
	if store == nil {
		return
	}
	for _, record := range records {
		if record == nil || record.GetScope() != activity.ScopeGuild || record.GetOwnerID() == 0 {
			continue
		}
		guild := store.getGuild(record.GetOwnerID())
		if guild == nil {
			continue
		}
		if record.GetID() != activity.RecordID(activity.ScopeGuild, guild.GetID()) {
			tlog.Log.Error("[guild/Store.ReplaceActivityRecords] 联盟活动文档主键无效, guild=%v id=%v", guild.GetID(), record.GetID())
			continue
		}
		if record.GetManager() == nil {
			record.SetManager(activity.NewManager())
			record.MarkDirty()
		}
		store.activityRecords[guild.GetID()] = record
		if len(guild.GetLegacyActivities()) > 0 {
			guild.ClearLegacyActivities()
			guild.MarkDirty()
		}
	}
}

// RangeActivityRecords visits guild activity documents in guild ID order.
func (store *Store) RangeActivityRecords(callback func(*activity.Record) bool) {
	if store == nil || callback == nil {
		return
	}
	store.Range(func(guild *Guild) bool {
		if guild == nil {
			return true
		}
		return callback(store.activityRecords[guild.GetID()])
	})
}

// AfterActivityRecordSaved clears the current record only if its version is unchanged.
func (store *Store) AfterActivityRecordSaved(snapshot *activity.Record) {
	if store == nil || snapshot == nil {
		return
	}
	if current := store.activityRecords[snapshot.GetOwnerID()]; current != nil &&
		current.GetID() == snapshot.GetID() && current.Revision() == snapshot.Revision() {
		current.SetDirty(false)
	}
}

// Range visits guilds in ID order for persistence snapshots.
func (store *Store) Range(callback func(*Guild) bool) {
	if store == nil || callback == nil {
		return
	}
	guildIDs := make([]uint64, 0, len(store.guilds))
	for guildID := range store.guilds {
		guildIDs = append(guildIDs, guildID)
	}
	sort.Slice(guildIDs, func(i, j int) bool { return guildIDs[i] < guildIDs[j] })
	for _, guildID := range guildIDs {
		if !callback(store.guilds[guildID]) {
			return
		}
	}
}

// Count returns the number of loaded guild aggregates.
func (store *Store) Count() int {
	if store == nil {
		return 0
	}
	return len(store.guilds)
}

func (store *Store) getGuild(guildID uint64) *Guild {
	if store == nil {
		return nil
	}
	return store.guilds[guildID]
}

// SyncActivityLifecycle creates configured guild activities and advances all guild instances.
func (store *Store) SyncActivityLifecycle(now int64) int {
	if store == nil {
		return 0
	}
	changedCount := 0
	store.Range(func(guild *Guild) bool {
		if guild == nil || guild.IsDeleted() {
			return true
		}
		activityRecord := store.activityRecords[guild.GetID()]
		if activityRecord == nil {
			activityRecord = activity.NewRecord(activity.ScopeGuild, guild.GetID(), activity.NewManager())
			store.activityRecords[guild.GetID()] = activityRecord
		}
		activityMgr := activityRecord.GetManager()
		if activityMgr == nil {
			activityMgr = activity.NewManager()
			activityRecord.SetManager(activityMgr)
			activityRecord.MarkDirty()
			changedCount++
		}
		activityChanged, transitions := activity.DefaultRuntime().Sync(
			activityMgr,
			activity.ScopeGuild,
			guild.GetID(),
			activity.DefaultCatalog().List(activity.ScopeGuild),
			now,
		)
		if activityChanged {
			activityRecord.MarkDirty()
			changedCount++
			tlog.Log.Info("[guild/Store.SyncActivityLifecycle] 已同步联盟活动模板, activities=%v guild=%v",
				len(activityMgr.List()), guild.GetID())
		}
		if len(transitions) > 0 {
			for _, transition := range transitions {
				tlog.Log.Info("[guild/Store.SyncActivityLifecycle] 联盟活动状态已变更, activity=%v from=%v to=%v guild=%v",
					transition.GetTemplateID(), transition.GetFrom(), transition.GetTo(), guild.GetID())
			}
		}
		return true
	})
	return changedCount
}

func (store *Store) reset() {
	store.guilds = make(map[uint64]*Guild)
	store.activityRecords = make(map[uint64]*activity.Record)
	store.memberGuild = make(map[uint64]uint64)
	store.nextID = 0
}
