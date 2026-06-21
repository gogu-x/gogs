// Package protoPlatform provides a JSON codec for platform gRPC services.
// Platform service messages use JSON instead of protobuf binary encoding.
package protoPlatform

import (
	"encoding/json"

	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
)

// RegisterJSONCodec overrides the default proto codec with JSON for the platform server process.
func RegisterJSONCodec() {
	encoding.RegisterCodec(jsonCodec{})
}

// ForceJSONCodec returns a DialOption that forces JSON encoding on a single gRPC connection.
func ForceJSONCodec() grpc.DialOption {
	return grpc.WithDefaultCallOptions(grpc.ForceCodec(jsonCodec{}))
}

type jsonCodec struct{}

func (jsonCodec) Name() string                        { return "proto" }
func (jsonCodec) Marshal(v interface{}) ([]byte, error) { return json.Marshal(v) }
func (jsonCodec) Unmarshal(data []byte, v interface{}) error { return json.Unmarshal(data, v) }
