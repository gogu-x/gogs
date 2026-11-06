package service

import (
	"os"

	"github.com/gogu-x/tree/tlog"
	"testing"

	"github.com/gogu-x/gogs/pb/pfpb/pb_pf"
	"github.com/gogu-x/gogs/platform/auth"
)

func TestInvalidRequestsReturnCodesBeforeDatabaseAccess(t *testing.T) {
	register, err := Register(nil, nil)
	if err != nil || register.GetCode() != pb_pf.ErrCode_PARAM {
		t.Fatalf("register=%v err=%v", register, err)
	}
	login, err := Login(nil, &pb_pf.AuthLoginReq{})
	if err != nil || login.GetCode() != pb_pf.ErrCode_PARAM {
		t.Fatalf("login=%v err=%v", login, err)
	}
	servers, err := GetServerList(nil, nil)
	if err != nil || servers.GetCode() != pb_pf.ErrCode_PARAM {
		t.Fatalf("servers=%v err=%v", servers, err)
	}
	order, err := CreateOrder(nil, nil)
	if err != nil || order.GetCode() != pb_pf.ErrCode_PARAM {
		t.Fatalf("order=%v err=%v", order, err)
	}
	detail, err := QueryOrder(nil, &pb_pf.QueryOrderReq{})
	if err != nil || detail.GetCode() != pb_pf.ErrCode_PARAM {
		t.Fatalf("detail=%v err=%v", detail, err)
	}
	if code := DeliverOrder(nil, ""); code != pb_pf.ErrCode_PARAM {
		t.Fatal(code)
	}
}

func TestVerifyTokenExplicitOutcomes(t *testing.T) {
	missing, err := VerifyToken(nil)
	if err != nil || missing.GetCode() != pb_pf.ErrCode_PARAM || missing.GetValid() {
		t.Fatalf("missing=%v err=%v", missing, err)
	}
	invalid, err := VerifyToken(&pb_pf.VerifyTokenReq{Token: "invalid"})
	if err != nil || invalid.GetCode() != pb_pf.ErrCode_INVALID_TOKEN || invalid.GetValid() {
		t.Fatalf("invalid=%v err=%v", invalid, err)
	}
	token, err := auth.Sign(42)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := VerifyToken(&pb_pf.VerifyTokenReq{Token: token})
	if err != nil || valid.GetCode() != pb_pf.ErrCode_OK || !valid.GetValid() || valid.GetUid() != 42 {
		t.Fatalf("valid=%v err=%v", valid, err)
	}
}

func TestMain(m *testing.M) {
	logger, err := tlog.New("error", "", tlog.NoFlags)
	if err != nil {
		panic(err)
	}
	tlog.Log = logger
	os.Exit(m.Run())
}
