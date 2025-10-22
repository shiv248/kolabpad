// Package server implements the Kolabpad collaborative editing server.
package server

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shiv248/kolabpad/internal/protocol"
	"github.com/shiv248/kolabpad/pkg/logger"
	"github.com/shiv248/kolabpad/pkg/ot"
)

// State represents the shared document state protected by a lock.
type State struct {
	Operations []protocol.UserOperation     // Complete operation history
	Text       string                        // Current document text
	Language   *string                       // Syntax highlighting language
	OTP        *string                       // One-time password for document protection
	Users      map[uint64]protocol.UserInfo  // Connected users
	Cursors    map[uint64]protocol.CursorData // User cursor positions
}

// Kolabpad is the main collaborative editing session manager.
type Kolabpad struct {
	state                *State
	mu                   sync.RWMutex
	count                atomic.Uint64                         // User ID counter
	killed               atomic.Bool                           // Document destruction flag
	lastEditTime         atomic.Int64                          // Unix timestamp of last edit (for idle detection)
	lastPersistedRevision atomic.Int32                         // Last revision written to DB
	lastCriticalWrite    atomic.Int64                          // Unix timestamp of last critical write (OTP changes)
	subscribers          map[uint64]chan *protocol.ServerMsg  // Per-connection channels for metadata broadcasts
	notify               chan struct{}                         // Closed to wake all connections when new operations arrive
	maxDocumentSize      int                                   // Maximum document size in bytes
	broadcastBufferSize  int                                   // Buffer size for metadata broadcast channels
}

// NewKolabpad creates a new collaborative editing session.
func NewKolabpad(maxDocumentSize, broadcastBufferSize int) *Kolabpad {
	return &Kolabpad{
		state: &State{
			Operations: make([]protocol.UserOperation, 0),
			Text:       "",
			Language:   nil,
			Users:      make(map[uint64]protocol.UserInfo),
			Cursors:    make(map[uint64]protocol.CursorData),
		},
		subscribers:         make(map[uint64]chan *protocol.ServerMsg),
		notify:              make(chan struct{}),
		maxDocumentSize:     maxDocumentSize,
		broadcastBufferSize: broadcastBufferSize,
	}
}

// FromPersistedDocument creates a Kolabpad instance from a persisted document.
func FromPersistedDocument(text string, language *string, otp *string, maxDocumentSize, broadcastBufferSize int) *Kolabpad {
	r := NewKolabpad(maxDocumentSize, broadcastBufferSize)

	// Initialize OTP from persisted state
	r.state.OTP = otp

	// Create an initial insert operation for the loaded text
	if text != "" {
		op := ot.NewOperationSeq()
		op.Insert(text)

		r.state.Text = text
		r.state.Language = language
		r.state.Operations = []protocol.UserOperation{
			{
				ID:        protocol.SystemUserID, // System operation
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

// GetOTP returns the current OTP (thread-safe).
func (r *Kolabpad) GetOTP() *string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state.OTP
}

// UserCount returns the number of connected users (thread-safe).
func (r *Kolabpad) UserCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.state.Users)
}

// HasUser checks if a user is currently connected to this document.
func (r *Kolabpad) HasUser(userID uint64) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.state.Users[userID]
	return exists
}

// LastEditTime returns the time of the last edit.
func (r *Kolabpad) LastEditTime() time.Time {
	timestamp := r.lastEditTime.Load()
	if timestamp == 0 {
		return time.Time{} // Zero time if never edited
	}
	return time.Unix(timestamp, 0)
}

// Kill marks this document as killed and closes channels to disconnect all clients.
func (r *Kolabpad) Kill() {
	if r.killed.CompareAndSwap(false, true) {
		r.mu.Lock()
		// Close all subscriber channels
		for _, ch := range r.subscribers {
			close(ch)
		}
		r.subscribers = make(map[uint64]chan *protocol.ServerMsg)
		// Close notify channel to wake all connections
		close(r.notify)
		r.mu.Unlock()
	}
}

// Killed returns true if this document has been killed.
func (r *Kolabpad) Killed() bool {
	return r.killed.Load()
}

// Subscribe creates a new channel for receiving metadata updates.
func (r *Kolabpad) Subscribe(userID uint64) <-chan *protocol.ServerMsg {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch := make(chan *protocol.ServerMsg, r.broadcastBufferSize)
	r.subscribers[userID] = ch
	return ch
}

