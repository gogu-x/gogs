package internal

import (
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
)

// InitMessageHandlers registers server activity Actor message handlers.
func InitMessageHandlers(
	router *tree.Router,
	onServerEvent func(tree.Context, *sspb.Event),
	onSaveAllActivities func(tree.Context, *ipb.SaveAllActivities),
	fallback tree.Handler,
) {
	tree.RegisterFunc(router, &sspb.Event{}, onServerEvent)
	tree.RegisterFunc(router, &ipb.SaveAllActivities{}, onSaveAllActivities)
	router.SetFallback(fallback)
}
