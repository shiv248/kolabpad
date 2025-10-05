// Package server implements the Kolabpad collaborative editing server.
package server

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/shiv/kolabpad/internal/protocol"
	"github.com/shiv/kolabpad/pkg/ot"
)

// State represents the shared document state protected by a lock.
type State struct {
	Operations []protocol.UserOperation     // Complete operation history
	Text       string                        // Current document text
	Language   *string                       // Syntax highlighting language
	Users      map[uint64]protocol.UserInfo  // Connected users
	Cursors    map[uint64]protocol.CursorData // User cursor positions
}

// Kolabpad is the main collaborative editing session manager.
type Kolabpad struct {
	state   *State
	mu      sync.RWMutex
	count   atomic.Uint64       // User ID counter
	killed  atomic.Bool         // Document destruction flag
	updates chan *protocol.ServerMsg // Broadcast channel for metadata updates
}

// NewKolabpad creates a new collaborative editing session.
func NewKolabpad() *Kolabpad {
	return &Kolabpad{
		state: &State{
			Operations: make([]protocol.UserOperation, 0),
			Text:       "",
			Language:   nil,
			Users:      make(map[uint64]protocol.UserInfo),
			Cursors:    make(map[uint64]protocol.CursorData),
		},
		updates: make(chan *protocol.ServerMsg, 16),
	}
}

// FromPersistedDocument creates a Kolabpad instance from a persisted document.
func FromPersistedDocument(text string, language *string) *Kolabpad {
	r := NewKolabpad()

	// Create an initial insert operation for the loaded text
	if text != "" {
		op := ot.NewOperationSeq()
		op.Insert(text)

		r.state.Text = text
		r.state.Language = language
		r.state.Operations = []protocol.UserOperation{
			{
				ID:        ^uint64(0), // u64::MAX - system operation
				Operation: op,
			},
		}
	}

	return r
}

// NextUserID returns the next available user ID.
func (r *Kolabpad) NextUserID() uint64 {
	return r.count.Add(1) - 1
}

// Revision returns the current revision number.
func (r *Kolabpad) Revision() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.state.Operations)
}

// Text returns a copy of the current document text.
func (r *Kolabpad) Text() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state.Text
}

// Snapshot returns a snapshot of the current document for persistence.
func (r *Kolabpad) Snapshot() (text string, language *string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state.Text, r.state.Language
}

// Kill marks this document as killed and closes the update channel.
func (r *Kolabpad) Kill() {
	if r.killed.CompareAndSwap(false, true) {
		close(r.updates)
	}
}

// Killed returns true if this document has been killed.
func (r *Kolabpad) Killed() bool {
	return r.killed.Load()
}

// Updates returns the channel for receiving metadata updates.
func (r *Kolabpad) Updates() <-chan *protocol.ServerMsg {
	return r.updates
}

// GetInitialState returns the initial state to send to a connecting client.
func (r *Kolabpad) GetInitialState() (
	ops []protocol.UserOperation,
	lang *string,
	users map[uint64]protocol.UserInfo,
	cursors map[uint64]protocol.CursorData,
) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Make copies to avoid race conditions
	ops = make([]protocol.UserOperation, len(r.state.Operations))
	copy(ops, r.state.Operations)

	lang = r.state.Language

	users = make(map[uint64]protocol.UserInfo)
	for k, v := range r.state.Users {
		users[k] = v
	}

	cursors = make(map[uint64]protocol.CursorData)
	for k, v := range r.state.Cursors {
		cursors[k] = v
	}

	return
}

// GetHistory returns operations from a starting revision.
func (r *Kolabpad) GetHistory(start int) []protocol.UserOperation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	length := len(r.state.Operations)
	if start >= length {
		return []protocol.UserOperation{}
	}

	ops := make([]protocol.UserOperation, length-start)
	copy(ops, r.state.Operations[start:])
	return ops
}

