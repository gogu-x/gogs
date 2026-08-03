package webhook

import (
	"log"
	"net/http"

	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/constant"
	platformgrpc "github.com/gogu-x/gogs/platform/grpc"
	"github.com/gogu-x/tree"
)

type Actor struct{}

func (a *Actor) Name() string { return constant.Web }

func (a *Actor) OnInit(_ tree.Context) {
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

func (a *Actor) HandleMessage(_ tree.Context, _ interface{}) {}
func (a *Actor) OnStop(_ tree.Context)                       {}
