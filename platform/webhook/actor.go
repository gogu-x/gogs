package webhook

import (
	"log"
	"net/http"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/tree"
)

type Actor struct{}

func (a *Actor) Name() string { return def.Web }

func (a *Actor) OnInit(_ tree.Context) {
	mux := http.NewServeMux()
	go func() {
		log.Printf("platform HTTP webhook on %s", conf.PlatformWebhookAddr)
		if err := http.ListenAndServe(conf.PlatformWebhookAddr, mux); err != nil {
			log.Printf("webhook: %v", err)
		}
	}()
}

func (a *Actor) HandleMessage(_ tree.Context, _ interface{}) {}
func (a *Actor) OnStop(_ tree.Context)                       {}
