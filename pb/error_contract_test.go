package pbregister

import (
	"strings"
	"testing"

	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_equipment"
	"github.com/gogu-x/gogs/pb/cspb/pb_souljade"
	"github.com/gogu-x/gogs/pb/cspb/pb_talent"
	"github.com/gogu-x/gogs/pb/pfpb/pb_pf"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func TestPublicResponsesUseErrCodeWithoutErrorText(t *testing.T) {
	checked := 0
	protoregistry.GlobalFiles.RangeFiles(func(file protoreflect.FileDescriptor) bool {
		if !strings.HasPrefix(file.Path(), "cspb/") && !strings.HasPrefix(file.Path(), "pfpb/") {
			return true
		}
		wantKind := protoreflect.Int32Kind
		if strings.HasPrefix(file.Path(), "cspb/") {
			wantKind = protoreflect.EnumKind
		}
		messages := file.Messages()
		for i := 0; i < messages.Len(); i++ {
			message := messages.Get(i)
			name := string(message.Name())
			if !strings.HasSuffix(name, "Ack") && name != "OrderDetail" {
				continue
			}
			checked++
			fields := message.Fields()
			var status protoreflect.FieldDescriptor
			for j := 0; j < fields.Len(); j++ {
				field := fields.Get(j)
				switch strings.ToLower(string(field.Name())) {
				case "error",
					"msg",
					"message":
					t.Errorf("%s still exposes error text field %s", message.FullName(), field.Name())
				case "code",
					"errcode",
					"err_code":
					status = field
				}
			}
			if status == nil || status.Kind() != wantKind {
				t.Errorf("%s must expose a %s error code", message.FullName(), wantKind)
			}
		}
		return true
	})
	if checked < 20 {
		t.Fatalf("expected public response descriptors, inspected only %d", checked)
	}
}

func TestStatusFieldsPreserveNumericCodes(t *testing.T) {
	messages := []proto.Message{
		&pb_common.GetRoleCombatStatsAck{},
		&pb_equipment.GetRoleEquipmentAck{},
		&pb_equipment.ChangeRoleEquipmentAck{},
		&pb_souljade.GetRoleSoulJadeAck{},
		&pb_souljade.ChangeRoleSoulJadeAck{},
		&pb_souljade.UpgradeRoleSoulJadeAck{},
		&pb_souljade.RefineRoleSoulJadeAck{},
		&pb_talent.GetRoleTalentAck{},
		&pb_talent.LearnRoleTalentAck{},
		&pb_talent.ResetRoleTalentAck{},
		&pb_battle.StartBattleAck{},
		&pb_actv.GetActvListAck{},
	}
	for _, message := range messages {
		reflection := message.ProtoReflect()
		field := reflection.Descriptor().Fields().ByName("code")
		if field == nil || field.HasPresence() {
			t.Fatalf("%T must use a non-optional status", message)
		}
		for _, code := range []pb_common.ErrCode{pb_common.ErrCode_OK, pb_common.ErrCode_STALE_REVISION, pb_common.ErrCode(-987654)} {
			reflection.Set(field, protoreflect.ValueOfEnum(protoreflect.EnumNumber(code)))
			encoded, err := proto.Marshal(message)
			if err != nil {
				t.Fatal(err)
			}
			decoded := reflection.New().Interface()
			if err := proto.Unmarshal(encoded, decoded); err != nil {
				t.Fatal(err)
			}
			result := decoded.ProtoReflect()
			if result.Get(field).Enum() != protoreflect.EnumNumber(code) {
				t.Fatalf("%T lost explicit status %d during serialization", message, code)
			}
		}
	}
}

func TestPlatformStatusFieldsPreserveNumericCodes(t *testing.T) {
	messages := []proto.Message{
		&pb_pf.VerifyAck{},
		&pb_pf.OrderDetail{},
	}
	for _, message := range messages {
		reflection := message.ProtoReflect()
		field := reflection.Descriptor().Fields().ByName("code")
		if field == nil || field.HasPresence() {
			t.Fatalf("%T must use a non-optional status", message)
		}
		for _, code := range []int32{0, int32(pb_pf.ErrCode_PARAM), -987654} {
			reflection.Set(field, protoreflect.ValueOfInt32(code))
			encoded, err := proto.Marshal(message)
			if err != nil {
				t.Fatal(err)
			}
			decoded := reflection.New().Interface()
			if err := proto.Unmarshal(encoded, decoded); err != nil {
				t.Fatal(err)
			}
			result := decoded.ProtoReflect()
			if result.Get(field).Int() != int64(code) {
				t.Fatalf("%T lost explicit status %d during serialization", message, code)
			}
		}
	}
}
