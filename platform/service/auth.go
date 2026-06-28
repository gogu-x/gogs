package service

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"

	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoPlatform"
	"github.com/gogu-x/gogs/platform/auth"
	"github.com/gogu-x/gogs/platform/store"
)

const dbTimeout = 5 * time.Second

func bg() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), dbTimeout)
}

func Register(db *mongo.Database, req *protoPlatform.RegisterReq) (*protoPlatform.AuthAck, error) {
	if req.ServerId == 0 {
		return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_ERR_PARAM, Msg: "server_id required"}, nil
	}
	ctx, cancel := bg()
	defer cancel()
	acc := &store.Account{}
	err := db.Collection(store.ColAccounts).FindOne(ctx, bson.M{"account": req.Account, "server_id": req.ServerId}).Decode(acc)
	if err == nil {
		return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_ERR_USERNAME_EXISTS, Msg: "account already exists"}, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_ERR_UNKNOWN, Msg: err.Error()}, nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.MinCost)
	if err != nil {
		return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_ERR_INTERNAL, Msg: "internal error"}, nil
	}
	uid := store.NextUID()
	newAcc := &store.Account{
		ID: bson.NewObjectID(), Account: req.Account,
		PasswordHash: string(hash), UID: uid, CreatedAt: time.Now(),
		ServerId: req.ServerId,
	}
	ctx2, cancel2 := bg()
	defer cancel2()
	if _, err := db.Collection(store.ColAccounts).InsertOne(ctx2, newAcc); err != nil {
		return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_ERR_INTERNAL, Msg: err.Error()}, nil
	}
	token, err := auth.Sign(uid)
	if err != nil {
		return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_ERR_INTERNAL, Msg: "sign error"}, nil
	}
	return &protoPlatform.AuthAck{Code: protoCommon.ErrCode_OK, Uid: uid, Token: token}, nil
}

func Login(db *mongo.Database, req *protoPlatform.AuthLoginReq) (*protoPlatform.AuthAck, error) {
	ctx, cancel := bg()
	defer cancel()
	acc := &store.Account{}
	if err := db.Collection(store.ColAccounts).FindOne(ctx, bson.M{"account": req.Account, "server_id": req.ServerId}).Decode(acc); err != nil {
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

// EnsureIndexes 在启动时建立必要的索引。
func EnsureIndexes(db *mongo.Database) error {
	ctx, cancel := bg()
	defer cancel()
	_, err := db.Collection(store.ColAccounts).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "account", Value: 1}, {Key: "server_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return err
}
