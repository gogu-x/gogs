package service

import (
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/crypto/bcrypt"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoPlatform"
	"github.com/gogu-x/gogs/platform/auth"
	"github.com/gogu-x/gogs/platform/store"
	rpcmongo "github.com/gogu-x/gogs/rpc/mongo"
)

func dbCall(mongoPID actor.PID, msg interface{}) (interface{}, error) {
	return actor.Default().Request(mongoPID, msg).AwaitTimeout(5 * time.Second)
}

func Register(mongoPID actor.PID, req *protoPlatform.RegisterReq) (*protoPlatform.AuthAck, error) {
	if req.ServerId == 0 {
		return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_ERR_PARAM, Msg: "server_id required"}, nil
	}
	acc := &store.Account{}
	err := callFindOne(mongoPID, store.ColAccounts, bson.M{"account": req.Account, "server_id": req.ServerId}, acc)
	if err == nil {
		return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_ERR_USERNAME_EXISTS, Msg: "account already exists"}, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_ERR_UNKNOWN, Msg: err.Error()}, nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_ERR_INTERNAL, Msg: "internal error"}, nil
	}
	uid := store.NextUID()
	newAcc := &store.Account{
		ID: bson.NewObjectID(), Account: req.Account,
		PasswordHash: string(hash), UID: uid, CreatedAt: time.Now(),
		ServerId: req.ServerId,
	}
	if _, err := dbCall(mongoPID, &rpcmongo.InsertOne{Collection: store.ColAccounts, Doc: newAcc}); err != nil {
		return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_ERR_INTERNAL, Msg: err.Error()}, nil
	}
	token, err := auth.Sign(uid)
	if err != nil {
		return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_ERR_INTERNAL, Msg: "sign error"}, nil
	}
	return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_OK, Uid: uid, Token: token}, nil
}

func Login(mongoPID actor.PID, req *protoPlatform.AuthLoginReq) (*protoPlatform.AuthAck, error) {
	acc := &store.Account{}
	if err := callFindOne(mongoPID, store.ColAccounts, bson.M{"account": req.Account, "server_id": req.ServerId}, acc); err != nil {
		return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_ERR_UNKNOWN, Msg: "user not found"}, nil
	}
	if err := bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(req.Password)); err != nil {
		return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_ERR_WRONG_PASSWORD, Msg: "wrong password"}, nil
	}
	token, err := auth.Sign(acc.UID)
	if err != nil {
		return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_ERR_INTERNAL, Msg: "sign error"}, nil
	}
	return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_OK, Uid: acc.UID, Token: token}, nil
}

func VerifyToken(req *protoPlatform.VerifyTokenReq) (*protoPlatform.VerifyAck, error) {
	uid, err := auth.Verify(req.Token)
	if err != nil {
		return &protoPlatform.VerifyAck{Valid: false}, nil
	}
	return &protoPlatform.VerifyAck{Valid: true, Uid: uid}, nil
}

func callFindOne(mongoPID actor.PID, col string, filter, result interface{}) error {
	_, err := dbCall(mongoPID, &rpcmongo.FindOne{Collection: col, Filter: filter, Result: result})
	return err
}
