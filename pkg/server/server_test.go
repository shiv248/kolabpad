package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/shiv248/kolabpad/internal/protocol"
	"github.com/shiv248/kolabpad/pkg/database"
	ot "github.com/shiv248/operational-transformation-go"
)

// testServer creates a test server with an in-memory database.
func testServer(t *testing.T) *Server {
	t.Helper()

	db, err := database.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	// Create server with test-friendly settings
	const maxDocumentSize = 256 * 1024
	const broadcastBufferSize = 256
	const wsReadTimeout = 5 * time.Minute
	const wsWriteTimeout = 5 * time.Second

	return NewServer(db, maxDocumentSize, broadcastBufferSize, wsReadTimeout, wsWriteTimeout)
}

// testServerNoDb creates a test server without a database.
func testServerNoDb(t *testing.T) *Server {
	t.Helper()

	// Create server with test-friendly settings
	const maxDocumentSize = 256 * 1024
	const broadcastBufferSize = 256
	const wsReadTimeout = 5 * time.Minute
	const wsWriteTimeout = 5 * time.Second

	return NewServer(nil, maxDocumentSize, broadcastBufferSize, wsReadTimeout, wsWriteTimeout)
}

// connectWebSocket establishes a WebSocket connection to a test server.
func connectWebSocket(t *testing.T, server *httptest.Server, docID string, otp string) *websocket.Conn {
	t.Helper()

	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/socket/" + docID
	if otp != "" {
		url += "?otp=" + otp
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("Failed to connect WebSocket: %v", err)
	}

	t.Cleanup(func() {
		conn.Close(websocket.StatusNormalClosure, "")
	})

	return conn
}

// readServerMsg reads a message from the WebSocket and returns the parsed ServerMsg.
func readServerMsg(t *testing.T, conn *websocket.Conn) *protocol.ServerMsg {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var msg protocol.ServerMsg
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	return &msg
}

// sendClientMsg sends a ClientMsg to the server.
func sendClientMsg(t *testing.T, conn *websocket.Conn, msg *protocol.ClientMsg) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := wsjson.Write(ctx, conn, msg); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}
}

// TestSingleUserConnection tests that a single user can connect and receive initial state.
func TestSingleUserConnection(t *testing.T) {
	server := testServer(t)
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Connect client
	conn := connectWebSocket(t, ts, "test123", "")

	// Read Identity message
	msg := readServerMsg(t, conn)
	if msg.Identity == nil {
		t.Fatalf("Expected Identity message, got %+v", msg)
	}
	if *msg.Identity != 0 {
		t.Errorf("Expected first user to get ID 0, got %d", *msg.Identity)
	}

	// For a new document, we shouldn't receive a History message (empty document)
	// The connection should be waiting for operations
}

// TestMultipleUsersConnection tests that multiple users can connect to the same document.
func TestMultipleUsersConnection(t *testing.T) {
	server := testServer(t)
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Connect first client
	conn1 := connectWebSocket(t, ts, "test123", "")
	msg1 := readServerMsg(t, conn1)
	if msg1.Identity == nil || *msg1.Identity != 0 {
		t.Fatalf("Expected first user to get ID 0, got %+v", msg1)
	}

	// Connect second client
	conn2 := connectWebSocket(t, ts, "test123", "")
	msg2 := readServerMsg(t, conn2)
	if msg2.Identity == nil || *msg2.Identity != 1 {
		t.Fatalf("Expected second user to get ID 1, got %+v", msg2)
	}
}

// TestEditBroadcast tests that edits are broadcast to all connected users.
func TestEditBroadcast(t *testing.T) {
	server := testServer(t)
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Connect two clients
	conn1 := connectWebSocket(t, ts, "test123", "")
	readServerMsg(t, conn1) // Read Identity for client 1

	conn2 := connectWebSocket(t, ts, "test123", "")
	readServerMsg(t, conn2) // Read Identity for client 2

	// Client 1 sends an edit
	op := ot.NewOperationSeq()
	op.Insert("hello")

	sendClientMsg(t, conn1, &protocol.ClientMsg{
		Edit: &protocol.EditMsg{
			Revision:  0,
			Operation: op,
		},
	})

	// Both clients should receive the History message
	msg1 := readServerMsg(t, conn1)
	msg2 := readServerMsg(t, conn2)

	// Verify both received History messages
	if msg1.History == nil {
		t.Fatalf("Client 1 expected History message, got %+v", msg1)
	}
	if msg2.History == nil {
		t.Fatalf("Client 2 expected History message, got %+v", msg2)
	}

	// Verify the operation was broadcast correctly
	if len(msg1.History.Operations) != 1 {
		t.Errorf("Client 1 expected 1 operation, got %d", len(msg1.History.Operations))
	}
	if len(msg2.History.Operations) != 1 {
		t.Errorf("Client 2 expected 1 operation, got %d", len(msg2.History.Operations))
	}
}

