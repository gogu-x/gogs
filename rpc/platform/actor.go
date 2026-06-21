package platform

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"time"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/pb/protoPlatform"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type route struct {
	method  string
	newResp func() any
}

type Actor struct {
	conn   *grpc.ClientConn
	routes map[reflect.Type]route
}

func NewActor() *Actor { return &Actor{routes: make(map[reflect.Type]route)} }

func (a *Actor) register(req any, method string, newResp func() any) {
	a.routes[reflect.TypeOf(req)] = route{method, newResp}
}

func (a *Actor) OnInit(_ actor.ActorContext) {
	conn, err := grpc.NewClient(config.PlatformGrpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		protoPlatform.ForceJSONCodec(),
	)
	if err != nil {
		log.Fatalf("rpc/platform: dial %s: %v", config.PlatformGrpcAddr, err)
	}
	a.conn = conn
	registerRoutes(a)
	log.Printf("rpc/platform: connected to %s", config.PlatformGrpcAddr)
}

func (a *Actor) HandleMessage(ctx actor.ActorContext, msg any) {
	r, ok := a.routes[reflect.TypeOf(msg)]
	if !ok {
		ctx.Response(nil, fmt.Errorf("rpc/platform: no route for %T", msg))
		return
	}
	rctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp := r.newResp()
	if err := a.conn.Invoke(rctx, r.method, msg, resp); err != nil {
		ctx.Response(nil, err)
		return
	}
	ctx.Response(resp, nil)
}

func (a *Actor) OnStop(_ actor.ActorContext) {
	if a.conn != nil {
		a.conn.Close()
	}
}
