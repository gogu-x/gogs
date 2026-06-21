// Package mongo provides a common MongoDB Actor that serializes all database
// operations to prevent concurrency issues. Any process needing MongoDB should
// Spawn one Actor and interact via messages.
package mongo

import (
	"context"
	"log"
	"time"

	actor "github.com/gogu-x/bigTree"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// InsertOne inserts a document. Response: (insertedID, error)
type InsertOne struct {
	Collection string
	Doc        interface{}
}

// FindOne queries a single document, decoding into Result. Response: error
type FindOne struct {
	Collection string
	Filter     interface{}
	Result     interface{}
}

// UpdateOne updates a single document. Response: error
type UpdateOne struct {
	Collection string
	Filter     interface{}
	Update     interface{}
}

// DeleteOne deletes a single document. Response: error
type DeleteOne struct {
	Collection string
	Filter     interface{}
}

type Actor struct {
	router actor.Router
	db     *mongo.Database
}

func NewActor(db *mongo.Database) *Actor { return &Actor{db: db} }

func (a *Actor) OnInit(_ actor.ActorContext) {
	a.router.Register(&InsertOne{}, a.onInsert)
	a.router.Register(&FindOne{}, a.onFind)
	a.router.Register(&UpdateOne{}, a.onUpdate)
	a.router.Register(&DeleteOne{}, a.onDelete)
	log.Printf("rpc/mongo: ready, db=%s", a.db.Name())
}

func (a *Actor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	a.router.Route(ctx, msg)
}

func (a *Actor) OnStop(_ actor.ActorContext) {}

func (a *Actor) onInsert(ctx actor.ActorContext, msg interface{}) {
	m := msg.(*InsertOne)
	res, err := a.db.Collection(m.Collection).InsertOne(bg(), m.Doc)
	if err != nil {
		ctx.Response(nil, err)
		return
	}
	ctx.Response(res.InsertedID, nil)
}

func (a *Actor) onFind(ctx actor.ActorContext, msg interface{}) {
	m := msg.(*FindOne)
	err := a.db.Collection(m.Collection).FindOne(bg(), m.Filter).Decode(m.Result)
	ctx.Response(m.Result, err)
}

func (a *Actor) onUpdate(ctx actor.ActorContext, msg interface{}) {
	m := msg.(*UpdateOne)
	_, err := a.db.Collection(m.Collection).UpdateOne(bg(), m.Filter, m.Update)
	ctx.Response(nil, err)
}

func (a *Actor) onDelete(ctx actor.ActorContext, msg interface{}) {
	m := msg.(*DeleteOne)
	_, err := a.db.Collection(m.Collection).DeleteOne(bg(), m.Filter)
	ctx.Response(nil, err)
}

func bg() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = cancel
	return ctx
}

// Connect connects to MongoDB and returns the named database.
func Connect(uri, dbName string) *mongo.Database {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatalf("rpc/mongo.Connect: %v", err)
	}
	return client.Database(dbName)
}
