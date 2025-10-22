// Package protocol defines constants used across the protocol.
package protocol

const (
	// SystemUserID is the user ID used for system-generated operations and initial state.
	// Set to max uint64 (^uint64(0)) to avoid conflicts with real user IDs (0, 1, 2, ...).
	SystemUserID = ^uint64(0) // 18446744073709551615
)
