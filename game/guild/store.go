package guild

import (
	"sync/atomic"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/pb/protoGuild"
)

// Member 工会成员快照
type Member struct {
	UID   uint64
	Name  string
	Level uint32
	Role  protoGuild.GuildRole
}

func (m *Member) ToProto() *protoGuild.GuildMember {
	return &protoGuild.GuildMember{Uid: m.UID, Name: m.Name, Level: m.Level, Role: m.Role}
}

// Guild 工会数据
type Guild struct {
	ID      uint64
	Name    string
	Notice  string
	Leader  uint64
	Members map[uint64]*Member
}

func (g *Guild) ToProto() *protoGuild.GuildInfo {
	members := make([]*protoGuild.GuildMember, 0, len(g.Members))
	for _, m := range g.Members {
		members = append(members, m.ToProto())
	}
	return &protoGuild.GuildInfo{
		GuildId: g.ID, Name: g.Name, Notice: g.Notice,
		Leader: g.Leader, Members: members,
	}
}

// UpdateMemberMsg 玩家数据变化时通知 GuildActor 同步快照
type UpdateMemberMsg struct {
	UID   uint64
	Name  string
	Level uint32
}

var idGen atomic.Uint64

// Store 工会数据存储，纯数据操作，无业务逻辑，在 GuildActor 单 goroutine 内使用
type Store struct {
	guilds map[uint64]*Guild // guildID → Guild
}

func NewStore() *Store {
	return &Store{
		guilds: make(map[uint64]*Guild),
	}
}

func (s *Store) Create(ctx actor.ActorContext, req *protoGuild.CreateGuildReq) {
	uid := req.GetUid()

	id := idGen.Add(1)
	g := &Guild{
		ID: id, Name: req.Name, Leader: uid,
		Members: map[uint64]*Member{
			uid: {UID: uid, Name: req.GetLeaderName(), Level: req.GetLeaderLevel(), Role: protoGuild.GuildRole_LEADER},
		},
	}
	s.guilds[id] = g
	ctx.Response(&protoGuild.CreateGuildResp{Guild: g.ToProto()}, nil)
}

func (s *Store) Join(ctx actor.ActorContext, req *protoGuild.JoinGuildReq) {
	ack := &protoGuild.JoinGuildResp{Code: 1}
	defer ctx.Response(ack, nil)
	uid := req.GetUid()

	g, ok := s.guilds[req.GuildId]
	if !ok {
		ack.Code = 3
		return
	}
	g.Members[uid] = &Member{UID: uid, Name: req.GetMemberName(), Level: req.GetMemberLevel(), Role: protoGuild.GuildRole_MEMBER}
}

func (s *Store) Leave(_ actor.ActorContext, req *protoGuild.LeaveGuildReq) {

}

func (s *Store) Get(ctx actor.ActorContext, req *protoGuild.GetGuildReq) {
	ack := &protoGuild.GetGuildResp{Code: 1}
	defer ctx.Response(ack, nil)
	g, ok := s.guilds[req.GuildId]
	if !ok {
		return
	}
	ack.Guild = g.ToProto()
}

func (s *Store) UpdateMember(uid uint64, name string, level uint32) {

}
