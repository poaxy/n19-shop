package ctxkeys

// Package ctxkeys defines typed keys for context values used across packages.
// Using typed keys avoids collisions with other context users.

type Key int

const (
	Username Key = iota
	Plan
)
