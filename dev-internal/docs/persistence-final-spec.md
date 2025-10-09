# Kolabpad Persistence: Final Implementation Specification

**Created:** 2025-10-08
**Status:** Approved - Ready for Implementation
**Consolidates:** memory-persistence-strategy.local.md + memory-extended.local.md

---

## 1. Overview & Architecture Decisions

### 1.1 Core Principles

**Memory-First Architecture:**
- All active documents live in memory (source of truth)
- Reads/writes happen in memory (instant, microseconds)
- DB is backup/recovery mechanism only (writes are lazy, conditional)

**Lazy Persistence:**
- Write to DB only when necessary (idle, safety net, critical events)
- NOT on every change or fixed timer
- Accept 30s-5min data loss on crash (ephemeral collaboration use case)

**User-Based Lifecycle:**
- Persister starts when first user connects
- Persister stops when last user disconnects
- Flush to DB on last disconnect (then stop)
- Document stays in memory for 24h (hot cache)

### 1.2 Conflict Resolutions

**Conflict 1 - Persister Lifecycle:**
- ✅ **DECISION: Option B (User-based)**
- Start persister when first user connects
- Stop persister when last user disconnects
- Flush immediately on last disconnect
- More resource efficient than 24/7 persisters

**Conflict 2 - OTP Validation:**
- ✅ **DECISION: Option B (Dual-check pattern)**
- Hot documents (in memory): validate from memory (no DB read)
- Cold documents (not in memory): validate from DB before loading (prevents DoS)
- One extra DB read for cold docs is acceptable trade-off

---

## 2. Data Flow Architecture

```
┌─────────────────────────────────────────┐
│         IN-MEMORY STATE                 │
│  (Source of truth for active docs)      │
│                                         │
│  State {                                │
│    Operations []UserOperation           │
│    Text       string                    │
│    Language   *string                   │
│    OTP        *string      ← NEW        │
│    Users      map[userID]               │
│    Cursors    map[userID]               │
│  }                                      │
│                                         │
│  Metadata (per document):               │
│    lastEditTime          int64          │
│    lastPersistedRevision int            │
│    lastCriticalWrite     int64          │
│    persisterCancel       func()         │
│  }                                      │
└─────────────┬───────────────────────────┘
              │
              │ Lazy persist
              │ (conditional writes)
              ▼
┌─────────────────────────────────────────┐
│       SQLITE DATABASE                   │
│    (Cold backup/recovery)               │
│                                         │
│  PersistedDocument {                    │
│    ID       string                      │
│    Text     string   (final only)       │
│    Language *string                     │
│    OTP      *string                     │
│  }                                      │
│                                         │
│  Note: OT history NOT persisted         │
└─────────────────────────────────────────┘
```

---

## 3. Write Scenarios (Complete Reference)

### 3.1 When Does Data Write to DB?

| Scenario | Trigger | Write Timing | Rationale |
|----------|---------|--------------|-----------|
| **Normal editing (continuous)** | User typing for 5+ min | Every 5min (safety net) | Prevent excessive data loss on crash during long sessions |
| **User stops typing** | Idle 30s detected | Once, 30-40s after last keystroke | Save completed work when user pauses |
| **OTP protect/unprotect** | API call to `/api/document/{id}/protect` | Immediately, synchronously | Critical security data must persist instantly |
| **Last user disconnects** | User count drops to 0 | Immediately before stopping persister | Document going dormant, ensure latest state saved |
| **Graceful shutdown** | Server receives SIGTERM | Immediately, all documents flushed | Zero data loss on deployments/restarts |
| **Memory eviction** | Document idle 24h, cleanup runs | Before eviction (always flush) | Free RAM safely without data loss |
| **First connection (new doc)** | Never in DB, first user connects | No write until idle/disconnect | New empty docs don't need immediate persist |

### 3.2 When Does Data Write to Memory?

| Operation | Timing | Propagation |
|-----------|--------|-------------|
| **User types/edits** | Instant (microseconds) | Broadcast to all connected users immediately |
| **Cursor moves** | Instant | Broadcast immediately |
| **Language changes** | Instant | Broadcast immediately |
| **OTP changes** | Instant | Broadcast immediately + DB write |
| **User joins/leaves** | Instant | Broadcast immediately |

**Key Point:** Memory writes are NEVER delayed. Collaboration is always real-time.

---

## 4. Read Scenarios (Complete Reference)

### 4.1 When Does Data Read from DB?

