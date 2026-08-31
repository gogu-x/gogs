package internal

import (
	"sync/atomic"

	"github.com/gogu-x/gogs/pb/pb_common"
	"github.com/gogu-x/gogs/pb/pb_guild"
	"github.com/gogu-x/tree"
)

type Member struct {
	UID   uint64
	Name  string
	Level uint32
	Role  pb_guild.GuildRole
}

func (m *Member) ToProto() *pb_guild.GuildMember {
	return &pb_guild.GuildMember{Uid: m.UID, Name: m.Name, Level: m.Level, Role: m.Role}
}

type Guild struct {
	ID      uint64
	Name    string
	Notice  string
	Leader  uint64
	Members map[uint64]*Member
}

func (g *Guild) ToProto() *pb_guild.GuildInfo {
	members := make([]*pb_guild.GuildMember, 0, len(g.Members))
	for _, m := range g.Members {
		members = append(members, m.ToProto())
	}
	return &pb_guild.GuildInfo{
		GuildId: g.ID, Name: g.Name, Notice: g.Notice,
		Leader: g.Leader, Members: members,
	}
}

var idGen atomic.Uint64

type Store struct {
	guilds map[uint64]*Guild
}

func NewStore() *Store {
	return &Store{guilds: make(map[uint64]*Guild)}
}

func (s *Store) Create(ctx tree.Context, req *pb_guild.CreateGuildReq) {
	id := idGen.Add(1)
	uid := req.GetUID()
	g := &Guild{
		ID: id, Name: req.Name, Leader: uid,
		Members: map[uint64]*Member{
			uid: {UID: uid, Name: req.GetLeaderName(), Level: req.GetLeaderLevel(), Role: pb_guild.GuildRole_LEADER},
		},
	}
	s.guilds[id] = g
	ctx.Response(&pb_guild.CreateGuildAck{Guild: g.ToProto()}, nil)
}

func (s *Store) Join(ctx tree.Context, req *pb_guild.JoinGuildReq) {
	ack := &pb_guild.JoinGuildAck{Code: pb_common.ErrCode_OK}
	defer ctx.Response(ack, nil)
	uid := req.GetUID()
	g, ok := s.guilds[req.GuildId]
	if !ok {
		ack.Code = pb_common.ErrCode_ERR_GUILD_NOT_FOUND
		return
	}
	g.Members[uid] = &Member{UID: uid, Name: req.GetMemberName(), Level: req.GetMemberLevel(), Role: pb_guild.GuildRole_MEMBER}
}

func (s *Store) Leave(_ tree.Context, req *pb_guild.LeaveGuildReq) {}

func (s *Store) Get(ctx tree.Context, req *pb_guild.GetGuildReq) {
	ack := &pb_guild.GetGuildAck{Code: pb_common.ErrCode_OK}
	defer ctx.Response(ack, nil)
	g, ok := s.guilds[req.GuildId]
	if !ok {
		ack.Code = pb_common.ErrCode_ERR_GUILD_NOT_FOUND
		return
	}
	ack.Guild = g.ToProto()
}
