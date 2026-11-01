package battle

import "errors"

var (
	// ErrUnavailable indicates that the battle client cannot accept new requests.
	ErrUnavailable = errors.New("battle client is unavailable")
	// ErrInvalidRequest identifies missing or invalid battle request data.
	ErrInvalidRequest = errors.New("invalid battle request")
	// ErrInProgress prevents a second battle while one is active.
	ErrInProgress = errors.New("battle is already in progress")
)
