// Package message defines in-process messages shared by Game actors.
package message

// SessionClosed notifies Play that the active stream for a UID has closed.
type SessionClosed struct {
	UID uint64 // Closed player UID.
}
