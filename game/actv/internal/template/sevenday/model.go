package sevenday

// LoginData 保存个人七日登录进度，重复登录与重启均按自然日去重。
// +deepcopy-gen=true
//
//go:generate accessor-gen
type LoginData struct {
	DayStart   int64 `bson:"day_start"`   // 最近处理的服务器自然日起点。
	DailyCount int32 `bson:"daily_count"` // 当天有效登录次数，只取 0 或 1。
	LoginDays  int32 `bson:"login_days"`  // 累计有效登录天数，最多七天。
}
