package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"

	"github.com/shiv248/kolabpad/pkg/database"
	"github.com/shiv248/kolabpad/pkg/logger"
)

// Document represents a document entry in the server map.
type Document struct {
	LastAccessed      time.Time
	Kolabpad          *Kolabpad
	persisterCancel   context.CancelFunc // Cancel function to stop persister
	persisterMu       sync.Mutex         // Protects persister start/stop
	connectionCount   int                // Number of active connections
	connectionCountMu sync.Mutex         // Protects connectionCount
}

// ServerState holds all server-wide state.
type ServerState struct {
	documents           sync.Map // map[string]*Document
	startTime           time.Time
	db                  *database.Database // Optional database
	maxDocumentSize     int
	maxMessageSize      int64 // WebSocket message size limit (maxDocumentSize + overhead)
	broadcastBufferSize int
	wsReadTimeout       time.Duration
	wsWriteTimeout      time.Duration
}

// NewServerState creates a new server state.
func NewServerState(db *database.Database, maxDocumentSize, broadcastBufferSize int, wsReadTimeout, wsWriteTimeout time.Duration) *ServerState {
	// Set message size limit to document size + 64KB overhead for JSON encoding
	const overheadBytes = 64 * 1024
	maxMessageSize := int64(maxDocumentSize + overheadBytes)

	return &ServerState{
		startTime:           time.Now(),
		db:                  db,
		maxDocumentSize:     maxDocumentSize,
		maxMessageSize:      maxMessageSize,
		broadcastBufferSize: broadcastBufferSize,
		wsReadTimeout:       wsReadTimeout,
		wsWriteTimeout:      wsWriteTimeout,
	}
}

// Stats represents server statistics.
type Stats struct {
	StartTime    int64 `json:"start_time"`    // Unix timestamp
	NumDocuments int   `json:"num_documents"` // Active documents
	DatabaseSize int   `json:"database_size"` // Documents in database (TODO)
}

// Server is the main HTTP server.
type Server struct {
	state *ServerState
	mux   *http.ServeMux
}

