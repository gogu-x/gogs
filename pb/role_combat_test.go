package pbregister

import (
	"testing"

	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree/codec"
	"google.golang.org/protobuf/proto"
)

func TestRoleCombatMessagesRoundTripWithAuthoritativeHeaders(t *testing.T) {
	request := &pb_common.GetRoleCombatStatsReq{RequestId: 9007199254740993}
	codec.WriteHeader(request, codec.ReqHeader{UID: 42, ServerID: 1, SessionID: "authenticated"})
	if request.GetUID() != 42 || request.GetServerID() != 1 || request.GetSessionID() != "authenticated" {
		t.Fatal("request is missing authoritative routing headers")
	}
	messages := []proto.Message{
		request,
		&pb_common.GetRoleCombatStatsAck{
			RequestId: request.GetRequestId(),
			Data: &pb_common.RoleCombatStatsData{
				RoleId: "role-42", MaxHp: 9007199254740993,
				Attack: 1234, Defense: 5678, Speed: 9012,
			},
			Code: pb_common.ErrCode_OK.Enum(),
		},
	}
	for _, message := range messages {
		encoded, err := codec.ProtoCodec.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := codec.ProtoCodec.Unmarshal(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(message, decoded.(proto.Message)) {
			t.Fatalf("registration or integer roundtrip failed: %T", message)
		}
	}
}