// Unsubscribe removes a channel from receiving metadata updates.
func (r *Kolabpad) Unsubscribe(userID uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ch, ok := r.subscribers[userID]; ok {
		close(ch)
		delete(r.subscribers, userID)
	}
}

// NotifyChannel returns the current notify channel for operation broadcasts.
func (r *Kolabpad) NotifyChannel() <-chan struct{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.notify
}

// broadcast sends a message to all subscribers (non-blocking).
func (r *Kolabpad) broadcast(msg *protocol.ServerMsg) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, ch := range r.subscribers {
		select {
		case ch <- msg:
		default:
			// Skip if subscriber channel is full
		}
	}
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

	// Track edit time for idle detection
	r.lastEditTime.Store(time.Now().Unix())

	currentLen := len(r.state.Operations)
	oldTextLen := len(r.state.Text)

	logger.Debug("ApplyEdit: user=%d, revision=%d/%d, op(base=%d, target=%d), docLen=%d",
		userID, revision, currentLen, operation.BaseLen(), operation.TargetLen(), oldTextLen)

	// Validate revision
	if revision > currentLen {
		return fmt.Errorf("invalid revision: got %d, current is %d", revision, currentLen)
	}

	// Transform against all operations since the client's revision
	transformed := operation
	transformCount := len(r.state.Operations[revision:])
	if transformCount > 0 {
		logger.Debug("ApplyEdit: transforming against %d historical operation(s)", transformCount)
	}
	for _, histOp := range r.state.Operations[revision:] {
		aPrime, _, err := transformed.Transform(histOp.Operation)
		if err != nil {
			return fmt.Errorf("transform failed: %w", err)
		}
		transformed = aPrime
	}

	// Enforce size limit
	if int(transformed.TargetLen()) > r.maxDocumentSize {
		return fmt.Errorf("target length %d exceeds maximum of %d bytes", transformed.TargetLen(), r.maxDocumentSize)
	}

	// Apply operation to text
	newText, err := transformed.Apply(r.state.Text)
	if err != nil {
		return fmt.Errorf("apply failed: %w", err)
	}

	logger.Debug("ApplyEdit: text changed from %d to %d bytes, notifying %d connection(s)",
		oldTextLen, len(newText), len(r.subscribers))

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

	// Notify all connections of new operation (broadcast by closing and recreating channel)
	// Only do this if document hasn't been killed
	if !r.killed.Load() {
		close(r.notify)
		r.notify = make(chan struct{})
	}

	return nil
}

// SetLanguage sets the document's syntax highlighting language.
func (r *Kolabpad) SetLanguage(lang string, userID uint64, userName string) {
	r.mu.Lock()
	r.state.Language = &lang
	r.mu.Unlock()

	// Track edit time for idle detection
	r.lastEditTime.Store(time.Now().Unix())

	// Broadcast to all clients with user info
	r.broadcast(protocol.NewLanguageMsg(lang, userID, userName))
}

// SetOTP updates the OTP in state and broadcasts to all connected clients.
func (r *Kolabpad) SetOTP(otp *string, userID uint64, userName string) {
	// Update state
	r.mu.Lock()
	r.state.OTP = otp
	r.mu.Unlock()

	// Mark as critical write (for persister debouncing)
	r.lastCriticalWrite.Store(time.Now().Unix())

	// Broadcast to all authenticated clients with user info
	r.broadcast(protocol.NewOTPMsg(otp, userID, userName))
}

// SetUserInfo updates a user's display information.
func (r *Kolabpad) SetUserInfo(userID uint64, info protocol.UserInfo) {
	r.mu.Lock()
	r.state.Users[userID] = info
	r.mu.Unlock()

	// Broadcast to all clients
	r.broadcast(protocol.NewUserInfoMsg(userID, &info))
}

// SetCursorData updates a user's cursor positions.
func (r *Kolabpad) SetCursorData(userID uint64, data protocol.CursorData) {
	r.mu.Lock()
	r.state.Cursors[userID] = data
	r.mu.Unlock()

	// Broadcast to all clients
	r.broadcast(protocol.NewUserCursorMsg(userID, data))
}

// RemoveUser removes a user from the session.
func (r *Kolabpad) RemoveUser(userID uint64) {
	r.mu.Lock()
	delete(r.state.Users, userID)
	delete(r.state.Cursors, userID)
	r.mu.Unlock()

	// Unsubscribe from updates
	r.Unsubscribe(userID)

	// Broadcast disconnection
	r.broadcast(protocol.NewUserInfoMsg(userID, nil))
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