// NewServer creates a new HTTP server.
func NewServer(db *database.Database, maxDocumentSize, broadcastBufferSize int, wsReadTimeout, wsWriteTimeout time.Duration) *Server {
	s := &Server{
		state: NewServerState(db, maxDocumentSize, broadcastBufferSize, wsReadTimeout, wsWriteTimeout),
		mux:   http.NewServeMux(),
	}

	// API routes (must be registered first for priority)
	s.mux.HandleFunc("/api/socket/", s.handleSocket)
	s.mux.HandleFunc("/api/stats", s.handleStats)
	s.mux.HandleFunc("/api/document/", s.handleDocument)

	// Serve frontend static files from dist/
	fs := http.FileServer(http.Dir("./dist"))
	s.mux.Handle("/", fs)

	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// handleSocket handles WebSocket connections for collaborative editing.
// Route: /api/socket/{id}
func (s *Server) handleSocket(w http.ResponseWriter, r *http.Request) {
	// Extract document ID from path
	docID := r.URL.Path[len("/api/socket/"):]
	if docID == "" {
		http.Error(w, "document ID required", http.StatusBadRequest)
		return
	}

	logger.Info("WebSocket connection request for document: %s", docID)

	// Validate OTP with dual-check pattern (prevents DoS)
	providedOTP := r.URL.Query().Get("otp")

	// Fast path: Document already in memory
	if val, ok := s.state.documents.Load(docID); ok {
		doc := val.(*Document)
		if otp := doc.Kolabpad.GetOTP(); otp != nil {
			if providedOTP != *otp {
				http.Error(w, "Invalid or missing OTP", http.StatusUnauthorized)
				logger.Info("Unauthorized access attempt for hot document: %s", docID)
				return
			}
		}
	} else {
		// Slow path: Document not in memory - validate from DB BEFORE loading
		if s.state.db != nil {
			if persisted, err := s.state.db.Load(docID); err == nil && persisted != nil && persisted.OTP != nil {
				if providedOTP != *persisted.OTP {
					http.Error(w, "Invalid or missing OTP", http.StatusUnauthorized)
					logger.Info("Unauthorized access attempt for cold document: %s (prevented DoS)", docID)
					return
				}
			}
		}
	}

	// Get or create document
	doc := s.getOrCreateDocument(docID)
	doc.LastAccessed = time.Now()

	// Track connection count and start persister if needed
	doc.connectionCountMu.Lock()
	doc.connectionCount++
	isFirstConnection := doc.connectionCount == 1
	doc.connectionCountMu.Unlock()

	// Start persister for first connection
	if isFirstConnection && s.state.db != nil {
		doc.persisterMu.Lock()
		ctx, cancel := context.WithCancel(context.Background())
		doc.persisterCancel = cancel
		go s.persister(ctx, docID, doc.Kolabpad)
		doc.persisterMu.Unlock()
		logger.Info("Started persister for document %s (first connection)", docID)
	}

	// Ensure persister is stopped when last connection closes
	defer func() {
		doc.connectionCountMu.Lock()
		doc.connectionCount--
		isLastConnection := doc.connectionCount == 0
		doc.connectionCountMu.Unlock()

		if isLastConnection && s.state.db != nil {
			doc.persisterMu.Lock()
			if doc.persisterCancel != nil {
				// Only flush if document was edited OR has OTP protection
				revision := doc.Kolabpad.Revision()
				otp := doc.Kolabpad.GetOTP()

				if revision > 0 || otp != nil {
					// Flush to DB immediately before stopping
					text, language := doc.Kolabpad.Snapshot()

					if err := s.state.db.Store(&database.PersistedDocument{
						ID:       docID,
						Text:     text,
						Language: language,
						OTP:      otp,
					}); err != nil {
						logger.Error("Failed to flush document %s on last disconnect: %v", docID, err)
					} else {
						logger.Debug("Flushed document %s on last disconnect (revision=%d, protected=%v)", docID, revision, otp != nil)
					}
				} else {
					logger.Debug("Skipping flush for empty unprotected document %s (never edited)", docID)
				}

				// Stop persister
				doc.persisterCancel()
				doc.persisterCancel = nil
				logger.Info("Stopped persister for document %s (last connection closed)", docID)
			}
			doc.persisterMu.Unlock()
		}
	}()

	// Upgrade to WebSocket
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		logger.Error("WebSocket upgrade failed: %v", err)
		return
	}

	// Set message size limit to prevent large message attacks while allowing document-sized operations
	conn.SetReadLimit(s.state.maxMessageSize)

	// Handle connection
	connHandler := NewConnection(doc.Kolabpad, conn, s.state.wsReadTimeout, s.state.wsWriteTimeout)
	if err := connHandler.Handle(r.Context()); err != nil {
		logger.Error("Connection error: %v", err)
	}

	conn.Close(websocket.StatusNormalClosure, "")
}

