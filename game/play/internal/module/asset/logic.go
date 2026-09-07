package asset

import (
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
)

func OnLogin(arg *comm.Arg) {
	ctx, _ := arg.Get("ctx")
	tlog.Log.Info("OnLogin ctx", ctx)
}
