package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/shiv248/kolabpad/internal/protocol"
	"github.com/shiv248/kolabpad/pkg/logger"
)

// readResult represents the result of a WebSocket read operation.
type readResult struct {
	msg protocol.ClientMsg
	err error
}

// Connection represents a single client WebSocket connection.
type Connection struct {
	userID       uint64
	kolabpad     *Kolabpad
	conn         *websocket.Conn
	ctx          context.Context
	cancel       context.CancelFunc
	sendMu       sync.Mutex
	readTimeout  time.Duration
	writeTimeout time.Duration
}

// NewConnection creates a new client connection handler.
func NewConnection(kolabpad *Kolabpad, conn *websocket.Conn, readTimeout, writeTimeout time.Duration) *Connection {
	ctx, cancel := context.WithCancel(context.Background())
	return &Connection{
		userID:       kolabpad.NextUserID(),
		kolabpad:     kolabpad,
		conn:         conn,
		ctx:          ctx,
		cancel:       cancel,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
	}
}

// Handle manages the WebSocket connection lifecycle.
func (c *Connection) Handle(ctx context.Context) error {
	defer c.cleanup()

	logger.Info("User %d connected", c.userID)

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
				status := websocket.CloseStatus(result.err)
				if status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
					return nil
				}
				return fmt.Errorf("read message: %w", result.err)
			}

			// Handle message
			if err := c.handleMessage(&result.msg); err != nil {
				logger.Error("Error handling message from user %d: %v", c.userID, err)
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
	readCtx, readCancel := context.WithTimeout(ctx, c.readTimeout)
	defer readCancel()

	var msg protocol.ClientMsg
	err := wsjson.Read(readCtx, c.conn, &msg)

	if err == nil {
		logger.Debug("User %d received message: Edit=%v, SetLanguage=%v, ClientInfo=%v, CursorData=%v",
			c.userID,
			msg.Edit != nil,
			msg.SetLanguage != nil,
			msg.ClientInfo != nil,
			msg.CursorData != nil)
	}

	result <- readResult{msg: msg, err: err}
}

// sendInitial sends the initial state to a newly connected client.
func (c *Connection) sendInitial() (int, error) {
	// Send Identity
	logger.Debug("User %d sending Identity", c.userID)
	if err := c.send(protocol.NewIdentityMsg(c.userID)); err != nil {
		return 0, err
	}

	// Get initial state
	ops, lang, users, cursors := c.kolabpad.GetInitialState()

	// Send operation history
	if len(ops) > 0 {
		logger.Debug("User %d sending History: %d operations from revision 0", c.userID, len(ops))
		if err := c.send(protocol.NewHistoryMsg(0, ops)); err != nil {
			return 0, err
		}
	}

	// Send language (with system user ID for initial state)
	if lang != nil {
		logger.Debug("User %d sending Language: %s", c.userID, *lang)
		if err := c.send(protocol.NewLanguageMsg(*lang, protocol.SystemUserID, "System")); err != nil {
			return 0, err
		}
	}

	// Send all users
	logger.Debug("User %d sending %d user(s)", c.userID, len(users))
	for id, info := range users {
		infoCopy := info
		if err := c.send(protocol.NewUserInfoMsg(id, &infoCopy)); err != nil {
			return 0, err
		}
	}

	// Send all cursors
	logger.Debug("User %d sending %d cursor(s)", c.userID, len(cursors))
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
		logger.Debug("User %d sending History: %d operations from revision %d", c.userID, len(ops), start)
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
		logger.Debug("User %d applying Edit at revision %d (base=%d, target=%d)",
			c.userID, msg.Edit.Revision, msg.Edit.Operation.BaseLen(), msg.Edit.Operation.TargetLen())
		if err := c.kolabpad.ApplyEdit(c.userID, msg.Edit.Revision, msg.Edit.Operation); err != nil {
			return fmt.Errorf("apply edit: %w", err)
		}
		return nil
	}

	if msg.SetLanguage != nil {
		userName := c.getUserName()
		logger.Debug("User %d (%s) setting Language: %s", c.userID, userName, *msg.SetLanguage)
		c.kolabpad.SetLanguage(*msg.SetLanguage, c.userID, userName)
		return nil
	}

	if msg.ClientInfo != nil {
		logger.Debug("User %d setting ClientInfo: name=%s, hue=%d", c.userID, msg.ClientInfo.Name, msg.ClientInfo.Hue)
		c.kolabpad.SetUserInfo(c.userID, *msg.ClientInfo)
		return nil
	}

	if msg.CursorData != nil {
		logger.Debug("User %d setting CursorData: %d cursors, %d selections", c.userID, len(msg.CursorData.Cursors), len(msg.CursorData.Selections))
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
			// Log what type of broadcast message
			msgType := "Unknown"
			if msg.Language != nil {
				msgType = "Language"
			} else if msg.UserInfo != nil {
				msgType = "UserInfo"
			} else if msg.UserCursor != nil {
				msgType = "UserCursor"
			}
			logger.Debug("User %d broadcasting %s", c.userID, msgType)

			if err := c.send(msg); err != nil {
				logger.Error("Error broadcasting to user %d: %v", c.userID, err)
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

	writeCtx, writeCancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer writeCancel()
	return c.conn.Write(writeCtx, websocket.MessageText, data)
}

// cleanup removes the user from the session.
func (c *Connection) cleanup() {
	logger.Info("User %d disconnected", c.userID)
	c.kolabpad.RemoveUser(c.userID)
	c.cancel()
}

// getUserName returns the user's display name from the kolabpad state.
// Returns empty string if user info is not found.
func (c *Connection) getUserName() string {
	c.kolabpad.mu.RLock()
	defer c.kolabpad.mu.RUnlock()

	if userInfo, exists := c.kolabpad.state.Users[c.userID]; exists {
		return userInfo.Name
	}
	return ""
}
