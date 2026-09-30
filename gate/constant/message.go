package constant

import "google.golang.org/grpc"

type GetServerGrpcClientReq struct {
	ServerId uint32
}

type GetServerGrpcClientAck struct {
	Grpc *grpc.ClientConn
}
