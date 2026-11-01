package ctl_battle

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gogu-x/gogs/game/battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
)

func TestBattleStartAckUsesDeclaredCodes(t *testing.T) {
	for _, test := range []struct {
		name  string
		value interface{}
		err   error
		code  pb_common.ErrCode
	}{
		{"success", battle.State{BattleID: "current"}, nil, pb_common.ErrCode_OK},
		{"deadline", battle.State{}, fmt.Errorf("wrapped: %w", context.DeadlineExceeded), pb_common.ErrCode_BATTLE_TIMEOUT},
		{"actor timeout", nil, tree.ErrTimeout, pb_common.ErrCode_BATTLE_TIMEOUT},
		{"unavailable", nil, battle.ErrUnavailable, pb_common.ErrCode_BATTLE_UNAVAILABLE},
		{"invalid request", nil, battle.ErrInvalidRequest, pb_common.ErrCode_INVALID_BATTLE_REQUEST},
		{"active", battle.State{BattleID: "active"}, fmt.Errorf("wrapped: %w", battle.ErrInProgress), pb_common.ErrCode_BATTLE_IN_PROGRESS},
		{"failure", nil, errors.New("private backend details"), pb_common.ErrCode_BATTLE_CREATE_FAILED},
		{"invalid response", nil, nil, pb_common.ErrCode_BATTLE_CREATE_FAILED},
	} {
		t.Run(test.name, func(t *testing.T) {
			ack := battleStartAck(test.value, test.err)
			if ack.Code == nil || ack.GetCode() != test.code {
				t.Fatalf("ack=%v want code=%v", ack, test.code)
			}
		})
	}
}