| Scenario | Condition | What's Read | Next Step |
|----------|-----------|-------------|-----------|
| **First user connects (cold start)** | Document not in memory | Full document (text, language, OTP) | Load into memory, start persister |
| **OTP validation (cold doc)** | Document not in memory AND has OTP | OTP only (lightweight) | Validate before loading full document |
| **After memory eviction** | Document re-accessed after 24h idle | Full document | Restore from backup, start persister |
| **Server restart** | All documents cold | Full document on first access | Lazy loading on demand |

### 4.2 When Does Data Read from Memory?

| Scenario | Condition | What's Read |
|----------|-----------|-------------|
| **User connects (hot doc)** | Document already in memory | Initial state (ops, users, cursors, language, OTP) |
| **OTP validation (hot doc)** | Document in memory | OTP from memory (no DB read) |
| **Real-time updates** | Another user edits | Operations since last revision |
| **API `/api/text/{id}`** | Document exists in memory | Current text string |

---

## 5. Persister Lifecycle (Complete Flow)

### 5.1 Persister Logic (Runs Every 10 Seconds)

```pseudocode
FUNCTION persister(docID, document):
    lastPersistedRevision = 0
    lastPersistTime = NOW()

    LOOP every 10 seconds:
        // Exit condition
        IF document.killed:
            RETURN

        // Skip if no changes
        currentRevision = document.revision()
        IF currentRevision <= lastPersistedRevision:
            CONTINUE

        // Debounce: Skip if critical write happened recently
        timeSinceCritical = NOW() - document.lastCriticalWrite
        IF timeSinceCritical < 2 seconds:
            CONTINUE

        // Check write triggers
        timeSinceEdit = NOW() - document.lastEditTime
        timeSincePersist = NOW() - lastPersistTime

        shouldWrite = FALSE

        // Trigger 1: Idle threshold
        IF timeSinceEdit >= 30 seconds:
            shouldWrite = TRUE
            reason = "idle"

        // Trigger 2: Safety net
        IF timeSincePersist >= 5 minutes:
            shouldWrite = TRUE
            reason = "safety_net"

        // Write to DB if triggered
        IF shouldWrite:
            snapshot = document.snapshot()
            otp = document.getOTP()  // From memory, no DB read!

            DB.store({
                id: docID,
                text: snapshot.text,
                language: snapshot.language,
                otp: otp
            })

            lastPersistedRevision = currentRevision
            lastPersistTime = NOW()
            LOG("persisted", docID, reason, currentRevision)
```

### 5.2 Persister Start/Stop Conditions

```pseudocode
// START PERSISTER (in handleSocket after user connects)
FUNCTION onUserConnected(docID, document):
    userID = document.nextUserID()
    document.addUser(userID)

    // First user? Start persister
    IF document.userCount() == 1:
        ctx, cancel = createCancellableContext()
        document.persisterCancel = cancel

        START_GOROUTINE persister(ctx, docID, document)
        LOG("persister_started", docID)

// STOP PERSISTER (in connection cleanup after user disconnects)
FUNCTION onUserDisconnected(docID, document, userID):
    document.removeUser(userID)

    // Last user? Flush and stop
    IF document.userCount() == 0:
        // Immediate flush (don't wait for idle)
        snapshot = document.snapshot()
        otp = document.getOTP()

        DB.store({
            id: docID,
            text: snapshot.text,
            language: snapshot.language,
            otp: otp
        })

        // Stop persister
        document.persisterCancel()
        document.persisterCancel = NULL

        LOG("persister_stopped", docID, "last_user_left")

    // Document stays in memory (hot cache for 24h)
```

### 5.3 Example Timeline

