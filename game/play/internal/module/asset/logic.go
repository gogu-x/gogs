package asset

import (
	"fmt"

	"github.com/gogu-x/gogs/comm"
)

func OnLogin(arg *comm.Arg) {
	fmt.Println(arg)
}
