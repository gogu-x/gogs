//go:generate go run ../../../tools/deepcopy-gen -dir ../..

package internal

import (
	"sort"

	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_guild"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

// Member 保存一名玩家在联盟中的成员数据。
// +deepcopy-gen=true
type Member struct {
	UID   uint64             `bson:"uid"`   // 玩家唯一 ID。
	Name  string             `bson:"name"`  // 玩家名称快照。
	Level uint32             `bson:"level"` // 玩家等级快照。
	Role  pb_guild.GuildRole `bson:"role"`  // 联盟职位。
}

// GetUID 返回成员的玩家 ID。
func (member *Member) GetUID() uint64 {
	if member == nil {
		return 0
	}
	return member.UID
}

// GetName 返回成员名称快照。
func (member *Member) GetName() string {
	if member == nil {
		return ""
	}
	return member.Name
}

// GetLevel 返回成员等级快照。
func (member *Member) GetLevel() uint32 {
	if member == nil {
		return 0
	}
	return member.Level
}

// GetRole 返回成员当前联盟职位。
func (member *Member) GetRole() pb_guild.GuildRole {
	if member == nil {
		return pb_guild.GuildRole_MEMBER
	}
	return member.Role
}

// ToProto 创建供客户端使用的成员快照。
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

// Guild 保存全局联盟 Actor 管理的联盟聚合数据。
// +deepcopy-gen=true
type Guild struct {
	ID       uint64             `bson:"_id"`     // 联盟唯一 ID。
	Name     string             `bson:"name"`    // 联盟名称。
	Notice   string             `bson:"notice"`  // 联盟公告。
	Leader   uint64             `bson:"leader"`  // 当前会长玩家 ID。
	Members  map[uint64]*Member `bson:"members"` // 按玩家 ID 索引的成员。
	Deleted  bool               `bson:"deleted"` // 联盟是否已解散。
	Dirty    bool               `bson:"-"`       // 是否需要存盘。
	revision uint64             `bson:"-"`       // 内存版本号，不写入联盟文档。
}

// GetID 返回联盟 ID。
func (guild *Guild) GetID() uint64 {
	if guild == nil {
		return 0
	}
	return guild.ID
}

// GetName 返回联盟名称。
func (guild *Guild) GetName() string {
	if guild == nil {
		return ""
	}
	return guild.Name
}

// GetNotice 返回联盟公告。
func (guild *Guild) GetNotice() string {
	if guild == nil {
		return ""
	}
	return guild.Notice
}

// GetLeader 返回会长玩家 ID。
func (guild *Guild) GetLeader() uint64 {
	if guild == nil {
		return 0
	}
	return guild.Leader
}

// IsDeleted 返回联盟是否已解散。
func (guild *Guild) IsDeleted() bool {
	return guild == nil || guild.Deleted
}

// GetMembers 返回按玩家 ID 索引的联盟成员。
func (guild *Guild) GetMembers() map[uint64]*Member {
	if guild == nil {
		return nil
	}
	return guild.Members
}

// MarkDirty 标记联盟文档已修改并递增版本号。
func (guild *Guild) MarkDirty() {
	if guild == nil {
		return
	}
	guild.SetDirty(true)
}

// IsDirty 返回联盟文档是否需要存盘。
func (guild *Guild) IsDirty() bool {
	return guild != nil && guild.Dirty
}

// Revision 返回用于保护并发存盘的内存版本号。
func (guild *Guild) Revision() uint64 {
	if guild == nil {
		return 0
	}
	return guild.revision
}

// SetDirty 更新联盟文档的存盘标记。
func (guild *Guild) SetDirty(dirty bool) {
	if guild == nil {
		return
	}
	if dirty {
		guild.revision++
	}
	guild.Dirty = dirty
}

// ToProto 创建成员顺序稳定的客户端联盟快照。
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

// Store 管理所有联盟聚合数据和玩家到联盟的成员索引。
type Store struct {
	guilds      map[uint64]*Guild
	memberGuild map[uint64]uint64
	nextID      uint64
}

// NewStore 创建空的联盟数据仓库。
func NewStore() *Store {
	return &Store{
		guilds:      make(map[uint64]*Guild),
		memberGuild: make(map[uint64]uint64),
	}
}

// Create 创建联盟并返回结果。
func (store *Store) Create(req *pb_guild.CreateGuildReq) *pb_guild.CreateGuildAck {
	return store.create(req)
}

// Join 处理加入联盟请求。
func (store *Store) Join(ctx tree.Context, req *pb_guild.JoinGuildReq) {
	ctx.Response(store.join(req), nil)
}

// Leave 处理退出联盟请求。
func (store *Store) Leave(ctx tree.Context, req *pb_guild.LeaveGuildReq) {
	ctx.Response(store.leave(req), nil)
}

// Get 处理查询联盟请求。
func (store *Store) Get(ctx tree.Context, req *pb_guild.GetGuildReq) {
	ctx.Response(store.get(req), nil)
}

// HandlePending 在联盟文档加载完成后执行排队请求。
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
	default:
		return nil, false
	}
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

// ReplaceAll 在启动后恢复联盟文档并重建成员索引。
func (store *Store) ReplaceAll(guilds []*Guild) {
	if store == nil {
		return
	}
	store.reset()
	for _, guild := range guilds {
		if guild == nil {
			continue
		}
		if guildID := guild.GetID(); guildID == 0 {
			continue
		} else {
			if guild.GetMembers() == nil {
				guild.Members = make(map[uint64]*Member)
			}
			store.guilds[guildID] = guild
			if guildID > store.nextID {
				store.nextID = guildID
			}
			if guild.IsDeleted() {
				continue
			}
			for _, member := range guild.GetMembers() {
				if member != nil && member.GetUID() != 0 {
					store.memberGuild[member.GetUID()] = guildID
				}
			}
		}
	}
}

// Range 按联盟 ID 顺序遍历存盘快照。
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

// Count 返回已加载的联盟数量。
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

// reset 清空联盟和活动的内存索引。
func (store *Store) reset() {
	store.guilds = make(map[uint64]*Guild)
	store.memberGuild = make(map[uint64]uint64)
	store.nextID = 0
}
