package app

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/game/activity"
	"github.com/gogu-x/gogs/game/guild"
	"github.com/gogu-x/gogs/game/player"
	"github.com/gogu-x/gogs/pb/gateway"
	"log"
	"reflect"

	"google.golang.org/protobuf/proto"
)

type App struct {
	Players  *player.Manager
	Guild    *guild.Manager
	Activity *activity.Manager
}

func New() *App {
	return &App{
		Players: player.NewManager(),
	}
}

// Context wraps ActorContext and App for each message handler call.
type Context struct {
	actor.ActorContext
	*App
	UID    uint64
	ConnID uint64
}

func NewContext(ctx actor.ActorContext, a *App, uid, connID uint64) *Context {
	return &Context{ActorContext: ctx, App: a, UID: uid, ConnID: connID}
}

func (a *App) Handle(fn func(*Context, interface{})) actor.Handler {
	return func(ctx actor.ActorContext, msg interface{}) {
		c := ctx.(*gameContext)
		fn(NewContext(c.ActorContext, a, c.uid, c.connID), msg)
	}
}

type gameContext struct {
	actor.ActorContext
	uid    uint64
	connID uint64
}

func WrapContext(ctx actor.ActorContext, uid, connID uint64) actor.ActorContext {
	return &gameContext{ActorContext: ctx, uid: uid, connID: connID}
}

// Reply wraps msg into a gateway.Frame and sends it back to GateActor.
func (c *Context) Reply(msg proto.Message) {
	gate, ok := c.Lookup("gate")
	if !ok {
		return
	}
	body, err := codec.ProtoCodec.Marshal(msg)
	if err != nil {
		log.Printf("Reply marshal error: %v", err)
		return
	}
	c.Send(gate, &gateway.Frame{
		Uid:     c.UID,
		ConnId:  c.ConnID,
		Payload: body,
		MsgType: reflect.TypeOf(msg).Elem().Name(),
	})
}
