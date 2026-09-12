package pbregister

import (
	"encoding/hex"
	"testing"

	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/tree/codec"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestUnityRegisterReqWireCompatibility(t *testing.T) {
	// RegisterReq{Account:"unity-account", Password:"pw", ServerId:1}
	// serialized by Google.Protobuf C# using the shared IDL fields 1, 2 and 3.
	unityWire, err := hex.DecodeString("0a0d756e6974792d6163636f756e74120270771801")
	if err != nil {
		t.Fatal(err)
	}

	req := &pb_gateway.RegisterReq{}
	if err := proto.Unmarshal(unityWire, req); err != nil {
		t.Fatalf("unmarshal Unity RegisterReq: %v", err)
	}
	if req.Account != "unity-account" || req.Password != "pw" || req.ServerId != 1 {
		t.Fatalf("decoded RegisterReq = account:%q password:%q server:%d", req.Account, req.Password, req.ServerId)
	}
	if req.UID != 0 || req.ServerID != 0 || req.SessionID != "" {
		t.Fatalf("client unexpectedly supplied internal header: uid:%d server:%d session:%q", req.UID, req.ServerID, req.SessionID)
	}

	codec.WriteHeader(req, codec.ReqHeader{UID: 99, ServerID: 7, SessionID: "session"})
	if req.UID != 99 || req.ServerID != 7 || req.SessionID != "session" {
		t.Fatalf("server header was not written: uid:%d server:%d session:%q", req.UID, req.ServerID, req.SessionID)
	}
	if req.Account != "unity-account" || req.Password != "pw" || req.ServerId != 1 {
		t.Fatalf("writing header changed business fields: account:%q password:%q server:%d", req.Account, req.Password, req.ServerId)
	}

	fields := req.ProtoReflect().Descriptor().Fields()
	wantNumbers := map[string]int32{
		"account": 1, "password": 2, "server_id": 3,
		"uID": 101, "serverID": 102, "sessionID": 103,
	}
	for name, want := range wantNumbers {
		field := fields.ByName(protoreflectName(name))
		if field == nil || int32(field.Number()) != want {
			if field == nil {
				t.Fatalf("field %q is missing", name)
			}
			t.Fatalf("field %q number = %d, want %d", name, field.Number(), want)
		}
	}
}

// protoreflect.Name is an alias-like defined string type; keep conversion local
// so the compatibility table above remains easy to read.
func protoreflectName(name string) protoreflect.Name { return protoreflect.Name(name) }