```
09:00:00 - User A connects
         → persister STARTS (check every 10s)

09:00:05 - User A types "hello"
         → memory updated instantly
         → lastEditTime = 09:00:05

09:00:10 - Persister check #1
         → timeSinceEdit = 5s (< 30s idle)
         → timeSincePersist = 10s (< 5min safety)
         → SKIP (no write)

09:00:20 - Persister check #2
         → timeSinceEdit = 15s (< 30s idle)
         → SKIP

09:00:30 - Persister check #3
         → timeSinceEdit = 25s (< 30s idle)
         → SKIP

09:00:40 - Persister check #4
         → timeSinceEdit = 35s (>= 30s idle) ✓
         → WRITE TO DB ("hello")
         → lastPersistTime = 09:00:40

09:00:50 - Persister check #5
         → No changes since last persist
         → SKIP

09:01:00 - User B connects
         → userCount = 2
         → persister CONTINUES running

09:01:10 - User B types "world"
         → memory updated instantly
         → lastEditTime = 09:01:10

09:01:20 - User A disconnects
         → userCount = 1
         → persister CONTINUES running

09:01:40 - Persister check
         → timeSinceEdit = 30s (>= 30s idle) ✓
         → WRITE TO DB ("hello world")

09:02:00 - User B disconnects
         → userCount = 0 (LAST USER!)
         → WRITE TO DB immediately ("hello world")
         → STOP persister
         → Document stays in memory (dormant, hot cache)

09:02:10 - (No persister running)
09:02:20 - (No persister running)
...
10:02:00 - User C connects (1 hour later)
         → Document still in memory (instant load)
         → persister STARTS again
```

---

## 6. Race Condition Handling (All Approved Pitfall Fixes)

### 6.1 Critical Race Conditions

#### Pitfall #1: Multiple Persisters per Document
**Problem:** Starting persister on every WebSocket connection → goroutine leak
**Fix:** Start persister only when first user connects, stop when last user disconnects
**Implementation:** Track user count, use `CompareAndSwap` logic on first/last user

#### Pitfall #2: OTP Race (Protect/Unprotect vs. Persister)
**Problem:** Concurrent OTP changes and persister writes → cache/DB desync
**Fix:** Debounce persister after critical writes
**Implementation:**
```pseudocode
// In SetOTP:
document.state.OTP = newOTP
document.lastCriticalWrite = NOW()
DB.updateOTP(docID, newOTP)  // Immediate write

// In persister:
IF NOW() - document.lastCriticalWrite < 2 seconds:
    SKIP  // Let critical write finish
```

#### Pitfall #3: Graceful Shutdown Data Loss
**Problem:** Shutdown kills documents without flushing
**Fix:** Synchronous flush all documents before killing
**Implementation:**
```pseudocode
FUNCTION shutdown():
    FOR EACH document IN activeDocuments:
        snapshot = document.snapshot()
        otp = document.getOTP()
        DB.store(docID, snapshot, otp)

        IF document.persisterCancel:
            document.persisterCancel()

        document.kill()
```

#### Pitfall #4: Memory Eviction Data Loss
**Problem:** Cleanup evicts documents without ensuring final persist
**Fix:** Always flush before eviction
**Implementation:**
```pseudocode
FUNCTION cleanupExpiredDocuments():
    FOR EACH docID IN expiredDocuments:
        document = activeDocuments.remove(docID)

        // Always flush (idempotent, safe)
        snapshot = document.snapshot()
        otp = document.getOTP()
        DB.store(docID, snapshot, otp)

        IF document.persisterCancel:
            document.persisterCancel()

        document.kill()
```

#### Pitfall #5: OTP Validation Order (DoS Prevention)
**Problem:** Load document before validating OTP → DoS vector
**Fix:** Dual-check pattern
**Implementation:**
```pseudocode
FUNCTION handleSocket(docID, requestedOTP):
    // Fast path: Document in memory
    IF document = activeDocuments.get(docID):
        otp = document.getOTP()  // From memory
        IF otp != NULL AND otp != requestedOTP:
            REJECT "Invalid OTP"

    // Slow path: Document not in memory (prevent DoS)
    ELSE:
        persisted = DB.load(docID)
        IF persisted.OTP != NULL AND persisted.OTP != requestedOTP:
            REJECT "Invalid OTP"  // Reject BEFORE loading

    // Safe to load/create document
    document = getOrCreateDocument(docID)
    ...
```

#### Pitfall #6: Context Cancellation Race
**Problem:** Persister uses request context → dies when first user disconnects
**Fix:** Use `context.Background()` instead of request context
**Implementation:**
```pseudocode
// WRONG:
START_GOROUTINE persister(httpRequest.context, docID, document)

// CORRECT:
START_GOROUTINE persister(context.Background(), docID, document)
```

### 6.2 Minor Pitfalls (Implementation Details)

#### Pitfall #7: Atomic Field Access
**Requirement:** Use atomic store/load methods, not direct assignment
```pseudocode
// WRONG:
document.lastEditTime = NOW()

// CORRECT:
document.lastEditTime.store(NOW())
document.lastEditTime.load()
```

#### Pitfall #8: Persister Jitter Scaling
**Status:** RESOLVED (not needed)
**Why:** User-based lifecycle provides natural jitter (users connect at different times)

