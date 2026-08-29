package conn

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/cluster"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type streamClosed struct{ err error }

// OpenSteam opens the per-WebSocket bidirectional stream to the selected Game
// server. The only gRPC receive goroutine sends replies back to the Conn actor
// mailbox, which remains the sole WebSocket writer.
func (c *Conn) OpenSteam(self tree.PID) error {
	addr, err := cluster.GetAddr(c.serverID)
	if err != nil {
		return fmt.Errorf("get game address for server %d: %w", c.serverID, err)
	}
	grpcConn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial game %s: %w", addr, err)
	}
	stream, err := protoGateway.NewGatewayClient(grpcConn).Stream(context.Background())
	if err != nil {
		_ = grpcConn.Close()
		return fmt.Errorf("open game stream: %w", err)
	}

	c.grpcConn = grpcConn
	c.stream = stream
	go c.receiveStream(self, stream)
	return nil
}

func (c *Conn) receiveStream(self tree.PID, stream protoGateway.Gateway_StreamClient) {
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
