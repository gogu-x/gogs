package glconf

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gogu-x/tree/comm"
)

const (
	// actvAbsoluteStart 使用绝对日期。
	actvAbsoluteStart = 1
	// actvServerOpenStart 使用相对于开服日期的偏移。
	actvServerOpenStart = 2
	// actvParentStart 跟随指定的父活动。
	actvParentStart = 5
	// actvDateLayout 是活动绝对日期格式。
	actvDateLayout = "2006-01-02 15:04:05"
)

// ErrUnsupportedActvTiming 表示活动时间类型不支持服务器定时创建。
var ErrUnsupportedActvTiming = errors.New("unsupported actv trigger type")

// ActvServerTiming 保存已换算为秒的服务器活动时间。
type ActvServerTiming struct {
	firstStart, earliestStart int64
	duration, closeAfterEnd   int64
	forecast, interval        int64
}

// GetFirstStart 返回首轮开始时间。
func (timing ActvServerTiming) GetFirstStart() int64 { return timing.firstStart }

// GetEarliestStart 返回允许开启的最早时间。
func (timing ActvServerTiming) GetEarliestStart() int64 { return timing.earliestStart }

// GetDuration 返回每轮的持续秒数。
func (timing ActvServerTiming) GetDuration() int64 { return timing.duration }

// GetCloseAfterEnd 返回活动结束后的保留秒数。
func (timing ActvServerTiming) GetCloseAfterEnd() int64 { return timing.closeAfterEnd }

// GetForecast 返回活动预告秒数。
func (timing ActvServerTiming) GetForecast() int64 { return timing.forecast }

// GetInterval 返回活动循环间隔秒数。
func (timing ActvServerTiming) GetInterval() int64 { return timing.interval }

// ActvTiming 保存活动表加载时解析的时间字段。
type ActvTiming struct {
	configID, triggerType, parentID int32
	parentValid                     bool
	playerDuration                  int64
	playerDurationError             error
	server                          ActvServerTiming
	serverError, startError         error
	openDayLimit, offset            int64
	absolute                        time.Time
}

// GetPlayerDuration 返回个人活动配置的时长及解析错误。
func (timing ActvTiming) GetPlayerDuration() (int64, error) {
	return timing.playerDuration, timing.playerDurationError
}

// GetParentID 返回已解析的父活动配置编号。
func (timing ActvTiming) GetParentID() (int32, bool) { return timing.parentID, timing.parentValid }

// ServerTiming 将已解析的时间绑定到当前区服的开服时间和时区。
func (timing ActvTiming) ServerTiming(openTs int64, loc *time.Location) (ActvServerTiming, error) {
	if timing.serverError != nil {
		return ActvServerTiming{}, timing.serverError
	}
	result := timing.server
	if timing.openDayLimit > 0 {
		if openTs <= 0 {
			return ActvServerTiming{}, fmt.Errorf("config %d open_day_limit requires server open time", timing.configID)
		}
		result.earliestStart = openTs + timing.openDayLimit
	}
	if timing.triggerType == actvServerOpenStart && openTs <= 0 {
		return ActvServerTiming{}, fmt.Errorf("config %d trigger type %d requires server open time", timing.configID, timing.triggerType)
	}
	if timing.startError != nil {
		return ActvServerTiming{}, timing.startError
	}
	if loc == nil {
		loc = time.Local
	}
	switch timing.triggerType {
	case actvAbsoluteStart:
		date := timing.absolute
		result.firstStart = time.Date(date.Year(), date.Month(), date.Day(), date.Hour(), date.Minute(), date.Second(), date.Nanosecond(), loc).Unix()
	case actvServerOpenStart:
		start, _ := comm.DayBounds(time.Unix(openTs, 0).In(loc))
		result.firstStart = start + timing.offset
	}
	return result, nil
}

// ActvTimingCatalog 保存本次加载的活动配置和对应时间字段。
type ActvTimingCatalog struct {
	rows   []*ActvOnlineCfg
	timing map[*ActvOnlineCfg]ActvTiming
}

func (catalog *ActvTimingCatalog) getTiming() map[*ActvOnlineCfg]ActvTiming { return catalog.timing }

