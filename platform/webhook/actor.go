package webhook

import (
	"encoding/json"
	"net/http"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/pb/pfpb/pb_pf"
	platformgrpc "github.com/gogu-x/gogs/platform/grpc"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

type Actor struct{}

func (a *Actor) Name() string { return def.Web }

func (a *Actor) OnInit(_ tree.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/pay", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Verify the payment provider signature.
		orderID := r.URL.Query().Get("order_id")
		if orderID == "" {
			writePaymentCode(w, pb_pf.ErrCode_PARAM)
			return
		}
		writePaymentCode(w, platformgrpc.DeliverByOrderID(orderID))
	})
	go func() {
		tlog.Info("[webhook/Actor.OnInit] 启动HTTP回调服务, addr=%s", conf.PlatformWebhookAddr)
		if err := http.ListenAndServe(conf.PlatformWebhookAddr, mux); err != nil {
			tlog.Error("[webhook/Actor.OnInit] HTTP回调服务退出, err=%v", err)
		}
	}()
}

func writePaymentCode(w http.ResponseWriter, code pb_pf.ErrCode) {
	status := http.StatusOK
	switch code {
	case pb_pf.ErrCode_OK:
	case pb_pf.ErrCode_PARAM:
		status = http.StatusBadRequest
	case pb_pf.ErrCode_ORDER_NOT_FOUND:
		status = http.StatusNotFound
	default:
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(struct {
		Code int32 `json:"code"`
	}{int32(code)}); err != nil {
		tlog.Error("[webhook/writePaymentCode] 回调响应写入失败, err=%v", err)
	}
}

func (a *Actor) HandleMessage(_ tree.Context, _ interface{}) {}
func (a *Actor) OnStop(_ tree.Context)                       {}
