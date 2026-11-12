package ipb

import (
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"google.golang.org/protobuf/proto"
)

// NewErrorNotification 构造通用错误通知，关联原请求类型，不暴露内部错误文本。
func NewErrorNotification(request proto.Message, code pb_common.ErrCode) *pb_common.ErrorNtf {
	return &pb_common.ErrorNtf{Code: code, RequestType: string(proto.MessageName(request))}
}