// TestLanguageBroadcast tests that language changes are broadcast to all users.
func TestLanguageBroadcast(t *testing.T) {
	server := testServer(t)
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Connect two clients
	conn1 := connectWebSocket(t, ts, "test123", "")
	readServerMsg(t, conn1) // Read Identity

	// Set client info for client 1
	sendClientMsg(t, conn1, &protocol.ClientMsg{
		ClientInfo: &protocol.UserInfo{
			Name: "Alice",
			Hue:  120,
		},
	})
	readServerMsg(t, conn1) // Read UserInfo broadcast

	conn2 := connectWebSocket(t, ts, "test123", "")
	readServerMsg(t, conn2) // Read Identity
	readServerMsg(t, conn2) // Read UserInfo for existing user

	// Client 1 changes language
	lang := "javascript"
	sendClientMsg(t, conn1, &protocol.ClientMsg{
		SetLanguage: &lang,
	})

	// Both clients should receive the Language broadcast
	msg1 := readServerMsg(t, conn1)
	msg2 := readServerMsg(t, conn2)

	if msg1.Language == nil {
		t.Fatalf("Client 1 expected Language message, got %+v", msg1)
	}
	if msg2.Language == nil {
		t.Fatalf("Client 2 expected Language message, got %+v", msg2)
	}

	if msg1.Language.Language != "javascript" {
		t.Errorf("Client 1 expected language 'javascript', got '%s'", msg1.Language.Language)
	}
	if msg2.Language.Language != "javascript" {
		t.Errorf("Client 2 expected language 'javascript', got '%s'", msg2.Language.Language)
	}

	if msg1.Language.UserID != 0 {
		t.Errorf("Expected UserID 0, got %d", msg1.Language.UserID)
	}
	if msg1.Language.UserName != "Alice" {
		t.Errorf("Expected UserName 'Alice', got '%s'", msg1.Language.UserName)
	}
}

