package server

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"github.com/shiv/kolabpad/pkg/database"
)

// Document represents a document entry in the server map.
type Document struct {
	LastAccessed time.Time
	Rustpad      *Rustpad
}

// ServerState holds all server-wide state.
type ServerState struct {
	documents sync.Map // map[string]*Document
	startTime time.Time
	db        *database.Database // Optional database
}

// NewServerState creates a new server state.
func NewServerState(db *database.Database) *ServerState {
	return &ServerState{
		startTime: time.Now(),
		db:        db,
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
func NewServer(db *database.Database) *Server {
	s := &Server{
		state: NewServerState(db),
		mux:   http.NewServeMux(),
	}

	// Register routes
	s.mux.HandleFunc("/api/socket/", s.handleSocket)
	s.mux.HandleFunc("/api/text/", s.handleText)
	s.mux.HandleFunc("/api/stats", s.handleStats)

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

	log.Printf("WebSocket connection request for document: %s", docID)

	// Get or create document
	doc := s.getOrCreateDocument(docID)
	doc.LastAccessed = time.Now()

	// Start persister if database is enabled
	if s.state.db != nil {
		go s.persister(r.Context(), docID, doc.Rustpad)
	}

	// Upgrade to WebSocket
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	// Handle connection
	connHandler := NewConnection(doc.Rustpad, conn)
	if err := connHandler.Handle(r.Context()); err != nil {
		log.Printf("Connection error: %v", err)
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
		text := doc.Rustpad.Text()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(text))
		return
	}

	// Try loading from database
	if s.state.db != nil {
		doc, err := s.state.db.Load(docID)
		if err != nil {
			log.Printf("Error loading document from DB: %v", err)
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

// getOrCreateDocument gets an existing document or creates a new one.
func (s *Server) getOrCreateDocument(id string) *Document {
	// Try to load existing
	if val, ok := s.state.documents.Load(id); ok {
		return val.(*Document)
	}

	// Try loading from database
	var rustpad *Rustpad
	if s.state.db != nil {
		if persisted, err := s.state.db.Load(id); err == nil && persisted != nil {
			log.Printf("Loaded document %s from database", id)
			rustpad = FromPersistedDocument(persisted.Text, persisted.Language)
		}
	}

	// Create new document if not in database
	if rustpad == nil {
		rustpad = NewRustpad()
	}

	doc := &Document{
		LastAccessed: time.Now(),
		Rustpad:      rustpad,
	}

	// Store with LoadOrStore to handle race conditions
	actual, _ := s.state.documents.LoadOrStore(id, doc)
	return actual.(*Document)
}

// StartCleaner starts the background document cleanup task.
func (s *Server) StartCleaner(ctx context.Context, expiryDays int) {
	ticker := time.NewTicker(1 * time.Hour)
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
		log.Printf("cleaner removing documents: %v", toDelete)
		for _, id := range toDelete {
			if val, ok := s.state.documents.LoadAndDelete(id); ok {
				doc := val.(*Document)
				doc.Rustpad.Kill()
			}
		}
	}
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe(addr string) error {
	log.Printf("Server listening on %s", addr)
	return http.ListenAndServe(addr, s)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	// Kill all documents
	s.state.documents.Range(func(key, value interface{}) bool {
		doc := value.(*Document)
		doc.Rustpad.Kill()
		return true
	})
	return nil
}

// persister periodically saves a document to the database.
func (s *Server) persister(ctx context.Context, id string, rustpad *Rustpad) {
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
		if rustpad.Killed() {
			return
		}

		// Check if there are new changes
		revision := rustpad.Revision()
		if revision > lastRevision {
			text, language := rustpad.Snapshot()
			doc := &database.PersistedDocument{
				ID:       id,
				Text:     text,
				Language: language,
			}

			log.Printf("persisting revision %d for id = %s", revision, id)
			if err := s.state.db.Store(doc); err != nil {
				log.Printf("error persisting document %s: %v", id, err)
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
