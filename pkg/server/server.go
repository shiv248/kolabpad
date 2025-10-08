package server

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"github.com/shiv248/kolabpad/pkg/database"
	"github.com/shiv248/kolabpad/pkg/logger"
)

// Document represents a document entry in the server map.
type Document struct {
	LastAccessed time.Time
	Kolabpad      *Kolabpad
}

// ServerState holds all server-wide state.
type ServerState struct {
	documents           sync.Map // map[string]*Document
	startTime           time.Time
	db                  *database.Database // Optional database
	maxDocumentSize     int
	broadcastBufferSize int
	wsReadTimeout       time.Duration
	wsWriteTimeout      time.Duration
}

// NewServerState creates a new server state.
func NewServerState(db *database.Database, maxDocumentSize, broadcastBufferSize int, wsReadTimeout, wsWriteTimeout time.Duration) *ServerState {
	return &ServerState{
		startTime:           time.Now(),
		db:                  db,
		maxDocumentSize:     maxDocumentSize,
		broadcastBufferSize: broadcastBufferSize,
		wsReadTimeout:       wsReadTimeout,
		wsWriteTimeout:      wsWriteTimeout,
	}
}

// Stats represents server statistics.
type Stats struct {
	StartTime    int64 `json:"start_time"`     // Unix timestamp
	NumDocuments int   `json:"num_documents"`  // Active documents
	DatabaseSize int   `json:"database_size"`  // Documents in database (TODO)
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
	s.mux.HandleFunc("/api/text/", s.handleText)
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

	// Validate OTP if document is protected
	if s.state.db != nil {
		if persisted, err := s.state.db.Load(docID); err == nil && persisted != nil && persisted.OTP != nil {
			providedOTP := r.URL.Query().Get("otp")
			if providedOTP != *persisted.OTP {
				http.Error(w, "Invalid or missing OTP", http.StatusUnauthorized)
				logger.Info("Unauthorized access attempt for document: %s", docID)
				return
			}
		}
	}

	// Get or create document
	doc := s.getOrCreateDocument(docID)
	doc.LastAccessed = time.Now()

	// Start persister if database is enabled
	if s.state.db != nil {
		go s.persister(r.Context(), docID, doc.Kolabpad)
	}

	// Upgrade to WebSocket
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		logger.Error("WebSocket upgrade failed: %v", err)
		return
	}

	// Handle connection
	connHandler := NewConnection(doc.Kolabpad, conn, s.state.wsReadTimeout, s.state.wsWriteTimeout)
	if err := connHandler.Handle(r.Context()); err != nil {
		logger.Error("Connection error: %v", err)
	}

	conn.Close(websocket.StatusNormalClosure, "")
}

// handleText returns the current document text.
// Route: /api/text/{id}
func (s *Server) handleText(w http.ResponseWriter, r *http.Request) {
	docID := r.URL.Path[len("/api/text/"):]
	if docID == "" {
		http.Error(w, "document ID required", http.StatusBadRequest)
		return
	}

	// Check if document exists in memory
	if val, ok := s.state.documents.Load(docID); ok {
		doc := val.(*Document)
		text := doc.Kolabpad.Text()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(text))
		return
	}

	// Try loading from database
	if s.state.db != nil {
		doc, err := s.state.db.Load(docID)
		if err != nil {
			logger.Error("Error loading document from DB: %v", err)
		} else if doc != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte(doc.Text))
			return
		}
	}

	// Document doesn't exist, return empty
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(""))
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
	// Generate OTP
	otp := GenerateOTP()

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
			return
		}
	} else {
		// Update existing document's OTP
		if err := s.state.db.UpdateOTP(docID, &otp); err != nil {
			logger.Error("Failed to update OTP: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	logger.Info("Document %s protected with OTP", docID)

	// Broadcast OTP to all connected clients
	if val, ok := s.state.documents.Load(docID); ok {
		doc := val.(*Document)
		doc.Kolabpad.SetOTP(&otp)
	}

	// Return OTP to client
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"otp": otp,
	})
}

// handleUnprotectDocument disables OTP protection for a document.
func (s *Server) handleUnprotectDocument(w http.ResponseWriter, r *http.Request, docID string) {
	// Remove OTP by setting it to NULL
	if err := s.state.db.UpdateOTP(docID, nil); err != nil {
		logger.Error("Failed to remove OTP: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	logger.Info("Document %s unprotected (OTP removed)", docID)

	// Broadcast OTP removal to all connected clients
	if val, ok := s.state.documents.Load(docID); ok {
		doc := val.(*Document)
		doc.Kolabpad.SetOTP(nil)
	}

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
			kolabpad = FromPersistedDocument(persisted.Text, persisted.Language, s.state.maxDocumentSize, s.state.broadcastBufferSize)
		}
	}

	// Create new document if not in database
	if kolabpad == nil {
		kolabpad = NewKolabpad(s.state.maxDocumentSize, s.state.broadcastBufferSize)
	}

	doc := &Document{
		LastAccessed: time.Now(),
		Kolabpad:      kolabpad,
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
		logger.Debug("cleaner removing documents: %v", toDelete)
		for _, id := range toDelete {
			if val, ok := s.state.documents.LoadAndDelete(id); ok {
				doc := val.(*Document)
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
	// Kill all documents
	s.state.documents.Range(func(key, value interface{}) bool {
		doc := value.(*Document)
		doc.Kolabpad.Kill()
		return true
	})
	return nil
}

// persister periodically saves a document to the database.
func (s *Server) persister(ctx context.Context, id string, kolabpad *Kolabpad) {
	if s.state.db == nil {
		return
	}

	const persistInterval = 3 * time.Second
	const persistJitter = 1 * time.Second

	lastRevision := 0

	for {
		// Add random jitter to avoid thundering herd
		jitter := time.Duration(rand.Int63n(int64(persistJitter)))
		select {
		case <-ctx.Done():
			return
		case <-time.After(persistInterval + jitter):
		}

		// Check if document has been killed
		if kolabpad.Killed() {
			return
		}

		// Check if there are new changes
		revision := kolabpad.Revision()
		if revision > lastRevision {
			text, language := kolabpad.Snapshot()

			// Load existing document to preserve OTP
			var otp *string
			if existing, err := s.state.db.Load(id); err == nil && existing != nil {
				otp = existing.OTP
			}

			doc := &database.PersistedDocument{
				ID:       id,
				Text:     text,
				Language: language,
				OTP:      otp,
			}

			logger.Debug("persisting revision %d for id = %s", revision, id)
			if err := s.state.db.Store(doc); err != nil {
				logger.Error("error persisting document %s: %v", id, err)
			} else {
				lastRevision = revision
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
