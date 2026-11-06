package conn

import (
	"os"
	"strconv"

	"errors"
	"github.com/gogu-x/tree/tlog"
	"testing"

	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/gogs/pb/pfpb/pb_pf"
)

type capturedAckCodec struct{ message interface{} }

func (c *capturedAckCodec) Marshal(message interface{}) ([]byte, error) {
	c.message = message
	return nil, errors.New("captured before socket write")
}
func (*capturedAckCodec) Unmarshal([]byte) (interface{}, error) { return nil, nil }

func TestPlatformFailuresPreserveCodesAndResetSession(t *testing.T) {
	for _, code := range []pb_pf.ErrCode{pb_pf.ErrCode_WRONG_PASSWORD, pb_pf.ErrCode_USERNAME_EXISTS, pb_pf.ErrCode_ACCOUNT_NOT_FOUND, pb_pf.ErrCode_PARAM, pb_pf.ErrCode_INTERNAL} {
		t.Run(strconv.FormatInt(int64(code), 10), func(t *testing.T) {
			capture := &capturedAckCodec{}
			c := &Conn{codec: capture, state: StateLoggIng}
			c.authLoginCb(nil, &pb_pf.AuthAck{Code: code}, nil)
			ack, ok := capture.message.(*pb_gateway.LoginAck)
			if !ok || ack.GetCode() != platformCode(code) || c.state != StateAnon {
				t.Fatalf("login ack=%v state=%v", capture.message, c.state)
			}
			c.state = StateRegIng
			c.regCb(nil, &pb_pf.AuthAck{Code: code}, nil)
			register, ok := capture.message.(*pb_gateway.RegisterAck)
			if !ok || register.GetCode() != platformCode(code) || c.state != StateAnon {
				t.Fatalf("register ack=%v state=%v", capture.message, c.state)
			}
			c.getServerListCb(nil, &pb_pf.ServerListAck{Code: code}, nil)
			list, ok := capture.message.(*pb_gateway.ServerListAck)
			if !ok || list.GetCode() != platformCode(code) {
				t.Fatalf("server list ack=%v", capture.message)
			}
		})
	}
}

func TestLoginTransportAndMalformedReplyReturnInternal(t *testing.T) {
	for _, value := range []interface{}{nil, (*pb_pf.AuthAck)(nil), "invalid response"} {
		capture := &capturedAckCodec{}
		c := &Conn{codec: capture, state: StateLoggIng}
		c.authLoginCb(nil, value, nil)
		if ack := capture.message.(*pb_gateway.LoginAck); ack.GetCode() != pb_common.ErrCode_INTERNAL || c.state != StateAnon {
			t.Fatalf("ack=%v state=%v", ack, c.state)
		}
	}
	if code := platformCode(pb_pf.ErrCode(123456)); code != pb_common.ErrCode_INTERNAL {
		t.Fatalf("unknown enum mapped to %v", code)
	}
	capture := &capturedAckCodec{}
	c := &Conn{codec: capture, state: StateLoggIng}
	c.authLoginCb(nil, nil, errors.New("private connection details"))
	if ack := capture.message.(*pb_gateway.LoginAck); ack.GetCode() != pb_common.ErrCode_INTERNAL {
		t.Fatal(ack)
	}
	c.LoginAck(nil, &pb_auth.LoginGameAck{Code: pb_common.ErrCode_UNAUTHORIZED})
	if ack := capture.message.(*pb_gateway.LoginAck); ack.GetCode() != pb_common.ErrCode_UNAUTHORIZED || c.state != StateAnon {
		t.Fatalf("game login ack=%v state=%v", ack, c.state)
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
