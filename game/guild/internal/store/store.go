package store

import (
	"sort"

	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_guild"
	"github.com/gogu-x/tree/tlog"
)

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

// Create 创建联盟并返回客户端响应。
func (store *Store) Create(req *pb_guild.CreateGuildReq) *pb_guild.CreateGuildAck {
	ack := &pb_guild.CreateGuildAck{Code: pb_common.ErrCode_PARAM}
	if store == nil || req == nil || req.GetUID() == 0 || req.GetName() == "" {
		uid := uint64(0)
		if req != nil {
			uid = req.GetUID()
		}
		tlog.Log.Error("[guild/Store.Create] 创建联盟参数无效, pid=%v", uid)
		return ack
	}
	uid := req.GetUID()
	if _, exists := store.memberGuild[uid]; exists {
		ack.Code = pb_common.ErrCode_ALREADY_IN_GUILD
		tlog.Log.Info("[guild/Store.Create] 玩家已有联盟，拒绝重复创建, pid=%v", uid)
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
	tlog.Log.Info("[guild/Store.Create] 已创建联盟, guild=%v pid=%v", guildID, uid)
	return ack
}

// Join 将玩家加入联盟并返回客户端响应。
func (store *Store) Join(req *pb_guild.JoinGuildReq) *pb_guild.JoinGuildAck {
	ack := &pb_guild.JoinGuildAck{Code: pb_common.ErrCode_PARAM}
	if store == nil || req == nil || req.GetUID() == 0 || req.GetGuildId() == 0 {
		uid := uint64(0)
		guildID := uint64(0)
		if req != nil {
			uid = req.GetUID()
			guildID = req.GetGuildId()
		}
		tlog.Log.Error("[guild/Store.Join] 加入联盟参数无效, pid=%v guild=%v", uid, guildID)
		return ack
	}
	uid := req.GetUID()
	if _, exists := store.memberGuild[uid]; exists {
		ack.Code = pb_common.ErrCode_ALREADY_IN_GUILD
		tlog.Log.Info("[guild/Store.Join] 玩家已有联盟，拒绝重复加入, pid=%v guild=%v", uid, req.GetGuildId())
		return ack
	}
	guild, exists := store.guilds[req.GetGuildId()]
	if !exists || guild == nil || guild.IsDeleted() {
		ack.Code = pb_common.ErrCode_GUILD_NOT_FOUND
		tlog.Log.Info("[guild/Store.Join] 联盟不存在, pid=%v guild=%v", uid, req.GetGuildId())
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
	tlog.Log.Info("[guild/Store.Join] 玩家加入联盟, guild=%v pid=%v beforeMembers=%v afterMembers=%v",
		guild.GetID(), uid, beforeCount, len(members))
	return ack
}

// Leave 将玩家移出其所属联盟并返回客户端响应。
func (store *Store) Leave(req *pb_guild.LeaveGuildReq) *pb_guild.LeaveGuildAck {
	ack := &pb_guild.LeaveGuildAck{Code: pb_common.ErrCode_PARAM}
	if store == nil || req == nil || req.GetUID() == 0 {
		uid := uint64(0)
		if req != nil {
			uid = req.GetUID()
		}
		tlog.Log.Error("[guild/Store.Leave] 离开联盟参数无效, pid=%v", uid)
		return ack
	}
	uid := req.GetUID()
	guildID, exists := store.memberGuild[uid]
	if !exists {
		ack.Code = pb_common.ErrCode_NOT_IN_GUILD
		tlog.Log.Info("[guild/Store.Leave] 玩家不属于任何联盟, pid=%v", uid)
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
	tlog.Log.Info("[guild/Store.Leave] 玩家离开联盟, guild=%v pid=%v beforeMembers=%v afterMembers=%v",
		guildID, uid, beforeCount, len(members))
	return ack
}

// Get 返回联盟的客户端数据快照。
func (store *Store) Get(req *pb_guild.GetGuildReq) *pb_guild.GetGuildAck {
	ack := &pb_guild.GetGuildAck{Code: pb_common.ErrCode_PARAM}
	if store == nil || req == nil || req.GetGuildId() == 0 {
		guildID := uint64(0)
		if req != nil {
			guildID = req.GetGuildId()
		}
		tlog.Log.Error("[guild/Store.Get] 查询联盟参数无效, guild=%v", guildID)
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

// Range 按联盟 ID 顺序遍历内存中的联盟数据。
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

// GetPlayerGuildID 返回玩家所属联盟编号，未加入联盟时返回 0。
func (store *Store) GetPlayerGuildID(uid uint64) uint64 {
	return store.memberGuild[uid]
}

func (store *Store) getGuild(guildID uint64) *Guild {
	if store == nil {
		return nil
	}
	return store.guilds[guildID]
}

// reset 清空联盟数据及成员索引。
func (store *Store) reset() {
	store.guilds = make(map[uint64]*Guild)
	store.memberGuild = make(map[uint64]uint64)
	store.nextID = 0
}
