package ipb

// PushToMsg routes a Game-side frame to the live stream identified by UID.
type PushToMsg struct {
	UID uint64
	Msg interface{}
}

// BanUID and UnbanUID are control messages for the in-memory session ban list.
type BanUID struct{ UID uint64 }

type UnbanUID struct{ UID uint64 }

type SessionClosed struct {
	UID uint64 // Closed player UID.
}