#### Pitfall #9: OTP Initialization on Load
**Problem:** FromPersistedDocument doesn't initialize OTP from DB
**Fix:** Add OTP parameter and store in state
```pseudocode
FUNCTION fromPersistedDocument(text, language, otp):
    document = newDocument()
    document.state.OTP = otp  // Initialize from DB
    document.state.text = text
    document.state.language = language
    RETURN document
```

---

## 7. OTP Handling (Special Considerations)

### 7.1 OTP Write Path (Critical - Must Be Immediate)

```pseudocode
FUNCTION handleProtectDocument(docID):
    otp = generateOTP()

    // Update memory
    document = getDocument(docID)
    document.setOTP(otp)  // Stores in state + sets lastCriticalWrite

    // Write to DB IMMEDIATELY (don't wait for persister)
    DB.updateOTP(docID, otp)

    // Broadcast to all connected clients
    document.broadcast(OTPMsg(otp))

    RETURN otp

FUNCTION handleUnprotectDocument(docID):
    // Update memory
    document = getDocument(docID)
    document.setOTP(NULL)  // Sets lastCriticalWrite

    // Write to DB IMMEDIATELY
    DB.updateOTP(docID, NULL)

    // Broadcast to all connected clients
    document.broadcast(OTPMsg(NULL))
```

### 7.2 OTP Read Path (Dual-Check Pattern)

**Hot Documents (in memory):**
```pseudocode
document = activeDocuments.get(docID)
otp = document.getOTP()  // From memory, no DB read
IF otp != NULL:
    validate(requestedOTP == otp)
```

**Cold Documents (not in memory):**
```pseudocode
persisted = DB.load(docID)  // One DB read
IF persisted.OTP != NULL:
    validate(requestedOTP == persisted.OTP)
THEN:
    document = fromPersistedDocument(persisted.text, persisted.language, persisted.OTP)
```

---

## 8. Performance Impact

### 8.1 Database Operations

**Before (aggressive persistence):**
```
100 active documents, continuous editing:
- Persister reads (OTP):  2,000/min (20/min × 100 docs)
- Persister writes:       2,000/min (20/min × 100 docs)
- OTP validations:          ~60/min (connection rate)
──────────────────────────────────────────────────────
TOTAL:                    ~4,060 DB ops/min
```

**After (lazy persistence):**
```
100 active documents, continuous editing:
- Persister reads:             0 (eliminated - OTP from memory)
- Persister writes:         ~100/min (idle/safety triggers)
- OTP validations (hot):       0 (from memory)
- OTP validations (cold):    ~10/min (cold starts only)
──────────────────────────────────────────────────────
TOTAL:                     ~110 DB ops/min

REDUCTION: 97.3% fewer DB operations
```

### 8.2 Memory Usage

**Per Document Overhead:**
```
State struct:
- Operations: ~10KB (average, grows with edits)
- Text: variable (max 256KB enforced)
- Language: ~8 bytes
- OTP: ~8 bytes (pointer)
- Users: ~500 bytes (assuming 10 users)
- Cursors: ~500 bytes

New tracking fields:
- lastEditTime: 8 bytes (atomic.Int64)
- lastPersistedRevision: 4 bytes (atomic.Int32)
- lastCriticalWrite: 8 bytes (atomic.Int64)
- persisterCancel: 8 bytes (func pointer)

TOTAL NEW OVERHEAD: ~28 bytes per document (negligible)
```

**Realistic Scale:**
```
10 docs:      ~250 KB   (trivial)
50 docs:      ~1.25 MB  (target scale)
100 docs:     ~2.5 MB   (comfortable)
1,000 docs:   ~25 MB    (easily supported)
10,000 docs:  ~250 MB   (needs decent server)
```

### 8.3 Persister Resource Usage

**Before (24/7 persisters):**
- 100 documents → 100 persisters always running
- 100 goroutines checking every 3s
- CPU: ~2,000 timer wakeups/min

**After (user-based lifecycle):**
- 100 documents, 50 active (users connected) → 50 persisters
- 50 goroutines checking every 10s
- CPU: ~300 timer wakeups/min
- **85% reduction in goroutines and timer events**

---

## 9. Data Loss Scenarios & Mitigation

### 9.1 Acceptable Data Loss (By Design)

