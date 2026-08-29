package asset

import (
	"fmt"

	"github.com/gogu-x/tree/comm"
)

func OnLogin(arg *comm.Arg) {
	fmt.Println(arg)
}
