//go:generate deepcopy-gen

package store

import (
	"sort"

	"github.com/gogu-x/gogs/pb/cspb/pb_guild"
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
