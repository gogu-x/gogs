// Package ipb defines in-process messages shared by game actors.
package ipb

// PushToMsg routes an encoded client payload to the live session identified by UID.
// Payload uses tree/codec.ProtoCodec framing and can be forwarded to the client as-is.
type PushToMsg struct {
	UID uint64
	Msg interface{}
}

// BanUID and UnbanUID control the in-memory session ban list.
type BanUID struct{ UID uint64 }

type UnbanUID struct{ UID uint64 }

// SessionClosed notifies Play that a player's gateway session has closed.
type SessionClosed struct{ UID uint64 }
