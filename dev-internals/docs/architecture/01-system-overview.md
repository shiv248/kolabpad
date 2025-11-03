# System Overview

**Purpose**: This document provides a high-level introduction to Kolabpad's architecture for new developers. It serves as the entry point for understanding the system before diving into specific components.

**Audience**: New contributors, system architects, and anyone seeking to understand how Kolabpad works.

---

## What is Kolabpad?

Kolabpad is a real-time collaborative text editor designed for ephemeral, short-term document sharing. It was originally forked from Rustpad but has been completely migrated to a Go backend and TypeScript + Vite frontend.

### Key Characteristics

- **Real-time collaboration**: Multiple users can edit the same document simultaneously with instant synchronization
- **Operational Transformation**: Uses OT algorithm to resolve concurrent edits (not CRDTs)
- **Ephemeral by design**: Documents are temporary, not meant for long-term storage
- **Memory-first architecture**: Active documents live in RAM for performance
- **Lazy persistence**: Database writes are deferred and batched for efficiency
- **No user accounts**: Simple, anonymous collaboration with optional document protection

### Primary Use Cases

Kolabpad excels at:
- Sharing code snippets during pair programming
- Quick collaborative note-taking during meetings
- Sharing configuration files (.env files, configs) with teammates
- Temporary document collaboration (less than 7 days)

Kolabpad is **not** designed for:
- Long-term document storage
- Compliance-sensitive data (HIPAA, PCI-DSS)
- Mission-critical documents (acceptable data loss window: 30s-5min on crashes)
- High-availability requirements across regions

---

## Core Components

### 1. Go Backend

**Location**: `cmd/server/`, `pkg/server/`, `pkg/database/`

The backend handles:
- WebSocket server for real-time communication
- Operational Transformation engine for conflict resolution
- Document state management (in-memory)
- SQLite persistence layer (backup and recovery)
- OTP-based document protection

**Technology**: Go 1.21+, nhooyr.io/websocket library, SQLite3

### 2. TypeScript Frontend

**Location**: `frontend/src/`