**Scenario 1: Server Crash During Active Editing**
```
Timeline:
- User types for 2 minutes straight
- Safety net hasn't triggered yet (< 5min)
- Server crashes (power loss, OOM, kernel panic)

Data Loss: Last 2 minutes of edits ❌

Mitigation:
✅ User can re-paste content (ephemeral collaboration use case)
✅ Lower safety net to 2-3 min for critical docs
✅ Acceptable trade-off for 97% DB reduction
```

**Scenario 2: Server Crash During Idle Period**
```
Timeline:
- User types "hello", stops at 09:00:00
- Server crashes at 09:00:25 (before 30s idle threshold)

Data Loss: Last 25 seconds of edits ❌

Mitigation:
✅ Worst case: 30 seconds of data loss
✅ Most edits saved within 1 minute of completion
✅ Acceptable for ephemeral collaboration
```

### 9.2 Zero Data Loss (Guaranteed)

**Scenario 1: Graceful Shutdown**
```
Timeline:
- Deployment/restart
- Server receives SIGTERM
- Shutdown handler persists all in-memory docs

Data Loss: ZERO ✅
```

**Scenario 2: User Disconnects (Last User)**
```
Timeline:
- Team edits for 30 minutes
- Last user disconnects
- Immediate flush before stopping persister

Data Loss: ZERO ✅
```

**Scenario 3: Memory Eviction**
```
Timeline:
- Document idle 24h in memory
- Cleanup task evicts document
- Always flush before eviction

Data Loss: ZERO ✅
```

---

## 10. Implementation Checklist

### Phase 1: Memory State Enhancement (`kolabpad.go`)

- [ ] Add `OTP *string` to `State` struct (line 15)
- [ ] Add `lastEditTime atomic.Int64` to `Kolabpad` struct (line 24)
- [ ] Add `lastPersistedRevision atomic.Int32` to `Kolabpad` struct
- [ ] Add `lastCriticalWrite atomic.Int64` to `Kolabpad` struct
- [ ] Add `GetOTP() *string` method (thread-safe getter with RLock)
- [ ] Update `SetOTP()` to store in `state.OTP` + set `lastCriticalWrite`
- [ ] Update `ApplyEdit()` to call `lastEditTime.Store(time.Now().Unix())`
- [ ] Update `SetLanguage()` to call `lastEditTime.Store(time.Now().Unix())`
- [ ] Add `UserCount() int` method (returns `len(state.Users)` with RLock)
- [ ] Add `LastEditTime() time.Time` method (returns `time.Unix(lastEditTime.Load(), 0)`)
- [ ] Update `FromPersistedDocument()` to accept `otp *string` parameter
- [ ] Update `FromPersistedDocument()` to initialize `state.OTP = otp`

### Phase 2: Document Lifecycle (`server.go`)

- [ ] Add `persisterCancel context.CancelFunc` to `Document` struct
- [ ] Add `persisterMu sync.Mutex` to `Document` struct
- [ ] Update `getOrCreateDocument()` to pass OTP: `FromPersistedDocument(text, lang, persisted.OTP, ...)`

### Phase 3: Persister Start/Stop Logic (`server.go` + `connection.go`)

- [ ] In `handleSocket()` after user connects:
  - [ ] Check `if doc.Kolabpad.UserCount() == 1` (first user)
  - [ ] If true: Create `context.WithCancel(context.Background())`
  - [ ] Store cancel func in `doc.persisterCancel`
  - [ ] Start persister goroutine with background context

- [ ] In connection cleanup (after `RemoveUser()`):
  - [ ] Check `if doc.Kolabpad.UserCount() == 0` (last user)
  - [ ] If true:
    - [ ] Flush to DB immediately (snapshot + OTP)
    - [ ] Call `doc.persisterCancel()`
    - [ ] Set `doc.persisterCancel = nil`

### Phase 4: Lazy Persister Logic (`server.go`)

- [ ] Change interval from `3 * time.Second` → `10 * time.Second`
- [ ] Add tracking: `lastPersistTime := time.Now()`
- [ ] Add tracking: `lastPersistedRev := 0`
- [ ] Add check: Skip if `revision <= lastPersistedRev`
- [ ] Add check: Skip if `time.Now().Unix() - kolabpad.lastCriticalWrite < 2`
- [ ] Add idle trigger: `time.Since(kolabpad.LastEditTime()) >= 30*time.Second`
- [ ] Add safety net: `time.Since(lastPersistTime) >= 5*time.Minute`
- [ ] Change `otp := db.Load(id).OTP` → `otp := kolabpad.GetOTP()`
- [ ] Update `lastPersistedRev` and `lastPersistTime` after write

