# Backend Broadcast System

**Purpose**: This document explains how the Kolabpad server broadcasts state changes to all connected clients, including the design decision to broadcast to senders and how user attribution works.

**Audience**: Backend and frontend developers working on real-time synchronization and understanding the WebSocket communication patterns.

---

## Table of Contents

1. [Broadcast Architecture](#broadcast-architecture)
2. [Broadcasting Pattern](#broadcasting-pattern)
3. [Message Types with User Attribution](#message-types-with-user-attribution)
4. [Why Broadcast to Sender?](#why-broadcast-to-sender)
5. [Rate Limiting and Backpressure](#rate-limiting-and-backpressure)
6. [Broadcast Ordering](#broadcast-ordering)

---

## Broadcast Architecture

The Kolabpad broadcast system follows a **broadcast-to-all** pattern where the server sends every state change to ALL connected clients, including the client that initiated the change.

### High-Level Flow

```
Client A                    Server                      Client B
   |                           |                           |
   |-- Edit Operation -------->|                           |
   |                           |                           |
   |                       [Apply Edit]                    |
   |                       [Update State]                  |
   |                           |                           |
   |<-- History (userId=A) ----|                           |
   |                           |-- History (userId=A) ---->|
   |                           |                           |
```

**Key Principle**: The server does NOT filter messages by userId. Every message goes to every connected client. Clients are responsible for filtering by `userId` to determine if they initiated the change.

### Why Broadcast to Everyone?

**Design Decision**: Broadcasting to all clients (including the sender) provides several advantages:

1. **Confirmation**: The sender knows their action succeeded when they receive the broadcast
2. **Consistency**: Same message handling logic for everyone—no special cases
3. **UX Differentiation**: Clients can show different UI based on whether they initiated the action
4. **Simplicity**: Server doesn't need filtering logic—just broadcast to all
5. **Race Avoidance**: No timing issues with "was this my action or someone else's?"

---

## Broadcasting Pattern

The server uses two broadcast mechanisms: **operation broadcasts** (for edits) and **metadata broadcasts** (for language, OTP, user info, cursors).

### Operation Broadcasts (Edit History)

Operations are broadcast using a **notify channel** pattern:

```pseudocode
WHEN client sends Edit operation:
    // Apply operation to document
    kolabpad.ApplyEdit(userId, revision, operation)

        INSIDE ApplyEdit (with state lock held):
            1. Transform operation against missed server operations
            2. Apply operation to document text
            3. Append to operations history
            4. Update lastEditTime (for idle detection)

            // Wake all connections (notify of new operation)
            close(notify channel)
            notify channel = create new channel

ALL CONNECTION LOOPS are waiting on notify channel:
    SELECT:
        CASE <-notifyChannel:
            // Channel closed - new operation available!
            newRevision = SendHistory(currentRevision)
```

**Why Close-and-Recreate Pattern?**

Closing a channel in Go immediately wakes ALL goroutines waiting on `<-channel`. This is an efficient way to wake multiple goroutines simultaneously. The alternative (sending N messages to N channels) would be slower and require tracking all connections.

### Metadata Broadcasts (Language, OTP, User Info, Cursors)

Metadata changes use **per-connection channels**:

```pseudocode
Kolabpad:
    subscribers: Map<userId → bufferedChannel<ServerMsg>>

FUNCTION Subscribe(userId) → channel:
    channel = create buffered channel (size: broadcastBufferSize)
    subscribers[userId] = channel
    RETURN channel

FUNCTION broadcast(message):
    FOR EACH channel IN subscribers:
        SELECT:
            CASE channel <- message:
                // Sent successfully
            DEFAULT:
                // Channel full - skip this client (non-blocking send)
```

**Why Non-Blocking Sends?**

If one client is slow to consume broadcasts (e.g., slow network, high latency), their channel buffer fills up. We skip sending to them rather than blocking the entire broadcast. The slow client will eventually timeout or catch up.

**Each Connection Handles Broadcasts**:

```pseudocode
CONNECTION LIFECYCLE:
    // Subscribe to metadata updates
    updates = kolabpad.Subscribe(userId)

    // Start broadcast forwarder goroutine
    START_GOROUTINE broadcastUpdates(updates):
        LOOP:
            SELECT:
                CASE message <- updates:
                    SendMessageToWebSocket(message)
                CASE <-connectionClosed:
                    RETURN
```

---

## Message Types with User Attribution

All metadata broadcasts include `userId` and `userName` fields to identify who initiated the change. This allows clients to show different UI for their own actions vs. others' actions.

### Language Change Broadcast

When a user changes the document's syntax highlighting language:

```pseudocode
CLIENT sends SetLanguage message:
    { "SetLanguage": "python" }

SERVER processes:
    userName = getUserName(userId)  // Look up from connected users
    kolabpad.SetLanguage("python", userId, userName)

        INSIDE SetLanguage:
            state.language = "python"
            lastEditTime = now()  // Track as edit for idle detection
            broadcast(LanguageMsg{
                language: "python",
                userId: userId,
                userName: userName
            })

ALL CLIENTS receive:
    {
        "Language": {
            "language": "python",
            "user_id": 1,
            "user_name": "Alice"
        }
    }

    IF message.userId == myUserId:
        showToast("Language updated to python")
    ELSE:
        showToast("Alice changed language to python")

    updateMonacoLanguage("python")  // Both cases: apply change
```

**Actual Message Structure** (from `internal/protocol/messages.go`):

```go
type LanguageMsg struct {
    Language string `json:"language"`
    UserID   uint64 `json:"user_id"`
    UserName string `json:"user_name"`
}
```

### OTP Protection Broadcast

When a user enables or disables OTP protection:

```pseudocode
CLIENT calls REST API:
    POST /api/document/abc123/protect
    Body: { "user_id": 2, "user_name": "Bob" }

SERVER processes:
    1. Generate OTP token
    2. Write to database (critical write)
    3. Update memory (kolabpad.SetOTP)
    4. Broadcast to all clients

ALL CLIENTS receive:
    {
        "OTP": {
            "otp": "abc123",  // or null if disabling
            "user_id": 2,
            "user_name": "Bob"
        }
    }

    IF message.userId == myUserId:
        IF message.otp is not null:
            showToast("OTP protection enabled")
            updateURL("?otp=" + message.otp)
        ELSE:
            showToast("OTP protection disabled")
            removeOTPFromURL()
    ELSE:
        IF message.otp is not null:
            showToast("Bob enabled OTP protection")
            updateURL("?otp=" + message.otp)
        ELSE:
            showToast("Bob disabled OTP protection")
            removeOTPFromURL()
```

**Actual Message Structure**:

```go
type OTPMsg struct {
    OTP      *string `json:"otp"`       // nil when disabling
    UserID   uint64  `json:"user_id"`
    UserName string  `json:"user_name"`
}
```

**Why Broadcast OTP Token?**

When protection is enabled, all connected users need to know the OTP so they can share the updated URL with others. The broadcast includes the OTP so clients can update the URL bar automatically.

**Security Note**: If an attacker is already connected to the document when protection is enabled, they'll receive the OTP. This is acceptable because:
1. They were already collaborating (had access)
2. The threat model protects against *new* unauthorized users
3. Users can disconnect all others by reloading the document with the new OTP

### User Info Broadcast (Join/Leave)

When a user connects or disconnects:

```pseudocode
USER CONNECTS:
    CLIENT sends ClientInfo message:
        { "ClientInfo": { "name": "Charlie", "hue": 270 } }

    SERVER processes:
        kolabpad.SetUserInfo(userId, userInfo)

            INSIDE SetUserInfo:
                state.users[userId] = userInfo
                broadcast(UserInfoMsg{
                    id: userId,
                    info: &userInfo
                })

    ALL OTHER CLIENTS receive (NOT sender):
        {
            "UserInfo": {
                "id": 3,
                "info": {
                    "name": "Charlie",
                    "hue": 270
                }
            }
        }

        addUserToSidebar(userId, userInfo)

USER DISCONNECTS:
    SERVER processes:
        kolabpad.RemoveUser(userId)

            INSIDE RemoveUser:
                DELETE state.users[userId]
                DELETE state.cursors[userId]
                broadcast(UserInfoMsg{
                    id: userId,
                    info: nil  // nil indicates disconnection
                })

    ALL REMAINING CLIENTS receive:
        {
            "UserInfo": {
                "id": 3,
                "info": null
            }
        }

        removeUserFromSidebar(userId)
```

**Special Case**: UserInfo messages are NOT sent to the sender when they first connect (they already know their own info). They ARE broadcast to other users when someone connects or updates their info.

### Cursor Position Broadcast

When a user moves their cursor or changes their selection:

```pseudocode
CLIENT sends CursorData message (debounced to 20ms):
    {
        "CursorData": {
            "cursors": [42],
            "selections": [[10, 25], [30, 35]]
        }
    }

SERVER processes:
    kolabpad.SetCursorData(userId, cursorData)

        INSIDE SetCursorData:
            state.cursors[userId] = cursorData
            broadcast(UserCursorMsg{
                id: userId,
                data: cursorData
            })

ALL OTHER CLIENTS receive:
    {
        "UserCursor": {
            "id": 1,
            "data": {
                "cursors": [42],
                "selections": [[10, 25]]
            }
        }
    }

    updateRemoteCursorDecoration(userId, cursorData)
```

**Note**: Cursor positions are Unicode codepoint offsets, not byte offsets or UTF-16 offsets. This matches the OT algorithm's indexing scheme.

### Edit Operation Broadcast (History)

Edit operations include `userId` per operation, not per message:

```pseudocode
CLIENT sends Edit:
    { "Edit": { "revision": 5, "operation": [10, "hello"] } }

SERVER processes:
    1. Transform against missed operations
    2. Apply to document
    3. Append to operations history WITH userId

ALL CLIENTS receive:
    {
        "History": {
            "start": 5,
            "operations": [
                {
                    "id": 0,  // userId who created this operation
                    "operation": [10, "hello"]
                }
            ]
        }
    }

    FOR EACH operation IN message.operations:
        IF operation.id == myUserId:
            // This is my operation echoed back - acknowledge
            clearPendingBuffer()
        ELSE:
            // Someone else's operation - transform and apply
            transformed = transform(operation, pendingOperations)
            applyToEditor(transformed)
```

**Why userId Per Operation?**

A single History message can contain multiple operations from different users. Each operation needs attribution so clients can handle them correctly.

---

## Why Broadcast to Sender?

The server broadcasts ALL messages to ALL clients, including the client that initiated the change. This design has several important benefits:

### 1. Confirmation of Success

```pseudocode
USER clicks "Enable OTP" button:
    → Frontend sends POST /api/document/{id}/protect
    → Server processes (generates OTP, writes to DB)
    → Server broadcasts OTP message to ALL clients
    → Frontend receives OTP broadcast (userId == myUserId)
    → Frontend shows "OTP protection enabled" toast
    → Frontend updates URL with ?otp=xyz
```

Without broadcasting to sender, the frontend would have to:
- Show success based on HTTP 200 response (but broadcast might fail)
- Handle race between HTTP response and WebSocket broadcast
- Have two code paths: one for "I did this" and one for "someone else did this"

### 2. Consistent Message Handling

```pseudocode
// ONE CODE PATH FOR ALL CLIENTS:
ON_RECEIVE LanguageMsg:
    IF msg.userId == myUserId:
        showToast("Language updated to " + msg.language)
    ELSE:
        showToast(msg.userName + " changed language to " + msg.language)

    // BOTH CASES: Apply the change
    updateMonacoLanguage(msg.language)
```

vs. without broadcasting to sender:

```pseudocode
// TWO CODE PATHS:

// When I change language:
ON_CLICK languageSelector:
    sendSetLanguageMessage(newLanguage)
    updateMonacoLanguage(newLanguage)  // Apply locally
    showToast("Language updated")

// When someone else changes language:
ON_RECEIVE LanguageMsg:
    updateMonacoLanguage(msg.language)  // Apply remotely
    showToast(msg.userName + " changed language")
```

The dual code path approach is error-prone (easy to forget to update both paths) and leads to bugs.

### 3. Better UX Differentiation

Broadcasting to sender enables different toast messages:

```
YOU enable OTP:     "OTP protection enabled"
ALICE enables OTP:  "Alice enabled OTP protection"

YOU change language: "Language updated to Python"
BOB changes language: "Bob changed language to Python"
```

This gives users clarity about who is doing what in the collaborative session.

### 4. Race Condition Avoidance

Without broadcasting to sender, the client must track "did I just send this action?" using flags:

```pseudocode
// PROBLEMATIC APPROACH:
ignoreBroadcastFlag = false

FUNCTION handleUserChangesLanguage():
    ignoreBroadcastFlag = true
    sendSetLanguageMessage()

ON_RECEIVE LanguageMsg:
    IF ignoreBroadcastFlag:
        ignoreBroadcastFlag = false
        RETURN  // Ignore, assuming this is my echo

    // Apply change...

PROBLEM:
    - What if my action fails? Flag is set but no broadcast comes
    - What if network delays cause broadcasts to arrive out of order?
    - What if someone else changes language at the same time?
```

With `userId` attribution, this is deterministic:

```pseudocode
// ROBUST APPROACH:
ON_RECEIVE LanguageMsg:
    IF msg.userId == myUserId:
        // Definitely my action
    ELSE:
        // Definitely someone else's action
```

---

## Rate Limiting and Backpressure

The broadcast system handles slow clients gracefully to prevent one slow client from blocking everyone.

### WebSocket Write Buffer

```pseudocode
CONNECTION write timeout: 10 seconds (configurable)

FUNCTION send(message):
    writeContext = CreateContextWithTimeout(writeTimeout)
    TRY:
        connection.Write(writeContext, message)
    CATCH timeout:
        LOG_ERROR "Client %d timed out, disconnecting"
        CloseConnection()
```

**Design Decision**: If a client can't consume messages within 10 seconds, they're too slow and get disconnected. This prevents their slow network from blocking the server.

### Non-Blocking Broadcast Sends

```pseudocode
BROADCAST PATTERN:
    subscribers: Map<userId → bufferedChannel>
    broadcastBufferSize = 16  // Configurable

    FUNCTION broadcast(message):
        FOR EACH channel IN subscribers:
            SELECT:
                CASE channel <- message:
                    // Sent successfully
                DEFAULT:
                    // Channel full - skip
                    LOG_WARNING "Skipped broadcast to user %d (buffer full)"
```

**Why Non-Blocking?**

If a client's goroutine is stuck (e.g., waiting for network write), their channel buffer fills up. We skip sending to them rather than blocking the broadcast. They'll eventually timeout and disconnect.

**Buffer Size Tuning**:
- **Default: 16 messages**: Sufficient for normal operation
- **Too small (e.g., 1)**: Risk of dropped messages if client has momentary delay
- **Too large (e.g., 1000)**: Wastes memory, delays feedback to slow clients

### Cursor Update Throttling

Cursor updates are throttled client-side to prevent flooding the server:

```pseudocode
FRONTEND PATTERN:
    lastCursorSendTime = 0
    throttleInterval = 20 milliseconds

    ON_EDITOR cursor or selection change:
        now = CurrentTime()
        IF now - lastCursorSendTime < throttleInterval:
            RETURN  // Throttled

        lastCursorSendTime = now
        sendCursorDataMessage(cursors, selections)
```

**Why 20ms?**

- **User perception**: 20ms is imperceptible (< 50ms refresh rate)
- **Network efficiency**: 50 updates/second maximum per user
- **Server load**: With 10 users, 500 cursor updates/second total (manageable)

### Edit Operation Batching

Edit operations are composed on the client before sending:

```pseudocode
FRONTEND PATTERN:
    pendingOperations = []

    ON_EDITOR content change:
        operation = CreateOperationFromChange()
        pendingOperations = compose(pendingOperations, operation)

        debounced(100ms):
            composedOp = composePendingOperations()
            sendEditMessage(composedOp)
            pendingOperations = []
```

**Why Compose?**

Instead of sending 100 individual character inserts, compose them into one operation:
- `[0, "h"]` + `[1, "e"]` + `[2, "l"]` + `[3, "l"]` + `[4, "o"]`
- Becomes: `[0, "hello"]`

This reduces network traffic and server processing load dramatically.

---

## Broadcast Ordering

### Operations Are Ordered

```pseudocode
GUARANTEE: Operations are broadcast in revision order

ENFORCEMENT:
    1. Operations are appended to state.operations (sequential)
    2. History messages send operations from start revision
    3. Connections track their current revision
    4. Notify channel ensures connections check for new operations

RESULT: All clients see operations in the same order
```

**Why This Matters**: The OT algorithm requires operations to be processed in the same order on all clients for convergence guarantees.

### Metadata Is NOT Strictly Ordered

```pseudocode
NO GUARANTEE: Language/OTP/UserInfo broadcasts may arrive in any order relative to operations

ACCEPTABLE BECAUSE:
    - Language changes don't affect document correctness
    - OTP changes don't affect document correctness
    - User info changes don't affect document correctness

EXAMPLE:
    Client A sends Edit at revision 5
    Client B changes language at revision 5

    Client C might receive:
        - Language change, then Edit operation
        OR
        - Edit operation, then Language change

    Both orders are fine - language doesn't affect text
```

**Design Decision**: We don't enforce total ordering across all message types because it's unnecessary and would add complexity. Operations have causal ordering (enforced by revisions), but metadata doesn't need it.

---

## Related Documentation

- [backend/01-server-architecture.md] - Server structure and concurrency model
- [protocol/01-websocket-protocol.md] - Complete WebSocket message reference
- [frontend/02-state-synchronization.md] - How frontend handles broadcasts
- [architecture/02-operational-transformation.md] - OT algorithm and operation ordering

---

## Summary

The Kolabpad broadcast system is designed for **simplicity** and **robustness**:

1. **Broadcast to all**: Server doesn't filter by userId—all clients receive all messages
2. **User attribution**: Every metadata message includes `userId` and `userName` for client-side filtering
3. **Confirmation**: Senders receive their own actions as confirmation of success
4. **Non-blocking**: Slow clients don't block fast clients—channels have buffers and timeouts
5. **Ordered operations**: Edit operations are strictly ordered by revision number
6. **Unordered metadata**: Language/OTP/user info broadcasts are eventually consistent

This design provides a responsive, real-time collaborative experience while gracefully handling network variability and slow clients.