// ApplyEdit applies an edit operation from a client.
func (r *Kolabpad) ApplyEdit(userID uint64, revision int, operation *ot.OperationSeq) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	currentLen := len(r.state.Operations)

	// Validate revision
	if revision > currentLen {
		return fmt.Errorf("invalid revision: got %d, current is %d", revision, currentLen)
	}

	// Transform against all operations since the client's revision
	transformed := operation
	for _, histOp := range r.state.Operations[revision:] {
		aPrime, _, err := transformed.Transform(histOp.Operation)
		if err != nil {
			return fmt.Errorf("transform failed: %w", err)
		}
		transformed = aPrime
	}

	// Enforce size limit (256 KiB)
	if transformed.TargetLen() > 256*1024 {
		return fmt.Errorf("target length %d exceeds 256 KiB maximum", transformed.TargetLen())
	}

	// Apply operation to text
	newText, err := transformed.Apply(r.state.Text)
	if err != nil {
		return fmt.Errorf("apply failed: %w", err)
	}

	// Transform all user cursors
	for id, cursorData := range r.state.Cursors {
		newCursors := make([]uint32, len(cursorData.Cursors))
		for i, cursor := range cursorData.Cursors {
			newCursors[i] = transformIndex(transformed, cursor)
		}

		newSelections := make([][2]uint32, len(cursorData.Selections))
		for i, sel := range cursorData.Selections {
			newSelections[i] = [2]uint32{
				transformIndex(transformed, sel[0]),
				transformIndex(transformed, sel[1]),
			}
		}

		r.state.Cursors[id] = protocol.CursorData{
			Cursors:    newCursors,
			Selections: newSelections,
		}
	}

	// Store operation and update text
	r.state.Operations = append(r.state.Operations, protocol.UserOperation{
		ID:        userID,
		Operation: transformed,
	})
	r.state.Text = newText

	return nil
}

// SetLanguage sets the document's syntax highlighting language.
func (r *Kolabpad) SetLanguage(lang string) {
	r.mu.Lock()
	r.state.Language = &lang
	r.mu.Unlock()

	// Broadcast to all clients
	select {
	case r.updates <- protocol.NewLanguageMsg(lang):
	default:
	}
}

// SetUserInfo updates a user's display information.
func (r *Kolabpad) SetUserInfo(userID uint64, info protocol.UserInfo) {
	r.mu.Lock()
	r.state.Users[userID] = info
	r.mu.Unlock()

	// Broadcast to all clients
	select {
	case r.updates <- protocol.NewUserInfoMsg(userID, &info):
	default:
	}
}

// SetCursorData updates a user's cursor positions.
func (r *Kolabpad) SetCursorData(userID uint64, data protocol.CursorData) {
	r.mu.Lock()
	r.state.Cursors[userID] = data
	r.mu.Unlock()

	// Broadcast to all clients
	select {
	case r.updates <- protocol.NewUserCursorMsg(userID, data):
	default:
	}
}

// RemoveUser removes a user from the session.
func (r *Kolabpad) RemoveUser(userID uint64) {
	r.mu.Lock()
	delete(r.state.Users, userID)
	delete(r.state.Cursors, userID)
	r.mu.Unlock()

	// Broadcast disconnection
	select {
	case r.updates <- protocol.NewUserInfoMsg(userID, nil):
	default:
	}
}

// transformIndex transforms a cursor position through an operation.
// This is ported from rustpad-server/src/ot.rs
func transformIndex(operation *ot.OperationSeq, position uint32) uint32 {
	index := int32(position)
	newIndex := index

	for _, op := range operation.Ops() {
		switch v := op.(type) {
		case ot.Retain:
			index -= int32(v.N)
		case ot.Insert:
			// Count characters in the inserted text
			charCount := int32(len([]rune(v.Text)))
			newIndex += charCount
		case ot.Delete:
			if index >= int32(v.N) {
				newIndex -= int32(v.N)
			} else if index > 0 {
				newIndex -= index
			}
			index -= int32(v.N)
		}

		if index < 0 {
			break
		}
	}

	if newIndex < 0 {
		return 0
	}
	return uint32(newIndex)
}
