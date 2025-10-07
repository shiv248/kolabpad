package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/shiv/kolabpad/internal/protocol"
)

// readResult represents the result of a WebSocket read operation.
type readResult struct {
	msg protocol.ClientMsg
	err error
}

// Connection represents a single client WebSocket connection.
type Connection struct {
	userID  uint64
	kolabpad *Kolabpad
	conn    *websocket.Conn
	ctx     context.Context
	cancel  context.CancelFunc
	sendMu  sync.Mutex
}

// NewConnection creates a new client connection handler.
func NewConnection(kolabpad *Kolabpad, conn *websocket.Conn) *Connection {
	ctx, cancel := context.WithCancel(context.Background())
	return &Connection{
		userID:  kolabpad.NextUserID(),
		kolabpad: kolabpad,
		conn:    conn,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Handle manages the WebSocket connection lifecycle.
func (c *Connection) Handle(ctx context.Context) error {
	defer c.cleanup()

	log.Printf("connection! id = %d", c.userID)

	// Send initial state to client
	revision, err := c.sendInitial()
	if err != nil {
		return fmt.Errorf("send initial: %w", err)
	}

	// Subscribe to metadata updates
	updates := c.kolabpad.Subscribe(c.userID)

	// Start update broadcaster
	updatesDone := make(chan struct{})
	go c.broadcastUpdates(updates, updatesDone)

	// Start first read
	readChan := make(chan readResult, 1)
	go c.readMessage(ctx, readChan)

	// Main message loop
	for {
		// Get current notify channel (before checking revision to avoid race)
		notified := c.kolabpad.NotifyChannel()

		// Check if document has been killed
		if c.kolabpad.Killed() {
			return nil
		}

		// Check for new history to send
		if c.kolabpad.Revision() > revision {
			newRev, err := c.sendHistory(revision)
			if err != nil {
				return fmt.Errorf("send history: %w", err)
			}
			revision = newRev
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-notified:
			// Notify channel closed, new operation available - loop to check revision
		case result := <-readChan:
			if result.err != nil {
				// Check if it's a normal close
				if websocket.CloseStatus(result.err) == websocket.StatusNormalClosure {
					return nil
				}
				return fmt.Errorf("read message: %w", result.err)
			}

			// Handle message
			if err := c.handleMessage(&result.msg); err != nil {
				log.Printf("error handling message from user %d: %v", c.userID, err)
				return err
			}

			// Start next read
			readChan = make(chan readResult, 1)
			go c.readMessage(ctx, readChan)
		}
	}
}

// readMessage reads a message from the WebSocket in a separate goroutine.
func (c *Connection) readMessage(ctx context.Context, result chan<- readResult) {
	readCtx, readCancel := context.WithTimeout(ctx, 30*time.Minute)
	defer readCancel()

	var msg protocol.ClientMsg
	err := wsjson.Read(readCtx, c.conn, &msg)

	result <- readResult{msg: msg, err: err}
}

// sendInitial sends the initial state to a newly connected client.
func (c *Connection) sendInitial() (int, error) {
	// Send Identity
	if err := c.send(protocol.NewIdentityMsg(c.userID)); err != nil {
		return 0, err
	}

	// Get initial state
	ops, lang, users, cursors := c.kolabpad.GetInitialState()

	// Send operation history
	if len(ops) > 0 {
		if err := c.send(protocol.NewHistoryMsg(0, ops)); err != nil {
			return 0, err
		}
	}

	// Send language
	if lang != nil {
		if err := c.send(protocol.NewLanguageMsg(*lang)); err != nil {
			return 0, err
		}
	}

	// Send all users
	for id, info := range users {
		infoCopy := info
		if err := c.send(protocol.NewUserInfoMsg(id, &infoCopy)); err != nil {
			return 0, err
		}
	}

	// Send all cursors
	for id, data := range cursors {
		if err := c.send(protocol.NewUserCursorMsg(id, data)); err != nil {
			return 0, err
		}
	}

	return len(ops), nil
}

// sendHistory sends operation history from a starting revision.
func (c *Connection) sendHistory(start int) (int, error) {
	ops := c.kolabpad.GetHistory(start)
	if len(ops) > 0 {
		if err := c.send(protocol.NewHistoryMsg(start, ops)); err != nil {
			return start, err
		}
	}
	return start + len(ops), nil
}

// handleMessage processes a message from the client.
func (c *Connection) handleMessage(msg *protocol.ClientMsg) error {
	if msg.Edit != nil {
		// Apply edit operation
		if err := c.kolabpad.ApplyEdit(c.userID, msg.Edit.Revision, msg.Edit.Operation); err != nil {
			return fmt.Errorf("apply edit: %w", err)
		}
		return nil
	}

	if msg.SetLanguage != nil {
		c.kolabpad.SetLanguage(*msg.SetLanguage)
		return nil
	}

	if msg.ClientInfo != nil {
		c.kolabpad.SetUserInfo(c.userID, *msg.ClientInfo)
		return nil
	}

	if msg.CursorData != nil {
		c.kolabpad.SetCursorData(c.userID, *msg.CursorData)
		return nil
	}

	return nil
}

// broadcastUpdates forwards metadata updates to this client.
func (c *Connection) broadcastUpdates(updates <-chan *protocol.ServerMsg, done chan struct{}) {
	defer close(done)

	for {
		select {
		case <-c.ctx.Done():
			return
		case msg, ok := <-updates:
			if !ok {
				// Channel closed, kolabpad killed
				return
			}
			if err := c.send(msg); err != nil {
				log.Printf("error broadcasting to user %d: %v", c.userID, err)
				c.cancel()
				return
			}
		}
	}
}

// send sends a message to the client (thread-safe).
func (c *Connection) send(msg *protocol.ServerMsg) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	writeCtx, writeCancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer writeCancel()
	return c.conn.Write(writeCtx, websocket.MessageText, data)
}

// cleanup removes the user from the session.
func (c *Connection) cleanup() {
	log.Printf("disconnection, id = %d", c.userID)
	c.kolabpad.RemoveUser(c.userID)
	c.cancel()
}