// GetRows 返回本次加载的活动配置行，列表供只读使用。
func (catalog *ActvTimingCatalog) GetRows() []*ActvOnlineCfg {
	if catalog == nil {
		return nil
	}
	return catalog.rows
}

var actvTimingCatalog atomic.Pointer[ActvTimingCatalog]

// GetActvTimingCatalog 返回活动表的时间配置快照。
func GetActvTimingCatalog() *ActvTimingCatalog { return actvTimingCatalog.Load() }

func loadActvTiming(table *CsvConf) {
	rows := make([]*ActvOnlineCfg, 0, table.NumRecord())
	timing := make(map[*ActvOnlineCfg]ActvTiming, table.NumRecord())
	for index := range table.NumRecord() {
		row := table.Record(index).(*ActvOnlineCfg)
		rows = append(rows, row)
		timing[row] = parseActvTiming(row)
	}
	actvTimingCatalog.Store(&ActvTimingCatalog{rows: rows, timing: timing})
}

// GetActvTiming 返回配置行的时间规则，也接受调用方直接构造的活动配置行。
func GetActvTiming(row *ActvOnlineCfg) ActvTiming {
	if catalog := GetActvTimingCatalog(); catalog != nil {
		if timing, ok := catalog.getTiming()[row]; ok {
			return timing
		}
	}
	return parseActvTiming(row)
}

func parseActvTiming(row *ActvOnlineCfg) ActvTiming {
	id, triggerType := row.GetID(), row.GetTriggerType()
	duration, durationError := parseActvDuration(row.GetDuration())
	timing := ActvTiming{configID: id, triggerType: triggerType, playerDuration: duration, playerDurationError: durationError}
	value := row.GetTriggerVal()
	if triggerType == actvParentStart {
		parent, err := strconv.ParseInt(value, 10, 32)
		timing.parentID, timing.parentValid = int32(parent), err == nil
	}
	if durationError != nil {
		timing.serverError = fmt.Errorf("config %d duration: %w", id, durationError)
		return timing
	}
	if duration <= 0 {
		timing.serverError = fmt.Errorf("config %d duration must be positive, got %q", id, row.GetDuration())
		return timing
	}
	timing.server = ActvServerTiming{duration: duration}
	for _, field := range []struct {
		name, value string
		output      *int64
	}{
		{"close_after_end", row.GetCloseAfterEnd(), &timing.server.closeAfterEnd},
		{"forecast_before_start", row.GetForecastBeforeStart(), &timing.server.forecast},
		{"trigger_rept", row.GetTriggerRept(), &timing.server.interval},
		{"open_day_limit", row.GetOpenDayLimit(), &timing.openDayLimit},
	} {
		seconds, err := parseActvDuration(field.value)
		if err != nil {
			timing.serverError = fmt.Errorf("config %d %s: %w", id, field.name, err)
			return timing
		}
		*field.output = seconds
	}
	switch triggerType {
	case actvAbsoluteStart:
		date, err := time.Parse(actvDateLayout, strings.TrimSpace(value))
		timing.absolute = date
		if err != nil {
			timing.startError = fmt.Errorf("config %d trigger_val date: %w", id, err)
		}
	case actvServerOpenStart:
		offset, err := parseActvDuration(value)
		timing.offset = offset
		if err != nil {
			timing.startError = fmt.Errorf("config %d trigger_val offset: %w", id, err)
		}
	default:
		timing.startError = fmt.Errorf("config %d trigger type %d: %w", id, triggerType, ErrUnsupportedActvTiming)
	}
	return timing
}

func parseActvDuration(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return 0, nil
	}
	var seconds int64
	switch value[len(value)-1] {
	case 'd':
		seconds = 86400
	case 'h':
		seconds = 3600
	case 'm':
		seconds = 60
	case 's':
		seconds = 1
	default:
		return 0, fmt.Errorf("invalid duration %q: unit must be d/h/m/s", value)
	}
	number, err := strconv.ParseInt(value[:len(value)-1], 10, 64)
	if err != nil || number < 0 {
		return 0, fmt.Errorf("invalid duration %q", value)
	}
	return number * seconds, nil
}
