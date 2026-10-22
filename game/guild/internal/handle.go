package internal

import (
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
)

// InitMessageHandlers registers Guild Actor message handlers.
func InitMessageHandlers(
	router *tree.Router,
	onServerEvent func(tree.Context, *sspb.Event),
	onSaveAllGuilds func(tree.Context, *ipb.SaveAllGuilds),
	fallback tree.Handler,
) {
	tree.RegisterFunc(router, &sspb.Event{}, onServerEvent)
	tree.RegisterFunc(router, &ipb.SaveAllGuilds{}, onSaveAllGuilds)
	router.SetFallback(fallback)
}