### Phase 5: OTP Validation Dual-Check (`server.go`)

- [ ] In `handleSocket()`, replace current OTP check with:
  - [ ] Fast path: `if doc, ok := documents.Load(docID)` → validate from memory
  - [ ] Slow path: `else` → load from DB, validate OTP, reject before getOrCreateDocument
  - [ ] Then proceed with `doc := s.getOrCreateDocument(docID)`

### Phase 6: Critical Write Handlers (`server.go`)

- [ ] Verify `handleProtectDocument()` calls `SetOTP(&otp)` (updates `lastCriticalWrite`)
- [ ] Verify `handleProtectDocument()` writes to DB immediately
- [ ] Verify `handleUnprotectDocument()` calls `SetOTP(nil)` (updates `lastCriticalWrite`)
- [ ] Verify `handleUnprotectDocument()` writes to DB immediately

### Phase 7: Graceful Shutdown & Cleanup (`server.go`)

- [ ] Update `Shutdown()`:
  - [ ] Iterate all documents with `Range()`
  - [ ] Flush each: `db.Store(snapshot + OTP from memory)`
  - [ ] Cancel persister: `doc.persisterCancel()`
  - [ ] Kill document
  - [ ] Log flush progress

- [ ] Update `cleanupExpiredDocuments()`:
  - [ ] Before `LoadAndDelete()` and `Kill()`
  - [ ] Always flush: `db.Store(snapshot + OTP from memory)`
  - [ ] Cancel persister: `doc.persisterCancel()`
  - [ ] Then evict and kill

### Phase 8: Testing & Validation

- [ ] Test: Multiple users → only 1 persister per document
- [ ] Test: Rapid protect/unprotect → no race condition
- [ ] Test: Document persists 30-40s after user stops typing
- [ ] Test: Safety net triggers during 5min continuous editing
- [ ] Test: Last user disconnect → immediate flush + persister stops
- [ ] Test: Graceful shutdown (SIGTERM) → all data persisted
- [ ] Test: Memory eviction → data flushed before eviction
- [ ] Test: Invalid OTP attempts → rejected before loading cold docs
- [ ] Test: Hot document OTP validation → no DB read (check logs)
- [ ] Test: Cold document OTP validation → one DB read (check logs)

---

## 11. Monitoring & Observability

### 11.1 Key Metrics to Track

**Performance Metrics:**
```
- db_writes_per_minute        (should be ~100 for 100 docs)
- db_reads_per_minute         (should be near 0 for OTP)
- persist_latency_ms          (how long DB writes take)
- documents_in_memory         (current count)
- active_persisters_count     (should equal docs with users)
- memory_usage_mb             (monitor for leaks)
```

**Behavior Metrics:**
```
- idle_persist_count          (writes triggered by 30s idle)
- safety_net_persist_count    (writes triggered by 5min safety)
- critical_write_count        (OTP changes)
- last_user_flush_count       (flushes on last disconnect)
- eviction_flush_count        (flushes before eviction)
- shutdown_flush_count        (flushes on graceful shutdown)
```

**Health Metrics:**
```
- persist_errors              (DB write failures)
- otp_validation_errors       (invalid OTP attempts)
- persister_start_count       (new persisters started)
- persister_stop_count        (persisters stopped)
- goroutine_count             (monitor for leaks)
```

### 11.2 Alerts

```
- db_writes_per_minute > 500 (for 100 docs) → Something wrong with lazy persistence
- memory_usage_mb > 80% of available        → Need to tune retention policy
- persist_errors > 0                        → DB issues, investigate immediately
- active_persisters > documents_in_memory   → Goroutine leak (Pitfall #1 not fixed)
- otp_validation_errors > 100/min           → Potential DoS attack
```

### 11.3 Debug Logging

**Key log events:**
```
- "persister_started"         (docID, userCount)
- "persister_stopped"         (docID, reason: "last_user_left" | "shutdown" | "eviction")
- "persisted"                 (docID, reason: "idle" | "safety_net", revision)
- "critical_write"            (docID, type: "otp_protect" | "otp_unprotect")
- "flush"                     (docID, reason: "last_disconnect" | "shutdown" | "eviction")
- "otp_validation"            (docID, source: "memory" | "db", result: "valid" | "invalid")
```

---

## 12. Configuration Values

**Recommended settings for production:**

