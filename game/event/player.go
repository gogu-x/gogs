package event

import (
	"strconv"
	"time"

	"github.com/gogu-x/gogs/pb/sspb"
)

const (
	TypeGuild             int32 = 3                  // 联盟归属事件类型。
	ParamGuildID                = "guild_id"         // 扩展参数中的联盟编号。
	TypePlayerLifecycle   int32 = 2                  // 玩家生命周期事件类型。
	NamePlayerInitialized       = "playerInitialize" // 玩家活动初始化入口。
	NamePlayerReleased          = "playerReleased"   // 玩家数据释放。
	ParamUID                    = "uid"              // 扩展参数中的玩家编号。
	ParamGeneration             = "generation"       // 扩展参数中的会话代次。
	ParamOccurredAt             = "occurred_at"      // 扩展参数中的事件发生时间，Unix 纳秒。
)

// NewPlayer 构造玩家类型事件，业务参数统一放入通用扩展参数，不扩充 Event 顶层。
func NewPlayer(name string, uid uint64, generation uint64) *sspb.Event {
	return &sspb.Event{Name: name, EventTyp: TypePlayerLifecycle, ExtArgs: map[string]string{
		ParamUID:        strconv.FormatUint(uid, 10),
		ParamGeneration: strconv.FormatUint(generation, 10),
		ParamOccurredAt: strconv.FormatInt(time.Now().UnixNano(), 10),
	}}
}

// Uint64Param 读取扩展参数中的无符号整数，缺失或无效时返回零。
func Uint64Param(message *sspb.Event, key string) uint64 {
	value, err := strconv.ParseUint(message.GetExtArgs()[key], 10, 64)
	if err != nil {
		return 0
	}
	return value
}

// PlayerUID 读取玩家事件的归属编号，不用于判断事件类型。
func PlayerUID(message *sspb.Event) uint64 { return Uint64Param(message, ParamUID) }

// PlayerGeneration 读取玩家事件的会话代次。
func PlayerGeneration(message *sspb.Event) uint64 { return Uint64Param(message, ParamGeneration) }

// PlayerEventTime 读取事件扩展时间，兼容未携带时间的旧事件。
func PlayerEventTime(message *sspb.Event) time.Time {
	if at, err := strconv.ParseInt(message.GetExtArgs()[ParamOccurredAt], 10, 64); err == nil && at > 0 {
		return time.Unix(0, at).In(time.Local)
	}
	return time.Now()
}

// IsPlayerInitialized 判断是否为明确的玩家活动初始化事件。
func IsPlayerInitialized(message *sspb.Event) bool {
	return message != nil && message.GetEventTyp() == TypePlayerLifecycle && message.GetName() == NamePlayerInitialized
}

// IsPlayerReleased 判断是否为玩家活动数据释放事件。
func IsPlayerReleased(message *sspb.Event) bool {
	return message != nil && message.GetEventTyp() == TypePlayerLifecycle && message.GetName() == NamePlayerReleased
}