// handleStats returns server statistics.
// Route: /api/stats
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	// Count active documents
	numDocs := 0
	s.state.documents.Range(func(key, value interface{}) bool {
		numDocs++
		return true
	})

	// Count database documents
	dbSize := 0
	if s.state.db != nil {
		if count, err := s.state.db.Count(); err == nil {
			dbSize = count
		}
	}

	stats := Stats{
		StartTime:    s.state.startTime.Unix(),
		NumDocuments: numDocs,
		DatabaseSize: dbSize,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleDocument handles document protection endpoints.
// Route: /api/document/{id}/protect
func (s *Server) handleDocument(w http.ResponseWriter, r *http.Request) {
	// Parse path to get document ID and action
	path := r.URL.Path[len("/api/document/"):]
	parts := strings.Split(path, "/")

	if len(parts) != 2 || parts[0] == "" || parts[1] != "protect" {
		http.Error(w, "invalid endpoint", http.StatusNotFound)
		return
	}

	docID := parts[0]

	if s.state.db == nil {
		http.Error(w, "database not enabled", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.handleProtectDocument(w, r, docID)
	case http.MethodDelete:
		s.handleUnprotectDocument(w, r, docID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProtectDocument enables OTP protection for a document.
func (s *Server) handleProtectDocument(w http.ResponseWriter, r *http.Request, docID string) {
	// Parse request body to get user info
	var reqBody struct {
		UserID   uint64 `json:"user_id"`
		UserName string `json:"user_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate user is connected to the document
	if val, ok := s.state.documents.Load(docID); ok {
		doc := val.(*Document)
		if !doc.Kolabpad.HasUser(reqBody.UserID) {
			logger.Info("User %d (%s) attempted to protect document %s without being connected", reqBody.UserID, reqBody.UserName, docID)
			http.Error(w, "Forbidden: not connected to document", http.StatusForbidden)
			return
		}
	} else {
		// Document not in memory - user can't be connected
		logger.Info("User %d (%s) attempted to protect non-existent document %s", reqBody.UserID, reqBody.UserName, docID)
		http.Error(w, "Forbidden: not connected to document", http.StatusForbidden)
		return
	}

	// Generate OTP
	otp := GenerateOTP()

	// CRITICAL: Write to DB FIRST (atomicity - prevents memory/DB desync)
	// Check if document exists in DB, if not create it
	doc, err := s.state.db.Load(docID)
	if err != nil {
		logger.Error("Failed to load document: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if doc == nil {
		// Document doesn't exist in DB yet, create it
		doc = &database.PersistedDocument{
			ID:       docID,
			Text:     "",
			Language: nil,
			OTP:      &otp,
		}
		if err := s.state.db.Store(doc); err != nil {
			logger.Error("Failed to store document: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return // DB write failed - do NOT update memory
		}
	} else {
		// Update existing document's OTP
		if err := s.state.db.UpdateOTP(docID, &otp); err != nil {
			logger.Error("Failed to update OTP: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return // DB write failed - do NOT update memory
		}
	}

	logger.Info("Document %s protected with OTP by user %d (%s) (DB write successful)", docID, reqBody.UserID, reqBody.UserName)

	// DB write successful - NOW update memory and broadcast
	if val, ok := s.state.documents.Load(docID); ok {
		doc := val.(*Document)
		doc.Kolabpad.SetOTP(&otp, reqBody.UserID, reqBody.UserName) // Updates memory + broadcasts to clients
	}

	// Return OTP to client
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"otp": otp,
	})
}

// handleUnprotectDocument disables OTP protection for a document.
func (s *Server) handleUnprotectDocument(w http.ResponseWriter, r *http.Request, docID string) {
	// Parse request body to get user info and current OTP
	var reqBody struct {
		UserID   uint64 `json:"user_id"`
		UserName string `json:"user_name"`
		OTP      string `json:"otp"` // Current OTP required for security
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate user is connected to the document
	var doc *Document
	if val, ok := s.state.documents.Load(docID); ok {
		doc = val.(*Document)
		if !doc.Kolabpad.HasUser(reqBody.UserID) {
			logger.Info("User %d (%s) attempted to unprotect document %s without being connected", reqBody.UserID, reqBody.UserName, docID)
			http.Error(w, "Forbidden: not connected to document", http.StatusForbidden)
			return
		}
	} else {
		// Document not in memory - user can't be connected
		logger.Info("User %d (%s) attempted to unprotect non-existent document %s", reqBody.UserID, reqBody.UserName, docID)
		http.Error(w, "Forbidden: not connected to document", http.StatusForbidden)
		return
	}

	// CRITICAL SECURITY: Validate the provided OTP matches the current OTP
	// This prevents anyone who just knows the document ID from disabling protection
	currentOTP := doc.Kolabpad.GetOTP()
	if currentOTP == nil {
		http.Error(w, "document is not OTP-protected", http.StatusBadRequest)
		return
	}
	if reqBody.OTP != *currentOTP {
		logger.Info("User %d (%s) attempted to unprotect document %s with invalid OTP", reqBody.UserID, reqBody.UserName, docID)
		http.Error(w, "Forbidden: invalid OTP", http.StatusForbidden)
		return
	}

	// CRITICAL: Write to DB FIRST (atomicity - prevents memory/DB desync)
	// Remove OTP by setting it to NULL
	if err := s.state.db.UpdateOTP(docID, nil); err != nil {
		logger.Error("Failed to remove OTP: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return // DB write failed - do NOT update memory
	}

	logger.Info("Document %s unprotected by user %d (%s) (OTP removed, DB write successful)", docID, reqBody.UserID, reqBody.UserName)

	// DB write successful - NOW update memory and broadcast
	doc.Kolabpad.SetOTP(nil, reqBody.UserID, reqBody.UserName) // Updates memory + broadcasts to clients

	w.WriteHeader(http.StatusNoContent)
}

// getOrCreateDocument gets an existing document or creates a new one.
func (s *Server) getOrCreateDocument(id string) *Document {
	// Try to load existing
	if val, ok := s.state.documents.Load(id); ok {
		return val.(*Document)
	}

	// Try loading from database
	var kolabpad *Kolabpad
	if s.state.db != nil {
		if persisted, err := s.state.db.Load(id); err == nil && persisted != nil {
			logger.Debug("Loaded document %s from database", id)
			kolabpad = FromPersistedDocument(persisted.Text, persisted.Language, persisted.OTP, s.state.maxDocumentSize, s.state.broadcastBufferSize)
		}
	}

	// Create new document if not in database
	if kolabpad == nil {
		kolabpad = NewKolabpad(s.state.maxDocumentSize, s.state.broadcastBufferSize)
	}

	doc := &Document{
		LastAccessed: time.Now(),
		Kolabpad:     kolabpad,
	}

	// Store with LoadOrStore to handle race conditions
	actual, _ := s.state.documents.LoadOrStore(id, doc)
	return actual.(*Document)
}

// StartCleaner starts the background document cleanup task.
func (s *Server) StartCleaner(ctx context.Context, expiryDays int, cleanupInterval time.Duration) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupExpiredDocuments(expiryDays)
		}
	}
}

// cleanupExpiredDocuments removes documents that haven't been accessed recently.
func (s *Server) cleanupExpiredDocuments(expiryDays int) {
	expiry := time.Duration(expiryDays) * 24 * time.Hour
	now := time.Now()
	var toDelete []string

	s.state.documents.Range(func(key, value interface{}) bool {
		docID := key.(string)
		doc := value.(*Document)

		if now.Sub(doc.LastAccessed) > expiry {
			toDelete = append(toDelete, docID)
		}
		return true
	})

	if len(toDelete) > 0 {
		logger.Debug("cleaner removing %d document(s): %v", len(toDelete), toDelete)

		for _, id := range toDelete {
			if val, ok := s.state.documents.LoadAndDelete(id); ok {
				doc := val.(*Document)

				// Only flush if document was edited OR has OTP protection
				if s.state.db != nil {
					revision := doc.Kolabpad.Revision()
					otp := doc.Kolabpad.GetOTP()

					if revision > 0 || otp != nil {
						text, language := doc.Kolabpad.Snapshot()

						if err := s.state.db.Store(&database.PersistedDocument{
							ID:       id,
							Text:     text,
							Language: language,
							OTP:      otp,
						}); err != nil {
							logger.Error("Failed to flush document %s before eviction: %v", id, err)
						} else {
							logger.Debug("Flushed document %s before eviction (revision=%d, protected=%v)", id, revision, otp != nil)
						}
					} else {
						logger.Debug("Skipping flush for empty unprotected document %s before eviction", id)
					}

					// Stop persister if running
					doc.persisterMu.Lock()
					if doc.persisterCancel != nil {
						doc.persisterCancel()
						doc.persisterCancel = nil
					}
					doc.persisterMu.Unlock()
				}

				// Kill document
				doc.Kolabpad.Kill()
			}
		}
	}
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe(addr string) error {
	logger.Info("Server listening on %s", addr)
	return http.ListenAndServe(addr, s)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.state.db == nil {
		// No database - just kill all documents
		s.state.documents.Range(func(key, value interface{}) bool {
			doc := value.(*Document)
			doc.Kolabpad.Kill()
			return true
		})
		return nil
	}

	logger.Info("Graceful shutdown: flushing all documents to DB")

	// Flush all documents in parallel with timeout
	var wg sync.WaitGroup
	var flushedCount, skippedCount, errorCount int32

	s.state.documents.Range(func(key, value interface{}) bool {
		docID := key.(string)
		doc := value.(*Document)

		wg.Add(1)
		go func(id string, d *Document) {
			defer wg.Done()

			// Only flush if document was edited OR has OTP protection
			revision := d.Kolabpad.Revision()
			otp := d.Kolabpad.GetOTP()

			if revision > 0 || otp != nil {
				// Flush to DB
				text, language := d.Kolabpad.Snapshot()

				if err := s.state.db.Store(&database.PersistedDocument{
					ID:       id,
					Text:     text,
					Language: language,
					OTP:      otp,
				}); err != nil {
					logger.Error("Failed to flush document %s during shutdown: %v", id, err)
					atomic.AddInt32(&errorCount, 1)
				} else {
					logger.Debug("Flushed document %s during shutdown (revision=%d, protected=%v)", id, revision, otp != nil)
					atomic.AddInt32(&flushedCount, 1)
				}
			} else {
				logger.Debug("Skipping flush for empty unprotected document %s during shutdown", id)
				atomic.AddInt32(&skippedCount, 1)
			}

			// Stop persister if running
			d.persisterMu.Lock()
			if d.persisterCancel != nil {
				d.persisterCancel()
				d.persisterCancel = nil
			}
			d.persisterMu.Unlock()
		}(docID, doc)

		return true
	})

	// Wait for all flushes with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("Shutdown flush complete: %d flushed, %d skipped (empty), %d errors", flushedCount, skippedCount, errorCount)
	case <-time.After(10 * time.Second):
		logger.Error("Shutdown timeout after 10s, some documents may not be flushed")
	}

	// Kill all documents
	s.state.documents.Range(func(key, value interface{}) bool {
		doc := value.(*Document)
		doc.Kolabpad.Kill()
		return true
	})

	logger.Info("Shutdown complete")
	return nil
}

// persister periodically saves a document to the database with lazy persistence.
func (s *Server) persister(ctx context.Context, id string, kolabpad *Kolabpad) {
	if s.state.db == nil {
		return
	}

	const persistCheckInterval = 10 * time.Second
	const idleWriteThreshold = 30 * time.Second
	const safetyNetInterval = 5 * time.Minute

	lastPersistedRev := 0
	lastPersistTime := time.Now()

	ticker := time.NewTicker(persistCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("persister for document %s stopped (context cancelled)", id)
			return
		case <-ticker.C:
		}

		// Check if document has been killed
		if kolabpad.Killed() {
			logger.Debug("persister for document %s stopped (document killed)", id)
			return
		}

		// Check if there are new changes
		revision := kolabpad.Revision()
		if revision <= lastPersistedRev {
			continue // No changes since last persist
		}

		// Debounce: Skip if critical write happened recently
		timeSinceCritical := time.Now().Unix() - kolabpad.lastCriticalWrite.Load()
		if timeSinceCritical < 2 {
			logger.Debug("persister skipping for document %s: critical write %ds ago", id, timeSinceCritical)
			continue
		}

		// Check write triggers
		timeSinceEdit := time.Since(kolabpad.LastEditTime())
		timeSincePersist := time.Since(lastPersistTime)

		shouldWrite := false
		reason := ""

		// Trigger 1: Idle threshold
		if timeSinceEdit >= idleWriteThreshold {
			shouldWrite = true
			reason = "idle"
		}

		// Trigger 2: Safety net
		if timeSincePersist >= safetyNetInterval {
			shouldWrite = true
			reason = "safety_net"
		}

		// Write to DB if triggered
		if shouldWrite {
			text, language := kolabpad.Snapshot()
			otp := kolabpad.GetOTP() // Get OTP from memory, not DB

			doc := &database.PersistedDocument{
				ID:       id,
				Text:     text,
				Language: language,
				OTP:      otp,
			}

			logger.Debug("persisting document %s: reason=%s, revision=%d, timeSinceEdit=%v, timeSincePersist=%v",
				id, reason, revision, timeSinceEdit, timeSincePersist)

			if err := s.state.db.Store(doc); err != nil {
				logger.Error("error persisting document %s: %v", id, err)
			} else {
				lastPersistedRev = revision
				lastPersistTime = time.Now()
			}
		}
	}
}

// Example usage:
//
//	db, _ := database.New("kolabpad.db")
//	server := NewServer(db)
//
//	// Start cleanup task
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	go server.StartCleaner(ctx, 1) // 1 day expiry
//
//	// Start server
//	if err := server.ListenAndServe(":3030"); err != nil {
//		log.Fatal(err)
//	}
