package conn

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/pb/pb_gateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gogu-x/tree/comm"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// sendGameSteam forwards a client protobuf message through a payload-only Frame.
func (c *Conn) sendGameSteam(ctx tree.Context, inner interface{}) {
	protoMsg, ok := inner.(proto.Message)
	if !ok {
		def.DLog.Debug("ConnActor[%d]: unsupported game message %T", c.connID, inner)
		return
	}
	codec.WriteHeader(protoMsg, codec.ReqHeader{
		UID:       c.uid,
		ServerID:  c.serverID,
		SessionID: comm.NewUUID(),
	})
	body, err := codec.ProtoCodec.Marshal(protoMsg)
	if err != nil {
		def.DLog.Debug("ConnActor[%d]: marshal game message: %v", c.connID, err)
		return
	}
	if c.stream == nil {
		def.DLog.Debug("ConnActor[%d]: game stream is unavailable", c.connID)
		return
	}
	if err := c.stream.Send(&pb_gateway.Frame{Payload: body}); err != nil {
		def.DLog.Debug("ConnActor[%d]: send stream error: %v", c.connID, err)
		ctx.Stop()
	}
}

func (c *Conn) WriteWsMsg(msg proto.Message) {
	def.DLog.Info("", msg)
	data, err := c.codec.Marshal(msg)
	if err != nil {
		return
	}
	_ = c.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *Conn) onNodeFailover(_ tree.Context, _ interface{}) {}
