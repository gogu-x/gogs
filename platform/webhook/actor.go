package webhook

import (
	"log"
	"net/http"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/config"
	platformgrpc "github.com/gogu-x/gogs/platform/grpc"
)

type Actor struct{}

func (a *Actor) OnInit(_ actor.ActorContext) {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/pay", func(w http.ResponseWriter, r *http.Request) {
		// TODO: 验证支付平台签名
		orderID := r.URL.Query().Get("order_id")
		if orderID == "" {
			http.Error(w, "missing order_id", http.StatusBadRequest)
			return
		}
		if err := platformgrpc.DeliverByOrderID(orderID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	go func() {
		log.Printf("platform HTTP webhook on %s", config.PlatformWebhookAddr)
		if err := http.ListenAndServe(config.PlatformWebhookAddr, mux); err != nil {
			log.Printf("webhook: %v", err)
		}
	}()
}

func (a *Actor) HandleMessage(_ actor.ActorContext, _ interface{}) {}
func (a *Actor) OnStop(_ actor.ActorContext)                       {}
