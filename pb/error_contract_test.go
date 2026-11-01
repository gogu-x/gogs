package pbregister

import (
	"strings"
	"testing"

	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_equipment"
	"github.com/gogu-x/gogs/pb/cspb/pb_souljade"
	"github.com/gogu-x/gogs/pb/cspb/pb_talent"
	"github.com/gogu-x/gogs/pb/pfpb/pb_pf"
	"google.golang.org/protobuf/encoding/protowire"
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
			if status == nil || status.Kind() != protoreflect.EnumKind || status.Enum().Name() != "ErrCode" {
				t.Errorf("%s must expose the predefined ErrCode enum", message.FullName())
			}
		}
		return true
	})
	if checked < 20 {
		t.Fatalf("expected public response descriptors, inspected only %d", checked)
	}
}

func TestNewStatusFieldsPreservePresenceAndNumericCodes(t *testing.T) {
	messages := []proto.Message{
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
		&pb_activity.GetActivityListAck{},
		&pb_pf.VerifyAck{},
		&pb_pf.OrderDetail{},
	}
	for _, message := range messages {
		reflection := message.ProtoReflect()
		field := reflection.Descriptor().Fields().ByName("code")
		if field == nil || !field.HasPresence() || reflection.Has(field) {
			t.Fatalf("%T must distinguish missing status from explicit success", message)
		}
		for _, code := range []protoreflect.EnumNumber{0, protoreflect.EnumNumber(pb_common.ErrCode_STALE_REVISION), -987654} {
			reflection.Set(field, protoreflect.ValueOfEnum(code))
			encoded, err := proto.Marshal(message)
			if err != nil {
				t.Fatal(err)
			}
			decoded := reflection.New().Interface()
			if err := proto.Unmarshal(encoded, decoded); err != nil {
				t.Fatal(err)
			}
			result := decoded.ProtoReflect()
			if !result.Has(field) || result.Get(field).Enum() != code {
				t.Fatalf("%T lost explicit status %d during serialization", message, code)
			}
		}
	}
}

func TestLegacyStringErrorCannotBecomeExplicitSuccess(t *testing.T) {
	cases := []struct {
		message proto.Message
		oldTag  protowire.Number
		newTag  protoreflect.FieldNumber
	}{
		{&pb_equipment.GetRoleEquipmentAck{}, 2, 4},
		{&pb_equipment.ChangeRoleEquipmentAck{}, 2, 4},
		{&pb_souljade.GetRoleSoulJadeAck{}, 3, 4},
		{&pb_souljade.ChangeRoleSoulJadeAck{}, 3, 4},
		{&pb_souljade.UpgradeRoleSoulJadeAck{}, 3, 4},
		{&pb_souljade.RefineRoleSoulJadeAck{}, 3, 4},
		{&pb_talent.GetRoleTalentAck{}, 3, 4},
		{&pb_talent.LearnRoleTalentAck{}, 3, 4},
		{&pb_talent.ResetRoleTalentAck{}, 3, 4},
		{&pb_battle.StartBattleAck{}, 2, 3},
	}
	for _, item := range cases {
		reflection := item.message.ProtoReflect()
		descriptor := reflection.Descriptor()
		field := descriptor.Fields().ByName("code")
		if field.Number() != item.newTag {
			t.Fatalf("%T changed status tag: %d", item.message, field.Number())
		}
		if !descriptor.ReservedRanges().Has(protoreflect.FieldNumber(item.oldTag)) || !descriptor.ReservedNames().Has("error") {
			t.Fatalf("%T must reserve the legacy error field number and name", item.message)
		}
		legacy := protowire.AppendTag(nil, item.oldTag, protowire.BytesType)
		legacy = protowire.AppendString(legacy, "LOADOUT_UNAVAILABLE")
		if err := proto.Unmarshal(legacy, item.message); err != nil {
			t.Fatal(err)
		}
		if reflection.Has(field) {
			t.Fatalf("%T legacy response must remain distinguishable from explicit OK", item.message)
		}
	}
}
