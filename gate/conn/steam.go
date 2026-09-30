package conn

import (
	"context"
	"io"
	"log"

	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type streamClosed struct{ err error }

// OpenSteam opens the per-WebSocket bidirectional stream to the selected Game
// server. The only gRPC receive goroutine sends replies back to the Conn actor
// mailbox, which remains the sole WebSocket writer.
func (c *Conn) OpenSteam(tctx tree.Context, msg *pb_auth.LoginGameReq) error {
	pid, ok := tree.Lookup(constant.ActorGateServer)
	if !ok {
		return status.Errorf(codes.NotFound, "no actor")
	}
	req := &constant.GetServerGrpcClientReq{
		ServerId: c.serverID,
	}
	tree.RequestCallback(pid, req, tctx.Self(), func(ctx tree.Context, i interface{}, err error) {
		if err != nil {
			return
		}
		ack := i.(*constant.GetServerGrpcClientAck)
		stream, err := pb_gateway.NewGatewayClient(ack.Grpc).Stream(context.Background())

		c.stream = stream

		c.sendGameSteam(ctx, msg)
		tlog.Log.Info("ConnActor[%d]: uid=%d login forwarded -> server=%d node=%s", c.connID, c.uid, c.serverID, c.nodeID)

		go c.receiveStream(tctx.Self(), stream)
	})

	return nil
}

func (c *Conn) receiveStream(self tree.PID, stream pb_gateway.Gateway_StreamClient) {
	for {
		frame, err := stream.Recv()
		if err != nil {
			if err != io.EOF && status.Code(err) != codes.Canceled {
				log.Printf("ConnActor[%d]: game stream receive error: %v", c.connID, err)
			}
			tree.Send(self, &streamClosed{err: err})
			return
		}
		if !tree.Send(self, frame) {
			return
		}
	}
}