```pseudocode
// Persister loop
persisterCheckInterval = 10 seconds

// Write triggers
idleWriteThreshold = 30 seconds   // Write after 30s idle
safetyNetInterval  = 5 minutes    // Force write every 5min

// Race condition prevention
criticalWriteDebounce = 2 seconds // Skip persister after OTP change

// Memory management
memoryRetentionTime = 24 hours    // Keep in memory 24h after last access

// Expiration
documentExpiry = 7 days           // Delete from DB after 7 days

// Cleanup task
cleanupInterval = 1 hour          // Run cleanup every hour
```

**Tuning guidance:**
- Lower safety net (2-3min) if data loss is critical concern
- Higher idle threshold (60s) if write reduction is priority
- Lower memory retention (6h) if RAM is constrained
- Higher expiry (30 days) if long-term storage needed

---

## 13. Testing Strategy

### 13.1 Unit Tests

```pseudocode
TEST multiple_persisters_prevented:
    server = newServer()
    docID = "test"

    // Connect 10 users
    FOR i = 1 TO 10:
        connectUser(docID)

    // Verify only 1 persister running
    ASSERT goroutineCount("persister") == 1

TEST otp_race_condition:
    server = newServer()
    docID = "test"

    // Rapidly protect/unprotect 100 times
    FOR i = 1 TO 100:
        server.protectDocument(docID)
        server.unprotectDocument(docID)

    // Verify memory and DB match
    memoryOTP = server.getDocument(docID).getOTP()
    dbOTP = DB.load(docID).OTP
    ASSERT memoryOTP == dbOTP

TEST graceful_shutdown:
    server = newServer()

    // Create document and edit
    doc = server.getDocument("test")
    doc.applyEdit("hello world")

    // Shutdown immediately (before idle threshold)
    server.shutdown()

    // Verify persisted
    persisted = DB.load("test")
    ASSERT persisted.text == "hello world"

TEST idle_persistence:
    server = newServer()
    doc = server.getDocument("test")

    doc.applyEdit("hello")
    WAIT 40 seconds

    // Should have persisted after 30s idle
    persisted = DB.load("test")
    ASSERT persisted.text == "hello"

TEST safety_net:
    server = newServer()
    doc = server.getDocument("test")

    // Edit continuously for 6 minutes
    FOR i = 0 TO 360 seconds BY 1:
        doc.applyEdit("char" + i)
        WAIT 1 second

    // Should have written at 5min mark
    ASSERT dbWriteCount >= 1

TEST last_user_disconnect:
    server = newServer()
    user1 = connectUser("test")
    user2 = connectUser("test")

    doc = server.getDocument("test")
    doc.applyEdit("data")

    // First user leaves - persister should continue
    user1.disconnect()
    ASSERT persisterRunning("test") == TRUE

    // Last user leaves - should flush and stop
    user2.disconnect()
    WAIT 100ms

    persisted = DB.load("test")
    ASSERT persisted.text == "data"
    ASSERT persisterRunning("test") == FALSE
```

### 13.2 Integration Tests

```bash
# Test 1: Multiple persisters prevention
curl -N ws://localhost:3030/api/socket/test &
curl -N ws://localhost:3030/api/socket/test &
curl -N ws://localhost:3030/api/socket/test &
# Check: curl localhost:6060/debug/pprof/goroutine | grep persister
# Expect: Only 1 persister goroutine

# Test 2: OTP race
for i in {1..100}; do
  curl -X POST http://localhost:3030/api/document/test/protect
  curl -X DELETE http://localhost:3030/api/document/test/protect
done
# Check logs: No "OTP mismatch" errors

# Test 3: Graceful shutdown
echo "test data" | websocat ws://localhost:3030/api/socket/test
pkill -SIGTERM kolabpad-server
./kolabpad-server &
curl http://localhost:3030/api/text/test
# Expect: "test data"

# Test 4: DoS prevention (invalid OTP, cold doc)
for i in {1..1000}; do
  curl "ws://localhost:3030/api/socket/protected?otp=wrong"
done
# Check logs: Should reject BEFORE loading document
# Check metrics: db_reads should be ~1, not 1000
```

---

## 14. Migration & Rollback

### 14.1 Deployment Safety

**This change is fully backward compatible:**

✅ No database schema changes
✅ Existing documents load correctly
✅ No data migration needed
✅ Can deploy incrementally
✅ Can roll back safely

