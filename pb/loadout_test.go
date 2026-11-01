package pbregister

import (
	"github.com/gogu-x/gogs/pb/cspb/pb_souljade"
	"github.com/gogu-x/gogs/pb/cspb/pb_talent"
	"github.com/gogu-x/tree/codec"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestLoadoutMessagesAreRegisteredAndPreserveRequestIDs(t *testing.T) {
	messages := []proto.Message{
		&pb_souljade.GetRoleSoulJadeReq{RequestId: 9007199254740993},
		&pb_souljade.GetRoleSoulJadeAck{RequestId: 9007199254740993},
		&pb_souljade.ChangeRoleSoulJadeReq{RequestId: 2, ExpectedRevision: 1, Slot: 6, InstanceId: "jade-1"},
		&pb_souljade.ChangeRoleSoulJadeAck{RequestId: 2},
		&pb_souljade.UpgradeRoleSoulJadeReq{RequestId: 3, ExpectedRevision: 2, InstanceId: "jade-1"},
		&pb_souljade.UpgradeRoleSoulJadeAck{RequestId: 3},
		&pb_souljade.RefineRoleSoulJadeReq{RequestId: 4, ExpectedRevision: 3, InstanceId: "jade-1"},
		&pb_souljade.RefineRoleSoulJadeAck{RequestId: 4},
		&pb_talent.GetRoleTalentReq{RequestId: 5},
		&pb_talent.GetRoleTalentAck{RequestId: 5},
		&pb_talent.LearnRoleTalentReq{RequestId: 6, ExpectedRevision: 1, NodeId: 25},
		&pb_talent.LearnRoleTalentAck{RequestId: 6},
		&pb_talent.ResetRoleTalentReq{RequestId: 7, ExpectedRevision: 2, Branch: -1},
		&pb_talent.ResetRoleTalentAck{RequestId: 7},
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
			t.Fatalf("protocol registration or roundtrip failed: %T", message)
		}
	}
}

func TestLoadoutRequestHeadersRemainServerControlled(t *testing.T) {
	req := &pb_souljade.UpgradeRoleSoulJadeReq{RequestId: 123, UID: 999, ServerID: 99, SessionID: "forged"}
	codec.WriteHeader(req, codec.ReqHeader{UID: 42, ServerID: 1, SessionID: "authenticated"})
	if req.GetUID() != 42 || req.GetServerID() != 1 || req.GetSessionID() != "authenticated" || req.GetRequestId() != 123 {
		t.Fatal("server header did not override forged identity")
	}
}
