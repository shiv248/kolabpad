# Backend Server Architecture

**Purpose**: This document explains the Go backend structure, document lifecycle management, and concurrency model for the Kolabpad collaborative editing server.

**Audience**: Backend developers working on the server implementation or debugging server-side issues.

---

## Table of Contents

1. [Project Structure](#project-structure)
2. [Server Initialization](#server-initialization)
3. [Document Lifecycle](#document-lifecycle)
4. [In-Memory Document State](#in-memory-document-state)
5. [Persister Lifecycle](#persister-lifecycle)
6. [Concurrency Model](#concurrency-model)
7. [Database Layer](#database-layer)
8. [Graceful Shutdown](#graceful-shutdown)

---

## Project Structure

The backend is organized into clean, modular packages:

```
cmd/
├── server/              # Main server entry point
└── ot-wasm-bridge/      # WASM bridge for OT (experimental)

pkg/
├── server/              # HTTP/WebSocket server, document management
│   ├── server.go        # HTTP routes and server lifecycle
│   ├── kolabpad.go      # Document state management
│   ├── connection.go    # WebSocket connection handling
│   └── secret.go        # OTP generation
├── database/            # SQLite persistence layer
│   ├── database.go      # CRUD operations
│   └── migrations.go    # Schema migrations
├── ot/                  # OT algorithm implementation
│   ├── operation.go     # Operation structure
│   ├── transform.go     # Transform algorithm
│   ├── compose.go       # Compose algorithm
│   └── apply.go         # Apply operations to text
├── logger/              # Logging utilities
└── protocol/            # Message format definitions

internal/
└── protocol/            # Internal protocol constants
    ├── messages.go      # WebSocket message types
    └── constants.go     # System constants
```

**Design Decision**: We separate `pkg/` (reusable packages) from `internal/` (internal-only) and `cmd/` (executables). This follows Go's standard project layout and makes it clear which packages can be imported by other projects.

---

## Server Initialization

When the server starts, it follows this sequence:

```pseudocode
FUNCTION main():
    // 1. Initialize logger
    InitializeLogger()

    // 2. Load configuration from environment
    config = LoadConfig():
        port = getEnv("PORT", default="3030")
        expiryDays = getEnvInt("EXPIRY_DAYS", default=7)
        sqliteURI = getEnv("SQLITE_URI")
        cleanupInterval = getEnvInt("CLEANUP_INTERVAL_HOURS", default=1) * hours
        maxDocumentSize = getEnvInt("MAX_DOCUMENT_SIZE_KB", default=256) * 1024
        wsReadTimeout = getEnvInt("WS_READ_TIMEOUT_MINUTES", default=30) * minutes
        wsWriteTimeout = getEnvInt("WS_WRITE_TIMEOUT_SECONDS", default=10) * seconds
        broadcastBufferSize = getEnvInt("BROADCAST_BUFFER_SIZE", default=16)

    // 3. Initialize database (optional)
    IF config.sqliteURI is set:
        database = OpenSQLiteDatabase(config.sqliteURI)
        RunMigrations(database)
    ELSE:
        database = null  // In-memory only mode

    // 4. Create server instance
    server = NewServer(database, config)

    // 5. Start background cleanup task
    context = CreateCancellableContext()
    START_BACKGROUND StartCleaner(context, config.expiryDays, config.cleanupInterval)

    // 6. Setup graceful shutdown handler
    ON_SIGNAL(SIGTERM, SIGINT):
        CancelContext(context)
        server.Shutdown(context)
        Exit(0)

    // 7. Start listening for HTTP/WebSocket connections
    server.ListenAndServe(":" + config.port)
```

**Configuration via Environment Variables**:

```bash
PORT=3030                      # HTTP server port
EXPIRY_DAYS=7                  # Document expiry after last access
SQLITE_URI=./data/kolabpad.db  # Database file path (optional)
CLEANUP_INTERVAL_HOURS=1       # How often to run cleanup
MAX_DOCUMENT_SIZE_KB=256       # Maximum document size (in KB)
WS_READ_TIMEOUT_MINUTES=30     # WebSocket read timeout
WS_WRITE_TIMEOUT_SECONDS=10    # WebSocket write timeout
BROADCAST_BUFFER_SIZE=16       # Channel buffer for broadcasts
```

**Design Decision**: We use environment variables for configuration instead of config files because it's simpler for containerized deployments (Docker, Kubernetes) and follows the [12-factor app methodology](https://12factor.net/config).

---

## Document Lifecycle

The document lifecycle consists of four phases: **cold start**, **hot (active)**, **idle**, and **eviction**.

### Cold Start: Loading or Creating

When a user connects to a document that's not in memory:

```pseudocode
FUNCTION handleWebSocketConnection(documentId, otp):
    // 1. OTP Validation (dual-check pattern - prevents DoS)
    IF document = activeDocuments.get(documentId):
        // HOT PATH: Document in memory
        IF document.hasOTP() AND document.getOTP() != otp:
            REJECT "Invalid OTP" (401 Unauthorized)
            LOG "Unauthorized access attempt for hot document"
            RETURN
    ELSE:
        // COLD PATH: Document not in memory
        IF database is not null:
            persisted = database.Load(documentId)

            // CRITICAL SECURITY: Check OTP BEFORE loading into memory
            IF persisted exists AND persisted.hasOTP() AND persisted.OTP != otp:
                REJECT "Invalid OTP" (401 Unauthorized)
                LOG "Unauthorized access attempt for cold document (prevented DoS)"
                RETURN  // Don't load document into memory!

    // 2. Get or create document
    document = getOrCreateDocument(documentId)
    document.lastAccessed = now()

    // 3. Assign user ID
    userId = document.nextUserId()  // Atomic counter: 0, 1, 2, ...

    // 4. Track connection count
    document.incrementConnectionCount()
    isFirstConnection = (document.connectionCount == 1)

    // 5. Start persister if first connection
    IF isFirstConnection AND database is not null:
        context, cancelFunc = CreateCancellableContext()
        document.persisterCancel = cancelFunc
        START_BACKGROUND persister(context, documentId, document)
        LOG "Started persister for document (first connection)"

    // 6. Upgrade to WebSocket
    connection = UpgradeToWebSocket()
    SetMessageSizeLimit(connection, maxDocumentSize + 64KB)

    // 7. Send initial state to client
    SendInitialState(connection, document, userId)

    // 8. Handle messages in loop
    WHILE connection is open:
        message = ReceiveMessage(connection)
        HandleMessage(message, connection, document, userId)

    // 9. Cleanup on disconnect
    OnDisconnect(error):
        // Log disconnect based on reason
        IF error is null:
            LOG_INFO "User disconnected"
        ELSE IF error is normal closure (StatusNormalClosure OR StatusGoingAway):
            LOG_INFO "User disconnected"
        ELSE:
            LOG_WARN "User disconnected forcefully"
            LOG_ERROR "Disconnect reason: %v", error

        document.decrementConnectionCount()
        isLastConnection = (document.connectionCount == 0)

        IF isLastConnection AND database is not null:
            // Last user disconnecting - flush and stop persister
            IF document was edited OR document has OTP:
                FlushToDatabase(document)  // Immediate flush
                LOG "Flushed document on last disconnect"

            IF document.persisterCancel is not null:
                document.persisterCancel()  // Stop background persister
                document.persisterCancel = null
                LOG "Stopped persister (last connection closed)"
```

**Why the Dual-Check OTP Pattern?**

The dual-check pattern protects against a DoS attack:

```pseudocode
// OLD VULNERABLE APPROACH:
Connect to protected document with wrong OTP
→ Load entire document into memory (parse operations, reconstruct text)
→ Validate OTP
→ Reject connection
→ Document stays in memory
→ Attacker repeats 1000 times = 1000 documents in memory = OOM

// NEW SECURE APPROACH:
Connect to protected document with wrong OTP
→ Query database for OTP only (lightweight SQL query)
→ Validate OTP
→ Reject WITHOUT loading document into memory
→ No memory exhaustion possible
```

**Design Decision**: We explicitly track connection count instead of relying on subscribers map length because subscribers are used for metadata broadcasts, not all connections. The connection count gives us precise control over when to start/stop the persister.

**Disconnect Logging Strategy**: The cleanup function differentiates between normal and abnormal disconnects by inspecting the error status. Normal closures (initiated by client or graceful shutdown) are logged at INFO level, while forceful disconnects (network failures, timeouts, crashes) are logged at WARN level with detailed error information. This helps operators distinguish between expected connection churn and potential network or client issues.

### Hot (Active): In-Memory with Active Users

When users are actively editing:

```pseudocode
HOT DOCUMENT STATE:
    - Document lives in memory (State struct)
    - All operations stored in memory
    - Persister running in background (checks every 10 seconds)
    - Changes broadcast to all connected users via WebSocket
    - Database writes happen lazily (30s idle or 5min safety net)
```

### Idle: No Users but Still Cached

When all users disconnect:

```pseudocode
IDLE DOCUMENT STATE:
    - Document still in memory (for fast reconnection)
    - Persister STOPPED (no background task running)
    - No active WebSocket connections
    - Last accessed timestamp tracked
    - If users reconnect within expiry window: HOT immediately
    - If no reconnection after expiryDays: EVICTION
```

**Why Keep Idle Documents in Memory?**

Users often disconnect and reconnect within minutes (e.g., page refresh, network hiccup). Keeping the document in memory provides instant reconnection without database reads. The trade-off is memory usage, but documents are small (average ~10KB, max 256KB).

### Eviction: Cleanup After Expiry

The cleanup task runs periodically (default: every 1 hour):

```pseudocode
FUNCTION cleanupExpiredDocuments(expiryDays):
    expiryThreshold = now() - (expiryDays * 24 * hours)
    documentsToDelete = []

    FOR EACH document IN activeDocuments:
        IF document.lastAccessed < expiryThreshold:
            documentsToDelete.append(document)

    FOR EACH document IN documentsToDelete:
        // Flush to database if edited or protected
        IF database is not null:
            IF document was edited OR document has OTP:
                FlushToDatabase(document)
                LOG "Flushed document before eviction"
            ELSE:
                LOG "Skipping flush for empty unprotected document"

        // Stop persister if running (shouldn't be, but defensive)
        IF document.persisterCancel is not null:
            document.persisterCancel()

        // Kill document (closes all channels, wakes connections)
        document.Kill()

        // Remove from memory
        activeDocuments.Remove(document.id)
        LOG "Evicted document from memory"
```

**Design Decision**: We use a pull-based cleanup model (periodic scan) instead of push-based (timers per document) because it's simpler and more efficient. With thousands of documents, we'd need thousands of timers. A single background task that scans every hour is sufficient and uses fewer resources.

---

## In-Memory Document State

Each document in memory consists of two main structures:

### Document Wrapper

```pseudocode
Document {
    lastAccessed: timestamp              // For eviction tracking
    kolabpad: *Kolabpad                  // The collaborative state
    persisterCancel: function            // Function to stop background persister
    persisterMutex: Mutex                // Protects persister start/stop
    connectionCount: int                 // Number of active WebSocket connections
    connectionCountMutex: Mutex          // Protects connectionCount
}
```

### Kolabpad State (The Core)

```pseudocode
Kolabpad {
    state: {
        operations: []UserOperation      // Complete OT history for active session
        text: string                     // Current document text (UTF-8)
        language: optional string        // Syntax highlighting language
        otp: optional string             // Access protection token (IN MEMORY!)
        users: Map<userId → UserInfo>    // Connected users (name, color hue)
        cursors: Map<userId → CursorData> // User cursor positions/selections
    }

    mutex: RWMutex                       // Protects entire state
    userIdCounter: AtomicUint64          // Generates unique user IDs: 0, 1, 2, ...
    killed: AtomicBool                   // Document destruction flag
    lastEditTime: AtomicInt64            // Unix timestamp of last edit (for idle detection)
    lastPersistedRevision: AtomicInt32   // Last revision written to DB
    lastCriticalWrite: AtomicInt64       // Unix timestamp of last critical write (OTP)

    subscribers: Map<userId → channel>   // Broadcast channels for metadata updates
    notify: channel                      // Closed/recreated to signal new operations

    maxDocumentSize: int                 // Size limit (default: 256KB)
    broadcastBufferSize: int             // Channel buffer size (default: 16)
}
```

**Actual Go Implementation**:

```go
// From pkg/server/kolabpad.go
type State struct {
    Operations []protocol.UserOperation
    Text       string
    Language   *string
    OTP        *string
    Users      map[uint64]protocol.UserInfo
    Cursors    map[uint64]protocol.CursorData
}

type Kolabpad struct {
    state                *State
    mu                   sync.RWMutex
    count                atomic.Uint64
    killed               atomic.Bool
    lastEditTime         atomic.Int64
    lastPersistedRevision atomic.Int32
    lastCriticalWrite    atomic.Int64
    subscribers          map[uint64]chan *protocol.ServerMsg
    notify               chan struct{}
    maxDocumentSize      int
    broadcastBufferSize  int
}
```

**Key Design Decisions**:

1. **OTP in Memory**: The OTP is stored in memory (not just in database) so it can be checked on the hot path without a database query. This is critical for performance.

2. **Atomic Fields**: `lastEditTime`, `lastPersistedRevision`, and `lastCriticalWrite` are atomic integers because they're read by the persister goroutine without holding the mutex. This avoids lock contention.

3. **Notify Channel Pattern**: Instead of broadcasting operations through channels (which would require buffering), we close and recreate a channel. All connections waiting on `<-notify` wake up immediately and check for new operations. This is Go's idiomatic way to wake multiple goroutines.

4. **Subscriber Channels**: Metadata updates (language, OTP, user info, cursors) use per-connection channels with a buffer. This allows non-blocking sends—if a slow client's buffer is full, we skip the send rather than blocking all broadcasts.

---

## Persister Lifecycle

The persister is a background goroutine that lazily writes document changes to the database. One persister runs per active document (when users are connected).

### Persister Algorithm

```pseudocode
FUNCTION persister(context, documentId, kolabpad):
    IF database is null:
        RETURN  // No persistence

    CONSTANTS:
        persistCheckInterval = 10 seconds       // How often to check
        idleWriteThreshold = 30 seconds        // Idle trigger
        safetyNetInterval = 5 minutes          // Safety net trigger

    lastPersistedRevision = 0
    lastPersistTime = now()

    ticker = CreateTicker(persistCheckInterval)

    LOOP:
        SELECT:
            CASE context is cancelled:
                LOG "Persister stopped (context cancelled)"
                RETURN

            CASE ticker fires (every 10 seconds):
                // Check if document has been killed
                IF kolabpad.killed:
                    LOG "Persister stopped (document killed)"
                    RETURN

                // Check if there are new changes
                currentRevision = kolabpad.Revision()
                IF currentRevision <= lastPersistedRevision:
                    CONTINUE  // No changes since last persist

                // Debounce: Skip if critical write happened recently (< 2 seconds)
                // This prevents double-writing when OTP changes (which do immediate writes)
                timeSinceCritical = now() - kolabpad.lastCriticalWrite
                IF timeSinceCritical < 2 seconds:
                    LOG "Persister skipping: critical write %ds ago"
                    CONTINUE

                // Check write triggers
                timeSinceEdit = now() - kolabpad.lastEditTime
                timeSincePersist = now() - lastPersistTime

                shouldWrite = FALSE
                reason = ""

                // TRIGGER 1: Idle threshold (user stopped typing)
                IF timeSinceEdit >= idleWriteThreshold:
                    shouldWrite = TRUE
                    reason = "idle"

                // TRIGGER 2: Safety net (force write even if actively editing)
                IF timeSincePersist >= safetyNetInterval:
                    shouldWrite = TRUE
                    reason = "safety_net"

                // Write to database if triggered
                IF shouldWrite:
                    text, language = kolabpad.Snapshot()
                    otp = kolabpad.GetOTP()

                    document = PersistedDocument{
                        id: documentId,
                        text: text,
                        language: language,
                        otp: otp
                    }

                    LOG "Persisting: reason=%s, revision=%d, timeSinceEdit=%v, timeSincePersist=%v"

                    TRY:
                        database.Store(document)
                        lastPersistedRevision = currentRevision
                        lastPersistTime = now()
                    CATCH error:
                        LOG_ERROR "Failed to persist: %v", error
```

**Why These Thresholds?**

- **10 second check interval**: Balances responsiveness with CPU usage. Checking every second would waste CPU; checking every minute would delay persistence too much.

- **30 second idle threshold**: If a user stops typing for 30 seconds, it's likely they're done with their thought. Write to disk for safety.

- **5 minute safety net**: Even if a user is actively typing, force a write every 5 minutes. This bounds the maximum data loss on a crash to 5 minutes.

- **2 second critical write debounce**: OTP changes trigger immediate database writes (outside the persister). The persister skips the next cycle to avoid redundant writes.

**Design Decision**: We use a lazy persistence strategy instead of writing on every edit because database writes are expensive (disk I/O). Writing every keystroke would:
1. Overwhelm the disk with writes
2. Reduce SSD lifespan (write amplification)
3. Increase latency for users

The lazy strategy provides the same user experience (instant collaboration) while dramatically reducing database load. For more details, see [architecture/03-persistence-strategy.md].

---

## Concurrency Model

The server uses Go's concurrency primitives to safely handle multiple documents and users.

### Thread-Safe Data Structures

```pseudocode
ServerState:
    documents: sync.Map<documentId → *Document>  // Thread-safe map

    WHY sync.Map:
        - Supports concurrent reads and writes without locks
        - Optimized for:
          * Many reads, few writes (common case: documents loaded once)
          * Disjoint key sets (different goroutines access different documents)
```

### Locking Strategy

**Document-Level Locking**:
```pseudocode
Kolabpad:
    mu: RWMutex  // Read-write mutex for state

    READ OPERATIONS (allow concurrent readers):
        - Revision()
        - Text()
        - Snapshot()
        - GetOTP()
        - GetInitialState()
        - GetHistory()

    WRITE OPERATIONS (exclusive lock):
        - ApplyEdit()
        - SetLanguage()
        - SetOTP()
        - SetUserInfo()
        - SetCursorData()
        - RemoveUser()
```

**Why RWMutex?**

Read operations are much more common than write operations (e.g., checking revision, getting OTP for validation). An RWMutex allows multiple concurrent readers, only blocking for writers. This improves throughput significantly.

### Goroutines Per Document

```pseudocode
FOR EACH active document:
    1 goroutine: persister (background DB writes)
    N goroutines: 1 per WebSocket connection (message handling)
    N goroutines: 1 per connection (broadcast forwarder)
```

**Example**: 100 active documents with average 3 users per document
- 100 persister goroutines
- 300 connection handler goroutines
- 300 broadcast forwarder goroutines
- Total: ~700 goroutines

**Why This Is Efficient**: Go's goroutines are lightweight (2KB stack initially) and scheduled cooperatively. 700 goroutines use < 2MB of memory and have negligible overhead.

### Atomic Operations

```pseudocode
Kolabpad:
    count: AtomicUint64            // User ID counter
    killed: AtomicBool              // Document killed flag
    lastEditTime: AtomicInt64       // Last edit timestamp
    lastPersistedRevision: AtomicInt32  // Last persisted revision
    lastCriticalWrite: AtomicInt64  // Last critical write timestamp

WHY Atomic:
    These fields are read by multiple goroutines (persister + connections)
    without holding the mutex. Atomic operations provide lock-free access
    with guaranteed memory visibility.
```

### Broadcast Channels

```pseudocode
BROADCAST PATTERN:

subscribers: Map<userId → buffered channel>

FUNCTION broadcast(message):
    FOR EACH channel IN subscribers:
        SELECT:
            CASE channel <- message:
                // Sent successfully
            DEFAULT:
                // Channel full - skip this client
                // WHY: Don't block entire broadcast for one slow client
```

**Design Decision**: Channels have a buffer (default: 16 messages). If a client is too slow to consume broadcasts (e.g., slow network), their channel fills up. We skip sending to them rather than blocking all other clients. The slow client will eventually be disconnected by a write timeout.

### Operation Notification (Wake All Pattern)

```pseudocode
notify: channel  // Closed to signal new operations

FUNCTION ApplyEdit():
    // ... apply operation to state

    // Wake all connections
    close(notify)
    notify = make(new channel)  // Create new channel for next wake

CONNECTION LOOP:
    notifyChannel = kolabpad.NotifyChannel()

    SELECT:
        CASE <-notifyChannel:
            // Channel closed - new operation available
            SendHistory(currentRevision)
```

**Why Close-and-Recreate?**

Closing a channel wakes all goroutines waiting on `<-chan`. This is Go's idiomatic pattern for waking multiple goroutines. The alternative (sending to N channels) would require knowing all connections and would be slower.

---

## Database Layer

The database is optional and uses SQLite for simplicity.

### Schema

```sql
-- From pkg/database/migrations.go
CREATE TABLE IF NOT EXISTS document (
    id TEXT PRIMARY KEY,
    text TEXT NOT NULL,
    language TEXT,
    otp TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Operations

```pseudocode
INTERFACE Database:
    Load(documentId) → PersistedDocument or null
    Store(document) → error or success
    Count() → int (number of documents)
    Delete(documentId) → error or success
    UpdateOTP(documentId, otp) → error or success
```

**Actual Go Implementation**:

```go
// From pkg/database/database.go
type PersistedDocument struct {
    ID       string
    Text     string
    Language *string
    OTP      *string
}

func (d *Database) Store(doc *PersistedDocument) error {
    query := `
    INSERT INTO document (id, text, language, otp)
    VALUES (?, ?, ?, ?)
    ON CONFLICT(id) DO UPDATE SET
        text = excluded.text,
        language = excluded.language,
        otp = excluded.otp
    `
    // ... execute query
}
```

### Why SQLite?

**Advantages**:
- Single file, zero configuration
- Embedded (no separate database process)
- ACID transactions
- Fast for read-heavy workloads (our use case)
- Easy deployment (just copy the .db file)

**Trade-offs**:
- Single writer (limits write concurrency)
- Not suitable for distributed systems (no network access)
- Limited scalability (good to ~1M documents)

**Design Decision**: SQLite is sufficient for Kolabpad's ephemeral collaboration model. We prioritize simplicity over scalability. If scaling beyond a single server is needed, we'd switch to PostgreSQL or Redis. See [architecture/03-persistence-strategy.md] for scaling discussion.

### Thread Safety

SQLite is thread-safe for concurrent reads. Writes are serialized automatically by SQLite's locking. Go's `database/sql` package handles connection pooling safely.

---

## Graceful Shutdown

When the server receives SIGTERM or SIGINT, it gracefully shuts down to prevent data loss.

### Shutdown Sequence

```pseudocode
FUNCTION Shutdown(context):
    IF database is null:
        // No persistence - just kill all documents
        FOR EACH document IN activeDocuments:
            document.Kill()
        RETURN

    LOG "Graceful shutdown: flushing all documents to DB"

    flushedCount = 0
    skippedCount = 0
    errorCount = 0

    // Flush all documents in PARALLEL (fast shutdown)
    waitGroup = CreateWaitGroup()

    FOR EACH documentId, document IN activeDocuments:
        waitGroup.Add(1)

        START_GOROUTINE:
            DEFER waitGroup.Done()

            // Only flush if edited or protected
            revision = document.Revision()
            otp = document.GetOTP()

            IF revision > 0 OR otp is not null:
                // Flush to database
                text, language = document.Snapshot()

                persisted = PersistedDocument{
                    id: documentId,
                    text: text,
                    language: language,
                    otp: otp
                }

                TRY:
                    database.Store(persisted)
                    LOG "Flushed document during shutdown (revision=%d, protected=%v)"
                    AtomicIncrement(flushedCount)
                CATCH error:
                    LOG_ERROR "Failed to flush document during shutdown: %v"
                    AtomicIncrement(errorCount)
            ELSE:
                LOG "Skipping flush for empty unprotected document during shutdown"
                AtomicIncrement(skippedCount)

            // Stop persister if running
            IF document.persisterCancel is not null:
                document.persisterCancel()
                document.persisterCancel = null

    // Wait for all flushes with timeout
    done = CreateChannel()
    START_GOROUTINE:
        waitGroup.Wait()
        Close(done)

    SELECT:
        CASE <-done:
            LOG "Shutdown flush complete: %d flushed, %d skipped, %d errors"
        CASE <-Timeout(10 seconds):
            LOG_ERROR "Shutdown timeout after 10s, some documents may not be flushed"

    // Kill all documents (close channels, disconnect clients)
    FOR EACH document IN activeDocuments:
        document.Kill()

    LOG "Shutdown complete"
```

**Why Parallel Flushing?**

With 1000 active documents and 10ms per database write, sequential flushing would take 10 seconds. Parallel flushing with 100 concurrent writes takes ~100ms (100x faster). SQLite handles concurrent writes by serializing them internally, but the Go goroutines don't block each other.

**Why 10 Second Timeout?**

Most cloud platforms (Kubernetes, Docker, systemd) send SIGTERM, wait 30 seconds, then send SIGKILL. We use 10 seconds to flush documents, leaving 20 seconds buffer for the shutdown to complete fully. If the timeout expires, we log an error but exit anyway—the alternative (blocking forever) would prevent restarts.

**Design Decision**: We flush all documents on graceful shutdown, even if they were recently persisted. This ensures zero data loss during deployments. The cost is a few extra database writes, which is acceptable during the rare event of a restart.

---

## Related Documentation

- [architecture/01-system-overview.md] - High-level system overview
- [architecture/02-operational-transformation.md] - OT algorithm details
- [architecture/03-persistence-strategy.md] - Full persistence strategy and monitoring
- [backend/02-broadcast-system.md] - How server broadcasts state changes
- [protocol/01-websocket-protocol.md] - WebSocket message format
- [security/01-authentication-model.md] - OTP validation and security

---

## Summary

The Kolabpad backend is designed for simplicity and efficiency:

1. **Modular structure**: Clean separation between server logic, database, OT algorithm, and protocols
2. **Memory-first**: Active documents live in RAM for instant access
3. **Lazy persistence**: Database writes only when idle or safety net triggers
4. **Fine-grained concurrency**: Document-level locking, atomic operations, and goroutines for parallelism
5. **Graceful degradation**: Slow clients don't block fast clients; optional database enables in-memory-only mode
6. **Zero-data-loss shutdown**: Parallel flushing ensures all documents are saved on graceful restarts

The architecture balances performance (memory-first, lazy writes) with safety (idle detection, safety nets, graceful shutdown) to provide a fast, reliable collaborative editing experience.
