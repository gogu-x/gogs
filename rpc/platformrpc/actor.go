package platformrpc

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type route struct {
	method  string
	newResp func() any
}

type PfRpc struct {
	conn   *grpc.ClientConn
	routes map[reflect.Type]route
}

func NewActor() *PfRpc { return &PfRpc{routes: make(map[reflect.Type]route)} }

func (a *PfRpc) Name() string { return def.PF }

func (a *PfRpc) register(req any, method string, newResp func() any) {
	a.routes[reflect.TypeOf(req)] = route{method, newResp}
}

func (a *PfRpc) OnInit(_ tree.Context) {
	conn, err := grpc.NewClient(conf.PlatformGrpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		tlog.Log.Info("rpc/platform: dial %s: %v", conf.PlatformGrpcAddr, err)
	}
	a.conn = conn
	registerRoutes(a)
	tlog.Log.Info("rpc/platform: connected to %s", conf.PlatformGrpcAddr)
}

func (a *PfRpc) HandleMessage(ctx tree.Context, msg any) {
	r, ok := a.routes[reflect.TypeOf(msg)]
	if !ok {
		ctx.Response(nil, fmt.Errorf("rpc/platform: no route for %T", msg))
		return
	}
	f := ctx.RequestEnvelope()
	conn := a.conn
	go func() {
		rctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp := r.newResp()
		if err := conn.Invoke(rctx, r.method, msg, resp); err != nil {
			f.Respond(nil, err)
			return
		}
		f.Respond(resp, nil)
	}()
}

func (a *PfRpc) OnStop(_ tree.Context) {
	if a.conn != nil {
		a.conn.Close()
	}
}
