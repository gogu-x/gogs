package service

import (
	"context"
	"errors"
	"time"

	"github.com/gogu-x/tree/tlog"
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

// Register creates an account.
func Register(db *mongo.Database, req *pb_pf.RegisterReq) (*pb_pf.AuthAck, error) {
	if req == nil {
		tlog.Log.Warn("Register rejected: nil request")
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_PARAM}, nil
	}
	if req.ServerId == 0 || req.Account == "" || req.Password == "" {
		tlog.Log.Warn("Register rejected: server_id required")
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_PARAM}, nil
	}

	ctx, cancel := bg()
	defer cancel()
	acc := &store.Account{}
	err := db.Collection(store.ColAccounts).FindOne(ctx, bson.M{"account": req.Account, "server_id": req.ServerId}).Decode(acc)
	if err == nil {
		tlog.Log.Warn("Register rejected: account already exists, server_id=%d", req.ServerId)
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_USERNAME_EXISTS}, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		tlog.Log.Error("Register account lookup failed: server_id=%d err=%v", req.ServerId, err)
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_INTERNAL}, nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.MinCost)
	if err != nil {
		tlog.Log.Error("Register password hash failed: server_id=%d err=%v", req.ServerId, err)
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_INTERNAL}, nil
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
		tlog.Log.Error("Register account creation failed: server_id=%d uid=%d err=%v", req.ServerId, uid, err)
		if mongo.IsDuplicateKeyError(err) {
			return &pb_pf.AuthAck{Code: pb_pf.ErrCode_USERNAME_EXISTS}, nil
		}
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_INTERNAL}, nil
	}

	token, err := auth.Sign(uid)
	if err != nil {
		tlog.Log.Error("Register token signing failed: server_id=%d uid=%d err=%v", req.ServerId, uid, err)
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_INTERNAL}, nil
	}
	tlog.Log.Info("Register succeeded: server_id=%d uid=%d", req.ServerId, uid)
	return &pb_pf.AuthAck{Code: pb_pf.ErrCode_OK, Uid: uid, Token: token, ServerId: req.GetServerId()}, nil
}

// Login verifies account credentials.
func Login(db *mongo.Database, req *pb_pf.AuthLoginReq) (*pb_pf.AuthAck, error) {
	if req == nil || req.GetServerId() == 0 || req.GetAccount() == "" || req.GetPassword() == "" {
		tlog.Log.Warn("Login rejected: nil request")
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_PARAM}, nil
	}

	ctx, cancel := bg()
	defer cancel()
	acc := &store.Account{}
	if err := db.Collection(store.ColAccounts).FindOne(ctx, bson.M{"account": req.Account, "server_id": req.ServerId}).Decode(acc); err != nil {
		tlog.Log.Error("Login account lookup failed: server_id=%d err=%v", req.ServerId, err)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return &pb_pf.AuthAck{Code: pb_pf.ErrCode_ACCOUNT_NOT_FOUND}, nil
		}
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_INTERNAL}, nil
	}
	if err := bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(req.Password)); err != nil {
		if !errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			tlog.Log.Error("Login password verification failed: server_id=%d uid=%d err=%v", req.ServerId, acc.UID, err)
			return &pb_pf.AuthAck{Code: pb_pf.ErrCode_INTERNAL}, nil
		}
		tlog.Log.Warn("Login rejected: invalid password, server_id=%d uid=%d", req.ServerId, acc.UID)
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_WRONG_PASSWORD}, nil
	}

	token, err := auth.Sign(acc.UID)
	if err != nil {
		tlog.Log.Error("Login token signing failed: server_id=%d uid=%d err=%v", req.ServerId, acc.UID, err)
		return &pb_pf.AuthAck{Code: pb_pf.ErrCode_INTERNAL}, nil
	}
	tlog.Log.Info("Login succeeded: server_id=%d uid=%d", acc.ServerId, acc.UID)
	return &pb_pf.AuthAck{Code: pb_pf.ErrCode_OK, Uid: acc.UID, Token: token, ServerId: acc.ServerId}, nil
}

// GetServerList returns every server registration for an account before login.
func GetServerList(db *mongo.Database, req *pb_pf.GetServerListReq) (*pb_pf.ServerListAck, error) {
	if req == nil || req.Account == "" {
		tlog.Log.Warn("GetServerList rejected: account required")
		return &pb_pf.ServerListAck{Code: pb_pf.ErrCode_PARAM}, nil
	}

	ctx, cancel := bg()
	defer cancel()
	cursor, err := db.Collection(store.ColAccounts).Find(
		ctx,
		bson.M{"account": req.Account},
		options.Find().SetProjection(bson.M{"server_id": 1, "uid": 1}).SetSort(bson.D{{Key: "server_id", Value: 1}}),
	)
	if err != nil {
		tlog.Log.Error("GetServerList lookup failed: err=%v", err)
		return &pb_pf.ServerListAck{Code: pb_pf.ErrCode_INTERNAL}, nil
	}
	defer cursor.Close(ctx)

	accounts := make([]*pb_pf.ServerAccount, 0)
	for cursor.Next(ctx) {
		var account store.Account
		if err := cursor.Decode(&account); err != nil {
			tlog.Log.Error("GetServerList decode failed: err=%v", err)
			return &pb_pf.ServerListAck{Code: pb_pf.ErrCode_INTERNAL}, nil
		}
		accounts = append(accounts, &pb_pf.ServerAccount{ServerId: account.ServerId, Uid: account.UID})
	}
	if err := cursor.Err(); err != nil {
		tlog.Log.Error("GetServerList cursor failed: err=%v", err)
		return &pb_pf.ServerListAck{Code: pb_pf.ErrCode_INTERNAL}, nil
	}
	tlog.Log.Info("GetServerList succeeded: accounts=%v", accounts)
	return &pb_pf.ServerListAck{Code: pb_pf.ErrCode_OK, Accounts: accounts}, nil
}

func VerifyToken(req *pb_pf.VerifyTokenReq) (*pb_pf.VerifyAck, error) {
	if req == nil {
		tlog.Log.Warn("VerifyToken rejected: nil request")
		return &pb_pf.VerifyAck{Valid: false, Code: pb_pf.ErrCode_PARAM.Enum()}, nil
	}

	uid, err := auth.Verify(req.Token)
	if err != nil {
		tlog.Log.Warn("VerifyToken rejected: verification failed, err=%v", err)
		return &pb_pf.VerifyAck{Valid: false, Code: pb_pf.ErrCode_INVALID_TOKEN.Enum()}, nil
	}
	tlog.Log.Info("VerifyToken succeeded: uid=%d", uid)
	return &pb_pf.VerifyAck{Valid: true, Uid: uid, Code: pb_pf.ErrCode_OK.Enum()}, nil
}

// EnsureIndexes creates account indexes during startup.
func EnsureIndexes(db *mongo.Database) error {
	ctx, cancel := bg()
	defer cancel()
	_, err := db.Collection(store.ColAccounts).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "account", Value: 1}, {Key: "server_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		tlog.Log.Error("EnsureIndexes failed: err=%v", err)
		return err
	}
	tlog.Log.Info("EnsureIndexes succeeded")
	return nil
}