// TestOTPProtection tests the OTP protection flow.
func TestOTPProtection(t *testing.T) {
	server := testServer(t)
	ts := httptest.NewServer(server)
	defer ts.Close()

	docID := "protected-doc"

	// Connect client without OTP (should succeed for unprotected document)
	conn1 := connectWebSocket(t, ts, docID, "")
	msg := readServerMsg(t, conn1)
	if msg.Identity == nil || *msg.Identity != 0 {
		t.Fatalf("Expected Identity message with ID 0, got %+v", msg)
	}

	// Send ClientInfo to register in session
	sendClientMsg(t, conn1, &protocol.ClientMsg{
		ClientInfo: &protocol.UserInfo{
			Name: "Alice",
			Hue:  0,
		},
	})
	readServerMsg(t, conn1) // Read UserInfo broadcast

	// Enable OTP protection via REST API
	reqBody := `{"user_id": 0, "user_name": "Alice"}`
	resp, err := http.Post(ts.URL+"/api/document/"+docID+"/protect", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("Failed to protect document: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var protectResp struct {
		OTP string `json:"otp"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&protectResp); err != nil {
		t.Fatalf("Failed to decode protect response: %v", err)
	}

	if protectResp.OTP == "" {
		t.Fatal("Expected non-empty OTP")
	}

	// Client 1 should receive OTP broadcast
	otpMsg := readServerMsg(t, conn1)
	if otpMsg.OTP == nil {
		t.Fatalf("Expected OTP broadcast, got %+v", otpMsg)
	}
	if otpMsg.OTP.OTP == nil || *otpMsg.OTP.OTP != protectResp.OTP {
		t.Errorf("Expected OTP '%s', got %v", protectResp.OTP, otpMsg.OTP.OTP)
	}

	// Close first connection
	conn1.Close(websocket.StatusNormalClosure, "")

	// Try connecting without OTP (should fail)
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/socket/" + docID
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, resp, err = websocket.Dial(ctx, url, nil)
	if err == nil {
		t.Fatal("Expected connection to fail without OTP")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", resp.StatusCode)
	}

	// Connect with wrong OTP (should fail)
	url = "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/socket/" + docID + "?otp=wrong"
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, resp, err = websocket.Dial(ctx, url, nil)
	if err == nil {
		t.Fatal("Expected connection to fail with wrong OTP")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", resp.StatusCode)
	}

	// Connect with correct OTP (should succeed)
	conn2 := connectWebSocket(t, ts, docID, protectResp.OTP)
	msg2 := readServerMsg(t, conn2)
	if msg2.Identity == nil {
		t.Fatalf("Expected Identity message, got %+v", msg2)
	}
}

// TestOTPColdStart tests that OTP validation works for documents loaded from DB.
func TestOTPColdStart(t *testing.T) {
	server := testServer(t)
	ts := httptest.NewServer(server)
	defer ts.Close()

	docID := "cold-start-doc"

	// Connect and protect document
	conn1 := connectWebSocket(t, ts, docID, "")
	readServerMsg(t, conn1) // Read Identity

	// Send ClientInfo to register in session
	sendClientMsg(t, conn1, &protocol.ClientMsg{
		ClientInfo: &protocol.UserInfo{
			Name: "Bob",
			Hue:  60,
		},
	})
	readServerMsg(t, conn1) // Read UserInfo broadcast

	// Enable OTP
	reqBody := `{"user_id": 0, "user_name": "Bob"}`
	resp, err := http.Post(ts.URL+"/api/document/"+docID+"/protect", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("Failed to protect document: %v", err)
	}
	defer resp.Body.Close()

	var protectResp struct {
		OTP string `json:"otp"`
	}
	json.NewDecoder(resp.Body).Decode(&protectResp)

	// Close connection to evict from memory
	conn1.Close(websocket.StatusNormalClosure, "")

	// Wait for document to be flushed
	time.Sleep(100 * time.Millisecond)

	// Force evict from memory by accessing server state
	server.state.documents.Delete(docID)

	// Try connecting without OTP (should fail - cold start validation)
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/socket/" + docID
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, httpResp, err := websocket.Dial(ctx, url, nil)
	if err == nil {
		t.Fatal("Expected connection to fail without OTP on cold start")
	}
	if httpResp != nil && httpResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401 on cold start, got %d", httpResp.StatusCode)
	}

	// Connect with correct OTP (should succeed and load from DB)
	conn2 := connectWebSocket(t, ts, docID, protectResp.OTP)
	msg := readServerMsg(t, conn2)
	if msg.Identity == nil {
		t.Fatalf("Expected Identity message on cold start, got %+v", msg)
	}
}

// TestUnprotectDocument tests removing OTP protection.
func TestUnprotectDocument(t *testing.T) {
	server := testServer(t)
	ts := httptest.NewServer(server)
	defer ts.Close()

	docID := "unprotect-test"

	// Connect and protect document
	conn := connectWebSocket(t, ts, docID, "")
	readServerMsg(t, conn) // Read Identity

	// Send ClientInfo to register in session
	sendClientMsg(t, conn, &protocol.ClientMsg{
		ClientInfo: &protocol.UserInfo{
			Name: "Charlie",
			Hue:  180,
		},
	})
	readServerMsg(t, conn) // Read UserInfo broadcast

	// Enable OTP
	reqBody := `{"user_id": 0, "user_name": "Charlie"}`
	resp, err := http.Post(ts.URL+"/api/document/"+docID+"/protect", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("Failed to protect document: %v", err)
	}
	defer resp.Body.Close()

	var protectResp struct {
		OTP string `json:"otp"`
	}
	json.NewDecoder(resp.Body).Decode(&protectResp)
	otp := protectResp.OTP

	// Read OTP broadcast
	readServerMsg(t, conn)

	// Disable OTP
	unprotectBody := `{"user_id": 0, "user_name": "Charlie", "otp": "` + otp + `"}`
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/document/"+docID+"/protect", strings.NewReader(unprotectBody))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to unprotect document: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Expected status 204, got %d", resp.StatusCode)
	}

	// Client should receive OTP broadcast with nil
	otpMsg := readServerMsg(t, conn)
	if otpMsg.OTP == nil {
		t.Fatalf("Expected OTP broadcast, got %+v", otpMsg)
	}
	if otpMsg.OTP.OTP != nil {
		t.Errorf("Expected nil OTP, got %v", otpMsg.OTP.OTP)
	}

	// Close and reconnect without OTP (should succeed)
	conn.Close(websocket.StatusNormalClosure, "")

	conn2 := connectWebSocket(t, ts, docID, "")
	msg := readServerMsg(t, conn2)
	if msg.Identity == nil {
		t.Fatalf("Expected to connect without OTP after unprotect, got %+v", msg)
	}
}

// TestCursorBroadcast tests that cursor updates are broadcast.
func TestCursorBroadcast(t *testing.T) {
	server := testServer(t)
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Connect two clients
	conn1 := connectWebSocket(t, ts, "cursor-test", "")
	readServerMsg(t, conn1) // Read Identity

	conn2 := connectWebSocket(t, ts, "cursor-test", "")
	readServerMsg(t, conn2) // Read Identity

	// Client 1 sends cursor data
	sendClientMsg(t, conn1, &protocol.ClientMsg{
		CursorData: &protocol.CursorData{
			Cursors:    []uint32{5},
			Selections: [][2]uint32{{0, 5}},
		},
	})

	// Both clients should receive the UserCursor broadcast
	msg1 := readServerMsg(t, conn1)
	msg2 := readServerMsg(t, conn2)

	if msg1.UserCursor == nil {
		t.Fatalf("Client 1 expected UserCursor message, got %+v", msg1)
	}
	if msg2.UserCursor == nil {
		t.Fatalf("Client 2 expected UserCursor message, got %+v", msg2)
	}

	if msg1.UserCursor.ID != 0 {
		t.Errorf("Expected UserID 0, got %d", msg1.UserCursor.ID)
	}
	if len(msg1.UserCursor.Data.Cursors) != 1 || msg1.UserCursor.Data.Cursors[0] != 5 {
		t.Errorf("Expected cursor at position 5, got %v", msg1.UserCursor.Data.Cursors)
	}
}

// TestUserInfoBroadcast tests that user info updates are broadcast.
func TestUserInfoBroadcast(t *testing.T) {
	server := testServer(t)
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Connect two clients
	conn1 := connectWebSocket(t, ts, "userinfo-test", "")
	readServerMsg(t, conn1) // Read Identity

	conn2 := connectWebSocket(t, ts, "userinfo-test", "")
	readServerMsg(t, conn2) // Read Identity

	// Client 1 sends user info
	sendClientMsg(t, conn1, &protocol.ClientMsg{
		ClientInfo: &protocol.UserInfo{
			Name: "TestUser",
			Hue:  180,
		},
	})

	// Both clients should receive the UserInfo broadcast
	msg1 := readServerMsg(t, conn1)
	msg2 := readServerMsg(t, conn2)

	if msg1.UserInfo == nil {
		t.Fatalf("Client 1 expected UserInfo message, got %+v", msg1)
	}
	if msg2.UserInfo == nil {
		t.Fatalf("Client 2 expected UserInfo message, got %+v", msg2)
	}

	if msg1.UserInfo.ID != 0 {
		t.Errorf("Expected UserID 0, got %d", msg1.UserInfo.ID)
	}
	if msg1.UserInfo.Info == nil || msg1.UserInfo.Info.Name != "TestUser" {
		t.Errorf("Expected user name 'TestUser', got %v", msg1.UserInfo.Info)
	}
}

// TestConcurrentEdits tests that concurrent edits from multiple users converge.
func TestConcurrentEdits(t *testing.T) {
	server := testServer(t)
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Connect two clients
	conn1 := connectWebSocket(t, ts, "concurrent-test", "")
	readServerMsg(t, conn1) // Read Identity (user 0)

	conn2 := connectWebSocket(t, ts, "concurrent-test", "")
	readServerMsg(t, conn2) // Read Identity (user 1)

	// Client 1 inserts "hello"
	op1 := ot.NewOperationSeq()
	op1.Insert("hello")
	sendClientMsg(t, conn1, &protocol.ClientMsg{
		Edit: &protocol.EditMsg{
			Revision:  0,
			Operation: op1,
		},
	})

	// Read broadcasts
	readServerMsg(t, conn1) // History for client 1
	readServerMsg(t, conn2) // History for client 2

	// Client 2 inserts " world" at the end
	op2 := ot.NewOperationSeq()
	op2.Retain(5)
	op2.Insert(" world")
	sendClientMsg(t, conn2, &protocol.ClientMsg{
		Edit: &protocol.EditMsg{
			Revision:  1,
			Operation: op2,
		},
	})

	// Read final broadcasts
	readServerMsg(t, conn1)
	readServerMsg(t, conn2)

	// Verify final document text
	if val, ok := server.state.documents.Load("concurrent-test"); ok {
		doc := val.(*Document)
		text := doc.Kolabpad.Text()
		if text != "hello world" {
			t.Errorf("Expected final text 'hello world', got '%s'", text)
		}
	} else {
		t.Fatal("Document not found in server state")
	}
}

// TestStatsEndpoint tests the /api/stats endpoint.
func TestStatsEndpoint(t *testing.T) {
	server := testServer(t)
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Connect a client to create a document
	conn := connectWebSocket(t, ts, "stats-test", "")
	readServerMsg(t, conn) // Read Identity

	// Request stats
	resp, err := http.Get(ts.URL + "/api/stats")
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var stats Stats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("Failed to decode stats: %v", err)
	}

	if stats.NumDocuments != 1 {
		t.Errorf("Expected 1 active document, got %d", stats.NumDocuments)
	}

	if stats.StartTime == 0 {
		t.Error("Expected non-zero start time")
	}
}

// TestServerWithoutDatabase tests that server works without a database.
func TestServerWithoutDatabase(t *testing.T) {
	server := testServerNoDb(t)
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Connect client
	conn := connectWebSocket(t, ts, "no-db-test", "")
	msg := readServerMsg(t, conn)

	if msg.Identity == nil {
		t.Fatalf("Expected Identity message, got %+v", msg)
	}

	// Send an edit
	op := ot.NewOperationSeq()
	op.Insert("test")
	sendClientMsg(t, conn, &protocol.ClientMsg{
		Edit: &protocol.EditMsg{
			Revision:  0,
			Operation: op,
		},
	})

	// Should receive History
	histMsg := readServerMsg(t, conn)
	if histMsg.History == nil {
		t.Fatalf("Expected History message, got %+v", histMsg)
	}

	// Try to protect document (should fail - no DB)
	reqBody := `{"user_id": 0, "user_name": "Test"}`
	resp, err := http.Post(ts.URL+"/api/document/no-db-test/protect", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("Failed to call protect endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 without database, got %d", resp.StatusCode)
	}
}

// TestInvalidDocumentID tests that requests with empty document IDs are rejected.
func TestInvalidDocumentID(t *testing.T) {
	server := testServer(t)
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Try connecting without document ID
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/socket/"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, resp, err := websocket.Dial(ctx, url, nil)
	if err == nil {
		t.Fatal("Expected connection to fail with empty document ID")
	}
	if resp != nil && resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

// TestInvalidRevision tests that edits with invalid revision numbers are rejected.
func TestInvalidRevision(t *testing.T) {
	server := testServer(t)
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Connect client
	conn := connectWebSocket(t, ts, "invalid-rev", "")
	readServerMsg(t, conn) // Read Identity

	// Send edit with future revision
	op := ot.NewOperationSeq()
	op.Insert("test")
	sendClientMsg(t, conn, &protocol.ClientMsg{
		Edit: &protocol.EditMsg{
			Revision:  999, // Invalid future revision
			Operation: op,
		},
	})

	// Connection should be closed due to error
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var msg protocol.ServerMsg
	err := wsjson.Read(ctx, conn, &msg)
	if err == nil {
		t.Error("Expected connection to close due to invalid revision")
	}
}
