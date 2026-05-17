package codec

import "google.golang.org/protobuf/proto"

// Codec 编解码接口
type Codec interface {
	Unmarshal(data []byte) (interface{}, error)
	Marshal(msg interface{}) ([]byte, error)
}

// RegisterMsg 注册消息到两种 codec
func RegisterMsg(msgs ...proto.Message) {
	for _, msg := range msgs {
		ProtoCodec.register(msg)
		JsonCodec.register(msg)
	}
}