**Deployment steps:**
1. Deploy new code (zero downtime)
2. Monitor `db_writes_per_minute` (should drop 95%+)
3. Monitor `memory_usage_mb` (should stay low)
4. Monitor logs for persist errors
5. If issues: Roll back (falls back to aggressive persistence)

### 14.2 Rollback Plan

**If issues arise:**
```
1. Revert to previous version (aggressive persistence)
2. All in-memory data will be flushed on shutdown
3. No data loss (backward compatible)
4. DB continues working with old code
```

**Rollback is safe because:**
- DB schema unchanged
- Persisted format unchanged (still `PersistedDocument`)
- Old code reads OTP from DB (works fine)
- Old code writes aggressively (redundant but safe)

---

## 15. Future Enhancements

### 15.1 If Scaling to 10,000+ Documents

**Tiered persistence based on protection level:**
```pseudocode
IF document.hasOTP():
    safetyNet = 2 minutes      // Protected docs are critical
    memoryRetention = 24 hours
ELSE:
    safetyNet = 10 minutes     // Public docs are ephemeral
    memoryRetention = 6 hours
```

**LRU eviction instead of time-based:**
```pseudocode
IF memoryUsage > threshold:
    // Keep hot documents regardless of age
    // Evict least-recently-used
    evict(leastRecentlyUsed)
```

### 15.2 If Multi-Region Deployment

**Sticky sessions:**
- Route user to same server (avoid split-brain)

**Redis for OTP/metadata:**
- Move OTP/language to Redis (fast, shared across servers)
- SQLite only for document text

**S3 persistence:**
- Replace SQLite with object storage
- One object per document (`{docID}.json`)

### 15.3 Optional: Skip DB for Public Docs

```pseudocode
// Only persist protected documents
IF document.hasOTP():
    // Normal lazy persistence
ELSE:
    // Never write to DB (truly ephemeral)
    // Only exists in memory
```

**Trade-off:** Public docs lost on server restart, but:
- Reduces DB size by ~90% (most docs are public)
- Faster eviction (no flush needed)
- Better resource efficiency

---

## 16. Open Questions & Decisions Needed

### 16.1 Performance Budget
- **Q:** How much slower is acceptable for graceful shutdown?
- **A:** TBD - measure with 100 docs, estimate <1 second total

### 16.2 DoS Prevention
- **Q:** Should we handle invalid OTP rate limiting at app level or edge?
- **A:** Dual-check pattern (Pitfall #5) prevents worst case. ALB rate limiting recommended as additional layer.

### 16.3 Memory Limits
- **Q:** At what point do we need LRU eviction instead of time-based?
- **A:** Current plan supports 1,000 docs (~25MB). Revisit at 5,000+ docs.

### 16.4 Monitoring Depth
- **Q:** What metrics should we track for race conditions?
- **A:** See section 11.1 - focus on `active_persisters_count` vs `documents_with_users`

---

## 17. References

**Related Documents:**
- `memory-persistence-strategy.local.md` - Original strategy proposal
- `memory-extended.local.md` - Pitfall analysis and fixes
- `OT.local.md` - OT algorithm details (why history doesn't persist)
- `rustpad-BE.local.md` - Backend architecture overview

**External Resources:**
- SQLite concurrency: https://www.sqlite.org/faq.html#q5
- Go atomic package: https://pkg.go.dev/sync/atomic
- Context cancellation: https://go.dev/blog/context

---

## 18. Summary (TL;DR)

### What Changes:
✅ OTP stored in memory (eliminates DB reads on every connection)
✅ Persister runs only when users connected (saves resources)
✅ DB writes conditional (idle 30s OR safety 5min)
✅ Immediate flush on: last disconnect, shutdown, eviction, OTP change
✅ Dual-check OTP validation (prevents DoS)

### What Stays Same:
✅ Real-time collaboration speed (instant memory updates)
✅ Data consistency (CP from CAP theorem)
✅ Database schema (fully backward compatible)
✅ API contracts (no frontend changes needed)

### Expected Results:
✅ 97% reduction in DB operations
✅ 85% reduction in active goroutines
✅ 30s-5min data loss on crash (acceptable for ephemeral use case)
✅ Zero data loss on graceful shutdown

### Next Steps:
1. Review this specification with team
2. Implement Phase 1-7 (see section 10)
3. Test thoroughly (see section 13)
4. Deploy with monitoring (see section 11)
5. Iterate based on production metrics

---

**END OF SPECIFICATION**
