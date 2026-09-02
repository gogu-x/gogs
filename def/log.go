package def

import (
	"github.com/gogu-x/tree/log"
)

var DLog *log.Logger

func NewLog(pathname string, flag int) {
	logs, err := log.New("debug", pathname, flag)
	if err != nil {
		panic(err)
	}
	DLog = logs
}
