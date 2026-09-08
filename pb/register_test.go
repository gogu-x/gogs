package pbregister

import (
	"testing"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree/codec"
	"google.golang.org/protobuf/proto"
)

func TestBattleLifecycleMessagesAreRegistered(t *testing.T) {
	messages := []proto.Message{
		&pb.BattleCreatedNtf{BattleId: "b"},
		&pb.BattleActionNtf{BattleId: "b", Event: &pb.BattleEvent{Sequence: 1}},
		&pb.BattleFinishedNtf{BattleId: "b", Checksum: "sum"},
		&pb.BattleResultConfirmedNtf{BattleId: "b"},
	}
	for _, message := range messages {
		data, err := codec.ProtoCodec.Marshal(message)
		if err != nil {
			t.Fatalf("marshal %T: %v", message, err)
		}
		decoded, err := codec.ProtoCodec.Unmarshal(data)
		if err != nil {
			t.Fatalf("unmarshal %T: %v", message, err)
		}
		if proto.MessageName(decoded.(proto.Message)) != proto.MessageName(message) {
			t.Fatalf("decoded %T, want %T", decoded, message)
		}
	}
}
