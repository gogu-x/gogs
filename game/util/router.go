package util

import (
	"github.com/gogu-x/tree"
)

// Register 将泛型 handler 注册到 Router，省去类型断言
func Register[Req any](r *tree.Router, req Req, fn func(tree.Context, Req)) {
	r.Register(req, func(ctx tree.Context, msg interface{}) {
		fn(ctx, msg.(Req))
	})
}
