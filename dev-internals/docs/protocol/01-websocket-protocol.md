# WebSocket Protocol

**Purpose**: Complete reference for WebSocket messages exchanged between client and server in Kolabpad.

**Audience**: Developers implementing clients, debugging communication issues, or extending the protocol.

---

## Table of Contents

1. [Protocol Overview](#protocol-overview)
2. [Connection Flow](#connection-flow)
3. [Message Format](#message-format)
4. [Client → Server Messages](#client--server-messages)
5. [Server → Client Messages](#server--client-messages)
6. [Message Flow Examples](#message-flow-examples)
7. [Standard Broadcast Pattern](#standard-broadcast-pattern)
8. [Operation Format](#operation-format)
9. [Reconnection and Resilience](#reconnection-and-resilience)
10. [Performance Considerations](#performance-considerations)

---

## Protocol Overview

Kolabpad uses WebSocket for real-time bidirectional communication between clients and server.

**Key Characteristics**:
- **Format**: JSON messages over WebSocket
- **Pattern**: Tagged union (only one field set per message)
- **Latency**: Typically 1-50ms for local networks
- **Compression**: Disabled (small messages, compression overhead not worth it)
- **Upgrades from**: HTTP/HTTPS via standard WebSocket upgrade handshake

**Why WebSocket Instead of HTTP Polling**:
- Lower latency: No request/response overhead
- Lower bandwidth: Persistent connection, no HTTP headers per message
- Real-time: Server can push updates instantly
- Efficient: Avoids polling overhead

**Trade-offs**:
- More complex: Requires WebSocket library
- Stateful: Server must track active connections
- Not cacheable: Can't use HTTP caching mechanisms

---

## Connection Flow

```
Client                                    Server
  |                                          |
  |--- HTTP Upgrade Request ---------------->|
  |    GET /api/socket/{docId}?otp={token}  |
  |    Upgrade: websocket                    |
  |                                          |
  |<-- HTTP 101 Switching Protocols ---------|
  |    (WebSocket connection established)    |
  |                                          |
  |<-- Identity ----------------------------|
  |    { "Identity": 0 }                    |
  |    (Assigns user ID to this client)     |
  |                                          |
  |<-- History ------------------------------|
  |    { "History": {...} }                 |
  |    (Full operation history)             |
  |                                          |
  |<-- Language -----------------------------|
  |    { "Language": {...} }                |
  |    (Current document language)          |
  |                                          |
  |<-- OTP (if protected) -------------------|
  |    { "OTP": {...} }                     |
  |    (Protection status)                  |
  |                                          |
  |<-- UserInfo (for each user) -------------|
  |    { "UserInfo": {...} }                |
  |    (All connected users)                |
  |                                          |
  |--- ClientInfo -------------------------->|
  |    { "ClientInfo": {...} }              |
  |    (Send own name and color)            |
  |                                          |
  |<--> Ongoing collaboration -------------->|
  |    Edit, SetLanguage, CursorData...     |
  |                                          |
```

**Initial Sync Sequence**:

The server sends initial state in a specific order to ensure clients are fully synchronized:

```pseudocode
ON client connects:
    1. Send Identity message    → Assign unique user ID
    2. Send History message     → All operations from revision 0
    3. Send Language message    → Current syntax highlighting language
    4. Send OTP message         → Protection status (if OTP exists)
    5. FOR EACH connected user:
         Send UserInfo message  → User's name and color
    6. FOR EACH user with cursor data:
         Send UserCursor message → Cursor positions

    CLIENT now fully synchronized and ready for collaboration
```

---

## Message Format

All messages use a **tagged union** pattern: only one field is set per message.

**Example**:
```json
{
  "Edit": {
    "revision": 42,
    "operation": [10, "hello"]
  }
}
```

**NOT**:
```json
{
  "Edit": {...},
  "SetLanguage": "python"  // INVALID - only one field allowed
}
```

**Why Tagged Union**:
- Type safety: Clear message type identification
- Easy parsing: Check which field is present
- Extensible: Add new message types without breaking existing clients
- Wire-compatible: Matches Rustpad protocol exactly

---

## Client → Server Messages

All client messages are wrapped in a `ClientMsg` envelope with exactly one field set.

### 1. Edit

**Purpose**: Apply a text editing operation to the document.

**Format**:
```json
{
  "Edit": {
    "revision": 42,
    "operation": [10, "hello"]
  }
}
```

**Fields**:
- `revision` (integer): Client's current revision number
- `operation` (array): OT operation in compact format (see [Operation Format](#operation-format))

**When Sent**:
- User types, deletes, or pastes text
- Operations are composed/batched by OT library for efficiency

**Server Response**:
- Transforms operation if client is behind server revision
- Applies operation to document
- Broadcasts `History` message to ALL clients (including sender)

---

### 2. SetLanguage

**Purpose**: Change the document's syntax highlighting language.

**Format**:
```json
{
  "SetLanguage": "python"
}
```

**Fields**:
- Value (string): Monaco language ID (e.g., "javascript", "python", "go", "plaintext")

**When Sent**:
- User selects language from dropdown
- On document load (to sync with server's language)

**Server Response**:
- Updates document language in memory
- Broadcasts `Language` message to ALL clients

**Supported Languages**:
- Determined by Monaco editor's language registry
- Common: javascript, typescript, python, go, rust, java, c, cpp, html, css, markdown, json, yaml

---

### 3. ClientInfo

**Purpose**: Update this client's display name and color.

**Format**:
```json
{
  "ClientInfo": {
    "name": "Alice",
    "hue": 180
  }
}
```

**Fields**:
- `name` (string): Display name shown to other users
- `hue` (integer 0-359): Color hue for cursor and selections

**When Sent**:
- Immediately after receiving `Identity` message
- When user changes their name
- When user's color changes (e.g., collision avoidance)

**Server Response**:
- Stores user info in memory
- Broadcasts `UserInfo` message to OTHER clients (not sender)

**Color Collision**:
- Frontend handles collision detection (see `frontend/01-frontend-architecture.md`)
- If two users have same hue, one client automatically changes

---

### 4. CursorData

**Purpose**: Update this client's cursor position and selections.

**Format**:
```json
{
  "CursorData": {
    "cursors": [42],
    "selections": [[10, 25], [30, 35]]
  }
}
```

**Fields**:
- `cursors` (array of integers): Cursor positions in Unicode codepoint offsets
- `selections` (array of [start, end] pairs): Selection ranges in codepoint offsets

**When Sent**:
- Debounced to every 20ms max (prevent flooding)
- When user moves cursor
- When user makes selections

**Server Response**:
- Stores cursor data in memory
- Broadcasts `UserCursor` message to OTHER clients (not sender)

**Codepoint Offsets**:
- Positions counted in Unicode codepoints, not bytes
- Emoji = 1 codepoint, not 2-4 bytes
- Frontend must convert between Monaco's UTF-16 offsets and codepoint offsets

---

## Server → Client Messages

All server messages are wrapped in a `ServerMsg` envelope with exactly one field set.

### 1. Identity

**Purpose**: Assign a unique user ID to this client.

**Format**:
```json
{
  "Identity": 0
}
```

**Fields**:
- Value (integer): User ID assigned to this client (starts from 0, increments)

**When Sent**:
- First message after WebSocket connection established
- Each new connection gets a unique ID

**Client Action**:
- Store user ID for filtering broadcasts
- Send `ClientInfo` with name and color

**Special Note**:
- This is the ONLY server message without `user_id`/`user_name` fields
- Reason: You're being assigned your ID, so you don't have one yet

---

### 2. History

**Purpose**: Send a batch of operations to sync client state.

**Format**:
```json
{
  "History": {
    "start": 5,
    "operations": [
      { "id": 0, "operation": [10, "hello"] },
      { "id": 1, "operation": [15, " world"] },
      { "id": 2, "operation": [21, -5] }
    ]
  }
}
```

**Fields**:
- `start` (integer): Starting revision number
- `operations` (array): List of user operations

**User Operation Structure**:
- `id` (integer): User ID who created this operation
- `operation` (array): OT operation in compact format

**When Sent**:
- Initial sync: Full history from revision 0 to current
- After each Edit: Broadcast single operation to all clients
- Catch-up: If client reconnects, send missed operations

**Client Action**:
```pseudocode
FOR EACH operation IN history.operations:
    IF operation.id == myUserId:
        // This is my operation echoed back
        acknowledge()  // Clear pending buffer
    ELSE:
        // Someone else's operation
        transform against pending operations
        apply to local document
        update editor
```

**Why Broadcast to Sender**:
- Confirmation: Sender knows their operation succeeded
- Consistency: Same handling logic for all clients
- Simplicity: Server doesn't filter by sender

---

### 3. Language

**Purpose**: Broadcast language change to all clients.

**Format**:
```json
{
  "Language": {
    "language": "python",
    "user_id": 1,
    "user_name": "Alice"
  }
}
```

**Fields**:
- `language` (string): New language mode
- `user_id` (integer): User who initiated the change
- `user_name` (string): User's display name

**When Sent**:
- After any client sends `SetLanguage`
- During initial sync (current language)

**Client Action**:
```pseudocode
IF broadcast.user_id == myUserId:
    show toast: "Language updated to {language}"
ELSE:
    show toast: "{user_name} changed language to {language}"

// Both cases: apply the change
editor.setLanguage(language)
```

**Attribution Benefits**:
- UX: Different messages for self vs others
- Awareness: Know who changed the language
- Conflict prevention: See if someone else is changing settings

---

### 4. OTP

**Purpose**: Broadcast OTP protection status change.

**Format** (when enabled):
```json
{
  "OTP": {
    "otp": "abc123",
    "user_id": 2,
    "user_name": "Bob"
  }
}
```

**Format** (when disabled):
```json
{
  "OTP": {
    "otp": null,
    "user_id": 2,
    "user_name": "Bob"
  }
}
```

**Fields**:
- `otp` (string or null): OTP token if enabled, null if disabled
- `user_id` (integer): User who made the change
- `user_name` (string): User's display name

**When Sent**:
- After REST API call to `POST /api/document/{id}/protect`
- After REST API call to `DELETE /api/document/{id}/protect`
- During initial sync (if document is protected)

**Client Action**:
```pseudocode
IF broadcast.user_id == myUserId:
    IF broadcast.otp != null:
        show toast: "OTP protection enabled"
        update URL: add ?otp={otp} query parameter
    ELSE:
        show toast: "OTP protection disabled"
        update URL: remove ?otp query parameter
ELSE:
    IF broadcast.otp != null:
        show toast: "{user_name} enabled OTP protection"
    ELSE:
        show toast: "{user_name} disabled OTP protection"
```

**Security Note**:
- OTP is broadcast to ALL connected clients (they're already authenticated)
- This allows all clients to update their URLs with the OTP
- See `security/01-authentication-model.md` for security details

---

### 5. UserInfo

**Purpose**: Broadcast user connection/disconnection events.

**Format** (user joined):
```json
{
  "UserInfo": {
    "id": 3,
    "info": {
      "name": "Charlie",
      "hue": 270
    }
  }
}
```

**Format** (user left):
```json
{
  "UserInfo": {
    "id": 3,
    "info": null
  }
}
```

**Fields**:
- `id` (integer): User ID
- `info` (object or null): User's name and hue, or null if disconnected

**When Sent**:
- When user sends `ClientInfo` (broadcast to others)
- When user disconnects (broadcast to all)
- During initial sync (for each connected user)

**Server Logic**:
```pseudocode
ON client sends ClientInfo:
    store info in document.users[userId]

    // Broadcast to ALL OTHER users (not sender)
    FOR EACH connection WHERE connection.userId != sender.userId:
        send UserInfo message
```

**Client Action**:
```pseudocode
IF broadcast.info != null:
    // User joined or updated info
    users[broadcast.id] = broadcast.info
    show toast: "{info.name} joined"
ELSE:
    // User left
    delete users[broadcast.id]
    show toast: "{previous_name} left"
```

---

### 6. UserCursor

**Purpose**: Broadcast cursor position update.

**Format**:
```json
{
  "UserCursor": {
    "id": 1,
    "data": {
      "cursors": [42],
      "selections": [[10, 25]]
    }
  }
}
```

**Fields**:
- `id` (integer): User ID
- `data` (object): Cursor and selection data (same as `CursorData`)

**When Sent**:
- When user sends `CursorData` (broadcast to others)
- During initial sync (for each user with cursor data)

**Server Logic**:
- Stores cursor data in memory
- Broadcasts to OTHER clients (not sender)

**Client Action**:
```pseudocode
userCursors[broadcast.id] = broadcast.data

// Update Monaco editor decorations
decorations = []
FOR EACH userId, cursorData IN userCursors:
    IF userId != myUserId:
        user = users[userId]
        FOR EACH position IN cursorData.cursors:
            decorations.add({
                range: position to position
                className: "remote-cursor-{user.hue}"
            })
        FOR EACH [start, end] IN cursorData.selections:
            decorations.add({
                range: start to end
                className: "remote-selection-{user.hue}"
            })

editor.deltaDecorations(oldDecorations, decorations)
```

---

## Message Flow Examples

### Example 1: User Types Text

```
Scenario: Alice (user 0) types "hello" at position 10

Client (Alice):
    User types "hello"
    → Compose operation: [10, "hello"]
    → Send: { "Edit": { "revision": 5, "operation": [10, "hello"] } }

Server:
    Receive Edit from user 0
    → Current revision: 5 (matches client)
    → Apply operation to document
    → Increment revision to 6
    → Broadcast to ALL clients:
      { "History": {
          "start": 5,
          "operations": [{ "id": 0, "operation": [10, "hello"] }]
      }}

Client (Alice):
    Receive History
    → operation.id == 0 (my ID)
    → Acknowledge: clear pending buffer
    → (Already applied locally, no editor update needed)

Client (Bob, user 1):
    Receive History
    → operation.id == 0 (not my ID)
    → Transform against my pending operations (if any)
    → Apply to local document: insert "hello" at position 10
    → Update Monaco editor
```

### Example 2: User Changes Language

```
Scenario: Bob (user 1, name "Bob") changes language to Go

Client (Bob):
    User selects "go" from language dropdown
    → Send: { "SetLanguage": "go" }

Server:
    Receive SetLanguage from user 1
    → Update document.language = "go"
    → Broadcast to ALL clients:
      { "Language": {
          "language": "go",
          "user_id": 1,
          "user_name": "Bob"
      }}

Client (Bob):
    Receive Language broadcast
    → user_id == 1 (my ID)
    → Show toast: "Language updated to go"
    → Monaco: editor.setLanguage("go")

Client (Alice):
    Receive Language broadcast
    → user_id == 1 (not my ID)
    → Show toast: "Bob changed language to go"
    → Monaco: editor.setLanguage("go")
```

### Example 3: Concurrent Edits with OT

```
Scenario: Alice and Bob edit simultaneously

Initial state (revision 10): "Hello world"
Alice: Insert "beautiful " at position 6
Bob: Delete "world" (5 chars) at position 6

Client (Alice):
    → Send: { "Edit": { "revision": 10, "operation": [6, "beautiful "] } }
    → Apply locally: "Hello beautiful world"
    → Mark operation as outstanding (awaiting server confirmation)

Client (Bob):
    → Send: { "Edit": { "revision": 10, "operation": [6, -5] } }
    → Apply locally: "Hello "
    → Mark operation as outstanding

Server (receives Alice's edit first):
    → revision 10 matches
    → Apply: "Hello beautiful world"
    → Increment to revision 11
    → Broadcast: { "History": { "start": 10, "operations": [{"id": 0, "operation": [6, "beautiful "]}] }}

Server (receives Bob's edit):
    → revision 10 < server revision 11
    → Transform Bob's [6, -5] against Alice's [6, "beautiful "]
    → Result: [16, -5] (delete shifted by "beautiful ")
    → Apply: "Hello beautiful "
    → Increment to revision 12
    → Broadcast: { "History": { "start": 11, "operations": [{"id": 1, "operation": [16, -5]}] }}

Client (Alice):
    Receive History (rev 11, Alice's operation)
    → operation.id == 0 (my ID)
    → Acknowledge, clear outstanding

    Receive History (rev 12, Bob's transformed operation)
    → operation.id == 1 (not my ID)
    → Apply transformed [16, -5]
    → Result: "Hello beautiful "

Client (Bob):
    Receive History (rev 11, Alice's operation)
    → operation.id == 0 (not my ID)
    → Transform against outstanding: [6, -5] ⨁ [6, "beautiful "] = [6, -5], [6, "beautiful "]
    → Apply Alice's operation: "Hello beautiful "
    → Update outstanding to [16, -5]

    Receive History (rev 12, Bob's operation)
    → operation.id == 1 (my ID)
    → Acknowledge, clear outstanding
    → Result: "Hello beautiful "

Final state (both clients): "Hello beautiful "
```

---

## Standard Broadcast Pattern

Most state-change broadcasts follow this standard pattern:

```json
{
  "MessageType": {
    "user_id": number,      // Who initiated the change
    "user_name": string,    // User's display name
    // ... message-specific fields
  }
}
```

**Messages using this pattern**:
- `Language`
- `OTP`

**Exceptions**:
- `Identity`: No userId (you're being assigned yours)
- `History`: userId per operation, not per message
- `UserInfo`: Only has userId (`id` field)
- `UserCursor`: Only has userId (`id` field)

**Why User Attribution**:
1. **Better UX**: Different toast messages for self vs others
2. **Awareness**: See who made changes
3. **Conflict prevention**: Avoid simultaneous conflicting actions
4. **Audit trail**: Track who did what (for future features)

---

## Operation Format

OT operations use a compact JSON array format for efficiency.

**Operation Components**:
- **Retain(n)**: Positive integer → Keep n characters
- **Delete(n)**: Negative integer → Delete n characters
- **Insert(s)**: String → Insert text

**Examples**:

```json
// Insert "hello" at start of document
["hello"]

// Insert "hello" at position 10
[10, "hello"]

// Delete 5 characters at position 3
[3, -5]

// Complex: Retain 5, Insert "hello", Delete 3, Retain 10
[5, "hello", -3, 10]

// Replace word: Delete 5, Insert "world"
[10, -5, "world"]
```

**Serialization Rules**:
```pseudocode
Retain(n) → positive integer n
Delete(n) → negative integer -n
Insert(s) → string "s"
```

**Why This Format**:
- Compact: Minimal bytes over wire
- Simple: Easy to parse and debug
- Efficient: No field names, just values
- Compatible: Matches Rustpad protocol exactly

**Unicode Handling**:
- All positions are **Unicode codepoint offsets**, not byte offsets
- Emoji count as 1 codepoint
- Example: "Hello 👋 World" has codepoints: H=0, e=1, l=2, l=3, o=4, (space)=5, 👋=6, (space)=7, W=8, ...

---

## Reconnection and Resilience

### Automatic Reconnection

```pseudocode
ON WebSocket close:
    IF intentional (user navigated away):
        don't reconnect
        return

    IF never connected before:
        wait exponentialBackoff(attempts)
    ELSE:
        wait RECONNECT_INTERVAL (default 2 seconds)

    tryConnect()
```

**Backoff Strategy**:
- Recent failures counter resets every 10 seconds
- First failure: immediate retry
- Subsequent failures: exponential backoff (capped at 30 seconds)

### State Recovery on Reconnect

```pseudocode
ON successful reconnect:
    // Server sends full state
    1. Receive Identity (new user ID)
    2. Receive History (full operation history)
    3. Receive Language
    4. Receive OTP (if protected)
    5. Receive UserInfo (all users)
    6. Receive UserCursor (all cursors)

    // Client reconciles
    IF have pending operations:
        compose pending operations
        send as new Edit (with server's latest revision)

    resend ClientInfo (with name and color)
```

**Pending Operations**:
- Client buffers local edits while disconnected
- On reconnect: send buffered operations as single Edit
- OT transform ensures consistency with server state

### Connection States

```pseudocode
"connected":      WebSocket open, Identity received
"disconnected":   WebSocket closed, trying to reconnect
"desynchronized": Connected but state mismatch detected
```

**Desynchronization Detection**:
- Rare edge case (server restart mid-session)
- Client detects if revision jumps unexpectedly
- Trigger: full page reload to resync

---

## Performance Considerations

### Message Size Limits

**WebSocket Read Limit**: maxDocumentSize + 64KB overhead
- Default maxDocumentSize: 256KB
- Total limit: ~320KB per message
- Prevents: DoS via large message attacks

**Why This Limit**:
- Allows: Full document in single History message
- Prevents: Memory exhaustion
- Trade-off: Documents larger than 256KB will be rejected

### Bandwidth Optimization

**1. Cursor Updates**: Debounced to 20ms
```pseudocode
ON cursor move:
    scheduleDebounced(sendCursorData, 20ms)
```

**2. Edit Operations**: Composed/batched
```pseudocode
ON user types rapidly:
    compose multiple Inserts into single operation
    Example: "h" + "e" + "l" + "l" + "o" → [0, "hello"]
```

**3. History Messages**: Can contain multiple operations
```pseudocode
// Efficient catch-up: send 100 operations in one message
{
  "History": {
    "start": 0,
    "operations": [
      {"id": 0, "operation": [...]},
      {"id": 1, "operation": [...]},
      // ... 98 more operations
    ]
  }
}
```

### Latency Characteristics

**Typical Round-Trip Times**:
- Local network: 1-5ms
- Same region cloud: 10-50ms
- Cross-region cloud: 50-200ms
- Intercontinental: 100-500ms

**Optimistic UI**:
- Client applies edits immediately (don't wait for server)
- Appears instant to user (0ms perceived latency)
- Server confirmation arrives 10-50ms later

---

## Related Documentation

- **OT Algorithm**: See `architecture/02-operational-transformation.md` for transform details
- **REST API**: See `protocol/02-rest-api.md` for HTTP endpoints (OTP protection, stats)
- **Backend Implementation**: See `backend/01-server-architecture.md` for server-side handling
- **Frontend Integration**: See `frontend/02-state-synchronization.md` for client-side logic
- **Security**: See `security/01-authentication-model.md` for OTP validation details

---

## Implementation References

**Backend**:
- Message definitions: `internal/protocol/messages.go`
- WebSocket handling: `pkg/server/connection.go`
- Broadcast logic: `pkg/server/kolabpad.go`

**Frontend**:
- Message types: `frontend/src/types/kolabpad.ts`
- WebSocket client: `frontend/src/services/kolabpad.ts`
- Message handling: Same file, `onMessage` handler

**OT Format**:
- Serialization: [serde.go](https://github.com/shiv248/operational-transformation-go/blob/main/serde.go)
- Operation structure: [operation.go](https://github.com/shiv248/operational-transformation-go/blob/main/operation.go)
