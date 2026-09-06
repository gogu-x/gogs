package service

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"

	"github.com/gogu-x/gogs/pb/pfpb/pb_pf"
	"github.com/gogu-x/gogs/platform/auth"
	"github.com/gogu-x/gogs/platform/store"
)

const dbTimeout = 5 * time.Second

func bg() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), dbTimeout)
}

// Register 注册账号
func Register(db *mongo.Database, req *pb_pf.RegisterReq) (*pb_pf.AuthAck, error) {
	if req.ServerId == 0 {
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_PARAM, Msg: "server_id required"}, nil
	}
	ctx, cancel := bg()
	defer cancel()
	acc := &store.Account{}
	err := db.Collection(store.ColAccounts).FindOne(ctx, bson.M{"account": req.Account, "server_id": req.ServerId}).Decode(acc)
	if err == nil {
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_USERNAME_EXISTS, Msg: "account already exists"}, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_UNKNOWN, Msg: err.Error()}, nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.MinCost)
	if err != nil {
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_INTERNAL, Msg: "internal error"}, nil
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
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_INTERNAL, Msg: err.Error()}, nil
	}
	token, err := auth.Sign(uid)
	if err != nil {
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_INTERNAL, Msg: "sign error"}, nil
	}
	return &pb_pf.AuthAck{Code: pb_pf.ErrCode_OK, Uid: uid, Token: token}, nil
}

// Login 登录账号
func Login(db *mongo.Database, req *pb_pf.AuthLoginReq) (*pb_pf.AuthAck, error) {
	ctx, cancel := bg()
	defer cancel()
	acc := &store.Account{}
	if err := db.Collection(store.ColAccounts).FindOne(ctx, bson.M{"account": req.Account, "server_id": req.ServerId}).Decode(acc); err != nil {
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_UNKNOWN, Msg: "user not found"}, nil
	}
	if err := bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(req.Password)); err != nil {
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_WRONG_PASSWORD, Msg: "wrong password"}, nil
	}
	token, err := auth.Sign(acc.UID)
	if err != nil {
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_INTERNAL, Msg: "sign error"}, nil
	}
	return &pb_pf.AuthAck{Code: pb_pf.ErrCode_OK, Uid: acc.UID, Token: token, ServerId: acc.ServerId}, nil
}

// GetServerList returns every server registration for an account before login.
func GetServerList(db *mongo.Database, req *pb_pf.GetServerListReq) (*pb_pf.ServerListAck, error) {
	if req == nil || req.Account == "" {
		return &pb_pf.ServerListAck{Code: pb_pf.ErrCode_PARAM, Msg: "account required"}, nil
	}

	ctx, cancel := bg()
	defer cancel()
	cursor, err := db.Collection(store.ColAccounts).Find(
		ctx,
		bson.M{"account": req.Account},
		options.Find().SetProjection(bson.M{"server_id": 1, "uid": 1}).SetSort(bson.D{{Key: "server_id", Value: 1}}),
	)
	if err != nil {
		return &pb_pf.ServerListAck{Code: pb_pf.ErrCode_INTERNAL, Msg: err.Error()}, nil
	}
	defer cursor.Close(ctx)

	accounts := make([]*pb_pf.ServerAccount, 0)
	for cursor.Next(ctx) {
		var account store.Account
		if err := cursor.Decode(&account); err != nil {
			return &pb_pf.ServerListAck{Code: pb_pf.ErrCode_INTERNAL, Msg: err.Error()}, nil
		}
		accounts = append(accounts, &pb_pf.ServerAccount{ServerId: account.ServerId, Uid: account.UID})
	}
	if err := cursor.Err(); err != nil {
		return &pb_pf.ServerListAck{Code: pb_pf.ErrCode_INTERNAL, Msg: err.Error()}, nil
	}
	return &pb_pf.ServerListAck{Code: pb_pf.ErrCode_OK, Accounts: accounts}, nil
}

func VerifyToken(req *pb_pf.VerifyTokenReq) (*pb_pf.VerifyAck, error) {
	uid, err := auth.Verify(req.Token)
	if err != nil {
		return &pb_pf.VerifyAck{Valid: false}, nil
	}
	return &pb_pf.VerifyAck{Valid: true, Uid: uid}, nil
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