The frontend provides:
- React-based user interface
- Monaco editor integration (VS Code's editor engine)
- WebSocket client for real-time sync
- State management using React Context + custom hooks
- REST API client for administrative operations

**Technology**: TypeScript, React, Vite, Monaco Editor

### 3. SQLite Database

**Location**: `pkg/database/`

The database serves as:
- **Backup storage** (not source of truth)
- **Cold-start recovery** (loading documents not in memory)
- **Persistence layer** for lazy writes

**Schema**: Documents table with columns: `id`, `text`, `language`, `otp`, `created_at`, `updated_at`

**Why SQLite**: Simple deployment (single file), sufficient for ephemeral collaboration, no setup required

### 4. WebSocket Protocol

**Location**: `internal/protocol/`

Provides bidirectional real-time communication:
- Client → Server: Edit operations, language changes, cursor updates, user info
- Server → Client: Operation history, broadcasts, user join/leave events

See [../protocol/01-websocket-protocol.md] for complete protocol specification.

---

## Key Design Principles

### 1. Memory-First Architecture

**Principle**: Active documents are stored entirely in RAM. The database is a backup, not the source of truth.

**Rationale**:
- **Performance**: Reading from memory is 1000x faster than disk
- **Simplicity**: No cache invalidation complexity
- **Consistency**: Single source of truth (server memory) eliminates sync issues

**Trade-off**: Documents are lost on server crash (mitigated by lazy persistence)

### 2. CP Consistency (from CAP Theorem)

**Principle**: Kolabpad prioritizes Consistency and Partition-tolerance over Availability.

**Rationale**:
- Single server design means no network partitions between nodes
- Server is the single authority for all operations
- All clients converge to identical state deterministically

**Trade-off**: Single point of failure (no multi-region failover)

### 3. Ephemeral Collaboration

**Principle**: Documents are temporary and may be lost on crashes.

**Acceptable data loss**: 30 seconds to 5 minutes on server crash (lazy persistence window)

**Rationale**:
- Target use case is short-term collaboration, not permanent storage
- Users can re-paste content if lost
- Simplifies architecture (no complex replication or durability guarantees)

**Mitigation**: Graceful shutdown flushes all data (zero loss on planned deployments)

### 4. Lazy Persistence

**Principle**: Write to database only when idle or safety nets trigger, not on every edit.

**Triggers**:
- **Idle threshold**: 30 seconds since last edit
- **Safety net**: 5 minutes since last persist (even if actively editing)
- **Critical writes**: OTP changes (immediate synchronous write)
- **Last disconnect**: Flush when last user leaves document
- **Graceful shutdown**: Flush all documents before exit

**Rationale**:
- Reduces database writes by ~99% (100 writes/second → ~1 write/minute)
- Extends SQLite lifespan (fewer write cycles)
- Improves performance (no I/O blocking on every keystroke)

See [03-persistence-strategy.md] for complete persistence design.

### 5. Real-Time First

**Principle**: All edits propagate instantly via WebSocket to all connected users.

**Latency**: Typically 1-50ms between typing and other users seeing changes

**Rationale**:
- Collaboration feels natural and responsive
- Operational Transformation guarantees convergence
- WebSocket provides low-latency bidirectional channel

---

## System Architecture Diagram

```
┌──────────────┐
│ User Browser │ ← WebSocket (real-time) → ┌─────────────┐
│   (React +   │                             │  Go Server  │
│   Monaco)    │ ← REST API (admin ops)  →  │             │
└──────────────┘                             └──────┬──────┘
                                                    │
                                              ┌─────▼────────┐
                                              │ OT Engine    │
                                              │ (in memory)  │
                                              └─────┬────────┘
                                                    │
                                           ┌────────▼─────────┐
                                           │ Lazy Persister   │
                                           │ (background)     │
                                           └────────┬─────────┘
                                                    │
                                                    ▼
                                              ┌──────────┐
                                              │  SQLite  │
                                              │ (backup) │
                                              └──────────┘
```

**Data flow**:
1. User types in browser → WebSocket Edit message to server
2. Server applies operation via OT engine (in memory)
3. Server broadcasts History message to all users (including sender)
4. Background persister writes to SQLite when triggered
5. On reload: Server loads document from SQLite into memory

---

## Document Lifecycle

### Cold Start (Document Not in Memory)

```pseudocode
WHEN user connects to document "abc123":
    IF "abc123" not in activeDocuments:
        // Check OTP BEFORE loading into memory (DoS prevention)
        IF database has OTP for "abc123":
            IF provided OTP doesn't match:
                REJECT connection

        // Load from database or create new
        persistedDoc = database.load("abc123")

        IF persistedDoc exists:
            document = createFromPersisted(persistedDoc)
        ELSE:
            document = createNewDocument()

        activeDocuments["abc123"] = document
```

### Hot (Document Active with Users)

```pseudocode
WHILE users are connected:
    // User 1 connects
    IF first user:
        START background persister goroutine

    document.addUser(userId)

    // Handle edits, cursor updates, etc.

    // User N disconnects
    document.removeUser(userId)

    IF last user disconnects:
        FLUSH document to database immediately
        STOP persister goroutine
```

### Idle (Document in Memory but No Users)

```pseudocode
WHEN last user disconnects:
    // Document stays in memory (cache)
    // Persister is stopped
    // Ready for fast reconnection

AFTER 7 days idle:
    // Cleanup goroutine runs
    FLUSH document to database
    REMOVE from activeDocuments
    DELETE from database
```

### Expiration (Database Cleanup)

```pseudocode
EVERY 1 hour:
    FOR EACH document in database:
        IF document.lastAccessed > 7 days ago:
            DELETE from database
```

---

## Concurrency Model

### Thread-Safe Components

- **Document storage**: `sync.Map` for lock-free reads
- **Document state**: `sync.RWMutex` per document
- **User ID counter**: `atomic.Uint64` for lock-free increments
- **Last edit time**: `atomic.Int64` for lock-free timestamp updates

### Goroutines

- **1 per WebSocket connection**: Handles messages for that user
- **1 persister per active document**: Periodically writes to database
- **1 cleanup goroutine**: Removes old documents every hour
- **1 graceful shutdown goroutine**: Listens for SIGTERM

### Broadcast Mechanism

```pseudocode
FUNCTION broadcast(message):
    // Send to ALL connected users via channels
    FOR EACH user IN document.subscribers:
        SELECT:
            CASE channel is ready:
                SEND message
            CASE channel is full:
                SKIP (non-blocking, prevents slow client from blocking others)
```

See [../backend/02-broadcast-system.md] for detailed broadcast architecture.

---

## Security Model

### No User Accounts

**Decision**: Kolabpad does not have user accounts or authentication.

**Rationale**:
- Reduces complexity (no signup, login, password management)
- Faster onboarding (just share a link)
- Sufficient for ephemeral collaboration

**Trade-off**: Cannot track document ownership or enforce fine-grained permissions

### OTP-Based Document Protection

**Feature**: Optional one-time password (OTP) to restrict document access

**How it works**:
```pseudocode
USER enables protection:
    → Server generates random 6-character OTP
    → Stores OTP in database (immediate synchronous write)
    → Stores OTP in memory
    → Returns OTP to user
    → User shares link: https://app.com/#doc123?otp=xyz789

OTHER USER opens link:
    → Client extracts OTP from URL
    → Connects WebSocket with ?otp=xyz789
    → Server validates OTP before accepting connection
```

**Security improvements**:
- Dual-check pattern: Validate OTP BEFORE loading document into memory (prevents DoS)
- Require current OTP to disable protection (prevents unauthorized changes)
- User attribution in broadcasts (track who enabled/disabled protection)

See [../security/01-authentication-model.md] for complete security architecture.

---

## Performance Characteristics

### Memory Usage

- **Empty document**: ~240 bytes
- **Typical document** (5KB text): ~5.2 KB
- **Large document** (256KB limit): ~260 KB
- **50 active documents**: ~12 MB RAM
- **1,000 active documents**: ~250 MB RAM

### Database Writes

With lazy persistence:
- **Before**: 100 writes/second (every keystroke)
- **After**: ~1 write/minute (idle threshold)
- **Reduction**: ~99%

### WebSocket Latency

- **Edit propagation**: 1-50ms (local network to same continent)
- **Cursor updates**: Debounced to 20ms intervals client-side
- **Typical round-trip**: <100ms for most users

### Connection Keepalive

- **Heartbeat mechanism**: Native WebSocket ping/pong frames
- **Ping interval**: 60 seconds (configurable via `WS_HEARTBEAT_INTERVAL_SECONDS`)
- **Purpose**: Prevents Cloudflare and reverse proxies from closing idle connections (typical 100s timeout)
- **Overhead**: ~12 bytes/minute per connection (6 bytes ping + 6 bytes pong)
- **Implementation**: Server-side only, browser automatically responds per WebSocket spec

---

## Deployment Model

### Single-Server Deployment

**Current design**: Kolabpad runs as a single server instance.

**Why**:
- Simplicity: No distributed state management
- Consistency: Single authority prevents conflicts
- Performance: All state in memory, no network hops

**Scaling**:
- **Vertical**: Add more RAM and CPU to handle more documents
- **Limits**: ~10,000 active documents on commodity hardware

**For horizontal scaling**, would need:
- Sticky sessions (route users to same server)
- Distributed state (Redis for shared metadata)
- Shared persistence (S3 instead of SQLite)

See [../development/03-deployment-monitoring.md] for deployment guide.

---

## Quick Links to Detailed Documentation

### Architecture Deep Dives
- **[02-operational-transformation.md]**: OT algorithm, transform logic, operation lifecycle
- **[03-persistence-strategy.md]**: Lazy persistence design, triggers, monitoring

### Backend
- **[../backend/01-server-architecture.md]**: Server structure, document lifecycle, concurrency
- **[../backend/02-broadcast-system.md]**: How state changes propagate to all clients

### Frontend
- **[../frontend/01-frontend-architecture.md]**: React architecture, hooks, state management
- **[../frontend/02-state-synchronization.md]**: State reset, broadcast sync, Monaco integration

### Protocol
- **[../protocol/01-websocket-protocol.md]**: Complete WebSocket message reference
- **[../protocol/02-rest-api.md]**: REST endpoints for OTP and admin operations

### Security
- **[../security/01-authentication-model.md]**: OTP protection, dual-check pattern
- **[../security/02-security-considerations.md]**: Threat model, acceptable risks

### Development
- **[../development/01-development-workflow.md]**: Running locally, building, contributing
- **[../development/02-testing-strategy.md]**: Test coverage, running tests
- **[../development/03-deployment-monitoring.md]**: Production deployment, monitoring, alerting

---

## Frequently Asked Questions

### Why Go instead of Rust?

The original Rustpad was written in Rust. Kolabpad was migrated to Go for:
- Easier onboarding for contributors (more developers know Go)
- Simpler concurrency model (goroutines + channels)
- Faster iteration during development
- Built-in HTTP/WebSocket libraries

### Why OT instead of CRDTs?

Operational Transformation fits Kolabpad's design better:
- **Sequential operations**: Easier to reason about and debug
- **Server authority**: Single source of truth eliminates conflicts
- **Smaller payloads**: OT operations are more compact than CRDT state
- **Proven**: OT has been used in Google Docs, Etherpad, etc.

Trade-off: Server is required (CRDTs can work peer-to-peer)

See [02-operational-transformation.md] for detailed comparison.

### Why SQLite instead of PostgreSQL/MySQL?

SQLite is sufficient for Kolabpad's use case:
- **Single-file deployment**: No database server to manage
- **Fast enough**: Read latency doesn't matter (memory-first)
- **Simpler operations**: No connection pooling, authentication, etc.
- **Embedded**: Runs in the same process as the server

Trade-off: Limited to single-server deployment

### What happens on server crash?

Without graceful shutdown:
- **Data loss**: Last 30 seconds to 5 minutes of edits (lazy persistence window)
- **Recovery**: Documents reload from database on next access
- **Acceptable**: Designed for ephemeral collaboration, not critical data

With graceful shutdown (normal deployments):
- **Zero data loss**: All documents flushed before exit
- **Clean restart**: Documents reload seamlessly

### How many concurrent users can Kolabpad support?

Depends on document size and edit frequency:
- **Small documents** (5KB, 10 users each): ~1,000 documents = 10,000 users
- **Large documents** (100KB, 50 users each): ~100 documents = 5,000 users
- **Bottleneck**: RAM for document storage, not CPU

Horizontal scaling requires architectural changes (see Deployment Model above).

---

## Next Steps

1. **Understand OT**: Read [02-operational-transformation.md] to see how concurrent edits are resolved
2. **Understand persistence**: Read [03-persistence-strategy.md] to see why lazy writes are safe
3. **Run locally**: Follow [../development/01-development-workflow.md] to get started
4. **Explore code**: Start with `cmd/server/main.go` and trace through a WebSocket connection
