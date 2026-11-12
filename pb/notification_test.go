package pbregister

import (
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree/codec"
	"google.golang.org/protobuf/proto"
	"testing"
)

// TestErrorNotificationWire 验证统一错误通知已注册且请求类型能够往返编解码。
func TestErrorNotificationWire(t *testing.T) {
	notification := ipb.NewErrorNotification(&pb_actv.GetActvListReq{}, pb_common.ErrCode_ACTV_UNAVAILABLE)
	encoded, err := codec.ProtoCodec.Marshal(notification)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := codec.ProtoCodec.Unmarshal(encoded)
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := decoded.(*pb_common.ErrorNtf)
	if !ok || !proto.Equal(notification, actual) || actual.GetRequestType() != "GetActvListReq" {
		t.Fatal("统一错误通知协议往返失败")
	}
}
