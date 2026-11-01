package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gogu-x/gogs/pb/pfpb/pb_pf"
)

func TestPaymentResponseContainsOnlyNumericCode(t *testing.T) {
	for code, status := range map[pb_pf.ErrCode]int{
		pb_pf.ErrCode_OK:              http.StatusOK,
		pb_pf.ErrCode_PARAM:           http.StatusBadRequest,
		pb_pf.ErrCode_ORDER_NOT_FOUND: http.StatusNotFound,
		pb_pf.ErrCode_INTERNAL:        http.StatusInternalServerError,
	} {
		response := httptest.NewRecorder()
		writePaymentCode(response, code)
		var body map[string]interface{}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != status || len(body) != 1 || body["code"] != float64(code) {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
	}
}
