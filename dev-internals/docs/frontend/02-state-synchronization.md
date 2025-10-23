# State Synchronization

## Purpose

This document explains how document state resets work, how broadcasts are synchronized, and how the frontend maintains consistency with the server. Understanding these patterns is critical for implementing new features that involve state changes.

**Target Audience**: Frontend developers implementing new broadcast types or state synchronization features.

---

## The State Reset Problem

### The Challenge

When a user switches documents (URL hash changes from `#abc123` to `#xyz789`), we need to reset **all** document-specific state:

- Clear list of connected users
- Disconnect from old document's WebSocket
- Clear Monaco editor content
- Reset language to new document's language
- Clear OTP protection state
- Clear remote cursor decorations
- Reset connection status
- Clear any pending operations

**Critical requirement**: We cannot forget to reset ANY piece of state, or we risk showing stale data from the previous document in the new document.

### The Old Approach (Manual Resets)

```pseudocode
# BAD: Manual state resets - fragile and error-prone

STATE users = {}
STATE myUserId = null
STATE language = "plaintext"
STATE otpBroadcast = undefined
STATE connection = "disconnected"
STATE editor = null
# ... many more state variables

FUNCTION switchDocument(newDocId):
    # Must remember to reset every single variable
    setUsers({})
    setMyUserId(null)
    setLanguage("plaintext")
    setOtpBroadcast(undefined)
    setConnection("disconnected")
    setLanguageBroadcast(undefined)
    # ... easy to forget one!

    # Must remember to clean up side effects
    websocket?.close()
    editor?.clear()
    clearDecorations()
    # ... easy to forget cleanup

    # Only then start new document
    connectToDocument(newDocId)
```

**Problems**:
1. **Easy to forget**: 10+ state variables, easy to forget one
2. **No compile-time safety**: TypeScript can't help detect missing resets
3. **Hard to test**: Must verify every reset happens correctly
4. **Maintenance burden**: Adding new state requires remembering to reset it
5. **Stale closures**: WebSocket callbacks might capture old state
6. **Race conditions**: Async cleanups might complete after new document starts

### The New Approach (Remounting)

```pseudocode
# GOOD: Automatic state resets via React key prop

COMPONENT App:
    documentId = useHash()  # e.g., "abc123" from #abc123

    RENDER:
        <SessionProvider>
            <DocumentProvider key={documentId} documentId={documentId}>
                <DocumentEditor />
            </DocumentProvider>
        </SessionProvider>
```

**Magic**: The `key={documentId}` prop tells React to completely unmount and remount `DocumentProvider` when `documentId` changes.

**What happens when documentId changes**:
```pseudocode
WHEN documentId changes from "abc123" to "xyz789":
    # Phase 1: Unmount old provider
    1. React calls all useEffect cleanup functions
       → WebSocket disconnects
       → Monaco editor event listeners removed
       → Kolabpad instance disposes
       → CSS styles cleaned up
    2. React destroys all state
       → users = {} discarded
       → myUserId = null discarded
       → connection = "disconnected" discarded
       → All state variables destroyed
    3. DocumentProvider component unmounted

    # Phase 2: Mount new provider
    4. React creates NEW DocumentProvider instance
    5. All state starts with initial values
       → users = {} (fresh empty object)
       → myUserId = null (fresh)
       → connection = "disconnected" (fresh)
    6. All useEffect hooks run with new documentId
       → New WebSocket connects to xyz789
       → New Monaco editor created
       → New Kolabpad instance created
    7. Server sends initial state for xyz789
       → History, Language, OTP, UserInfo messages
```

**Benefits**:
- **Impossible to forget**: React guarantees all state is destroyed
- **Automatic cleanup**: All useEffect cleanups run automatically
- **No stale closures**: New hooks capture new state values
- **No race conditions**: Old provider fully unmounted before new one mounts
- **Type-safe**: Adding new state automatically includes it in reset
- **Easy to test**: Just verify `key` prop changes

---

## Remounting Pattern Details

### How key Prop Works

React's `key` prop is typically used for list items, but can be used on any component to force remounting.

```pseudocode
# Without key - component updates in place
<DocumentProvider documentId={documentId}>
    # When documentId changes: props update, component re-renders
    # State is PRESERVED
    # useEffect runs with new documentId
    # Problem: Must manually reset all state

# With key - component remounts
<DocumentProvider key={documentId} documentId={documentId}>
    # When documentId changes: key changes, React sees as different component
    # Old component UNMOUNTS (cleanup runs)
    # New component MOUNTS (state starts fresh)
    # Problem: None! Automatic reset
```

### Real Implementation

**File**: `frontend/src/components/App.tsx`

```typescript
function App() {
  const documentId = useHash();  // Returns current hash (e.g., "abc123")

  return (
    <SessionProvider>
      <DocumentProvider key={documentId} documentId={documentId}>
        <DocumentEditor />
      </DocumentProvider>
    </SessionProvider>
  );
}
```

**Key observation**: Both `key` and `documentId` prop use the same value. This ensures:
- `key` changes → React remounts component
- `documentId` prop → New provider knows which document to connect to

### useEffect Cleanup

All cleanup happens automatically via useEffect cleanup functions.

**Example from DocumentProvider**:

```pseudocode
EFFECT when editor is ready:
    # Setup: Create Kolabpad instance
    kolabpad = new Kolabpad({
        uri: getWsUri(documentId),
        editor: editor,
        onConnected: () => setConnection("connected"),
        onDisconnected: () => setConnection("disconnected"),
        # ... other callbacks
    })

    # Cleanup: Dispose instance
    RETURN cleanup function:
        kolabpad.dispose()  # Closes WebSocket, removes listeners
        kolabpad = undefined
```

**What happens**:
1. When provider mounts: Effect runs, Kolabpad instance created
2. When provider unmounts: Cleanup runs, Kolabpad disposes
3. WebSocket closes gracefully
4. Monaco event listeners removed
5. No memory leaks

---

## Broadcast Synchronization Pattern

### The Problem with Flags and Refs

Many real-time apps use boolean flags or refs to track "did I initiate this change?". This is fragile and race-prone.

```pseudocode
# BAD: Flag-based synchronization (don't do this)

REF ignoreBroadcastFlag = false

FUNCTION handleUserChangeLanguage(newLanguage):
    # User clicks language selector
    ignoreBroadcastFlag = true
    sendLanguageChangeToServer(newLanguage)

    # Update local state optimistically
    setLanguage(newLanguage)

EFFECT when languageBroadcast received:
    # Server echoes our change back to us
    IF ignoreBroadcastFlag:
        ignoreBroadcastFlag = false
        RETURN  # Ignore - we already updated locally

    # Someone else changed it
    setLanguage(languageBroadcast.language)
    showToast("Language changed by {user}")
```

**Problems**:
1. **Race condition**: What if server broadcasts someone else's change before our echo?
   - Flag is true
   - We ignore their change
   - Wrong language shown
2. **Multiple changes**: What if user changes language twice quickly?
   - Flag handling becomes complex
3. **Fragile**: Easy to forget to reset flag
4. **Hard to test**: Must simulate exact timing of broadcasts

### The userId-Based Solution

Kolabpad uses a **deterministic, server-authoritative** approach: the server includes `userId` and `userName` in every broadcast.

```pseudocode
# GOOD: userId-based synchronization (do this)

EFFECT when languageBroadcast received:
    IF languageBroadcast.userId == myUserId:
        # I initiated this change
        showToast("Language updated to {language}")
    ELSE:
        # Someone else initiated this change
        showToast("{userName} changed language to {language}")

    # BOTH cases: apply the change
    setLanguage(languageBroadcast.language)
    updateMonacoLanguage(languageBroadcast.language)
```

**Why this works**:
1. **Deterministic**: No race conditions - we always know who initiated the change
2. **Server-authoritative**: Server decides the userId (can't be spoofed)
3. **Simple logic**: Just compare two numbers
4. **Easy to test**: Mock different userId values
5. **Works for concurrent changes**: Each change has its own userId

### Broadcast Flow Example

```pseudocode
# Scenario: Alice changes language to Python

CLIENT (Alice, userId=0):
    User clicks "Python" in language selector
    → sendLanguageChange("python")
    → WebSocket sends: { SetLanguage: "python" }

SERVER:
    Receives SetLanguage from connection with userId=0
    → Updates document.language = "python"
    → Broadcasts to ALL clients:
      {
          Language: {
              language: "python",
              user_id: 0,
              user_name: "Alice"
          }
      }

CLIENT (Alice, userId=0):
    Receives Language broadcast
    → languageBroadcast.userId (0) == myUserId (0)
    → Shows toast: "Language updated to python"
    → Updates Monaco language mode

CLIENT (Bob, userId=1):
    Receives Language broadcast
    → languageBroadcast.userId (0) != myUserId (1)
    → Shows toast: "Alice changed language to python"
    → Updates Monaco language mode

RESULT: Both clients in sync, with appropriate UX messages
```

### Why Server Echoes to Sender

**Question**: Why does the server send the broadcast back to the sender? Can't the client just update locally?

**Answer**: Server echo ensures consistency and provides confirmation.

**Benefits**:
1. **Confirmation**: Sender knows server accepted their change
2. **Consistency**: Same message handling for everyone (simpler code)
3. **Ordering**: Sender sees changes in same order as everyone else
4. **Error handling**: If server rejects, sender finds out immediately
5. **UX**: Different toasts for "you did this" vs "someone else did this"

**Example**: OTP protection

```pseudocode
# Without server echo (bad):
CLIENT clicks "Enable OTP"
    → setOtpEnabled(true)  # Optimistic update
    → POST /api/document/abc123/protect
    → IF error:
        setOtpEnabled(false)  # Revert
        # Problem: What if broadcast arrives during error handling?

# With server echo (good):
CLIENT clicks "Enable OTP"
    → setIsToggling(true)  # Show loading spinner
    → POST /api/document/abc123/protect
    → Wait for broadcast (not API response)
    → Broadcast arrives: setOtpEnabled(true)
    → setIsToggling(false)
    # Guaranteed consistency with server
```

---

## Initial State Synchronization

When a client connects to a document, the server sends initial state in a specific order.

### Connection Sequence

```pseudocode
CLIENT connects WebSocket to /api/socket/abc123?otp=xyz

SERVER receives connection:
    1. Send Identity message
       → Client learns its userId

    2. Send History message
       → Client receives all operations (full document text)

    3. Send Language message
       → Client learns current language setting

    4. Send OTP message (if document is protected)
       → Client learns OTP protection status

    5. Send UserInfo messages (one per connected user)
       → Client learns about other users

    6. Send UserCursor messages (one per user)
       → Client learns cursor positions

CLIENT is now fully synchronized and ready to edit
```

### Why This Order?

1. **Identity first**: Client needs userId before handling other messages
2. **History second**: Document content must be loaded before anything else
3. **Language third**: Syntax highlighting depends on content being loaded
4. **OTP fourth**: Protection status determines URL shown to user
5. **UserInfo/Cursors last**: Collaborative features after core state loaded

### Handling Initial vs. Update Broadcasts

Some hooks need to distinguish between initial state and updates.

**Example**: Language broadcast on connect

```pseudocode
# Server sends Language message on connect:
{
    Language: {
        language: "python",
        user_id: -1,           # Special SYSTEM_USER_ID
        user_name: "System"
    }
}

# Hook handling:
EFFECT when languageBroadcast changes:
    isInitialState = languageBroadcast.userId == SYSTEM_USER_ID

    setLanguage(languageBroadcast.language)  # Always update

    IF NOT isInitialState:
        # Real user change - show toast
        showToast(...)
    ELSE:
        # Initial state - don't show toast
        # (user just connected, no need to notify)
```

**Pattern**: Use special userId (`-1` = `SYSTEM_USER_ID`) to indicate server-initiated state.

---

## Monaco Editor Integration

### Applying OT Operations

The Kolabpad service applies operations to Monaco via `pushEditOperations`.

```pseudocode
FUNCTION applyOperation(operation):
    ignoreChanges = true  # Don't trigger onChange while applying

    ops = JSON.parse(operation.to_string())  # e.g., [10, "hello", -5]
    index = 0  # Current position in document

    FOR EACH op IN ops:
        IF op is string:
            # Insert text
            position = unicodePosition(model, index)
            model.pushEditOperations([], [{
                range: { start: position, end: position },
                text: op
            }])
            index += unicodeLength(op)

        ELSE IF op >= 0:
            # Retain (skip forward)
            index += op

        ELSE IF op < 0:
            # Delete
            chars = -op
            startPos = unicodePosition(model, index)
            endPos = unicodePosition(model, index + chars)
            model.pushEditOperations([], [{
                range: { start: startPos, end: endPos },
                text: ""
            }])

    ignoreChanges = false
```

**Key points**:
- `ignoreChanges` flag prevents recursive onChange calls
- Unicode position conversion required (JavaScript uses UTF-16)
- Operations applied in sequence (Insert, Delete, Retain)

### Remote Cursor Decorations

Monaco decorations show where other users are editing.

```pseudocode
FUNCTION updateCursors():
    decorations = []

    FOR EACH userId, cursorData IN userCursors:
        IF userId in users:
            user = users[userId]
            hue = user.hue

            # Add cursor decorations
            FOR EACH cursor IN cursorData.cursors:
                position = unicodePosition(model, cursor)
                decorations.add({
                    range: { start: position, end: position },
                    className: "remote-cursor-${hue}",
                    zIndex: EDITOR_CURSOR
                })

            # Add selection decorations
            FOR EACH [start, end] IN cursorData.selections:
                startPos = unicodePosition(model, start)
                endPos = unicodePosition(model, end)
                decorations.add({
                    range: { start: startPos, end: endPos },
                    className: "remote-selection-${hue}",
                    hoverMessage: user.name,
                    zIndex: EDITOR_SELECTION
                })

    # Update all decorations in one call (efficient)
    oldDecorations = model.deltaDecorations(oldDecorations, decorations)
```

**CSS styles** are generated dynamically based on user hue:

```pseudocode
FUNCTION generateCssStyles(hue):
    IF hue already generated:
        RETURN

    # Create <style> element if doesn't exist
    IF NOT styleElement:
        styleElement = createElement("style")
        document.head.appendChild(styleElement)

    # Add rules for this hue
    styleSheet.insertRule(`
        .monaco-editor .remote-cursor-${hue} {
            border-left: 2px solid hsl(${hue}, 90%, 25%);
        }
    `)
    styleSheet.insertRule(`
        .monaco-editor .remote-selection-${hue} {
            background-color: hsla(${hue}, 90%, 80%, 0.5);
        }
    `)

    generatedHues.add(hue)
```

**Memory management**: When Kolabpad instance disposes, it removes the `<style>` element to prevent memory leaks across document switches.

---

## Avoiding Stale Closures

### The Problem

WebSocket callbacks are created once but need to read current state values.

```pseudocode
# Problem: Stale closure

STATE myUserId = null

EFFECT on mount:
    websocket.onMessage = (msg) => {
        # This closure captures myUserId = null
        # Even after myUserId updates, this callback still sees null!
        IF msg.userId == myUserId:
            handleMyChange()
    }
```

**Why**: JavaScript closures capture variables by value at creation time. When `onMessage` is assigned, `myUserId` is `null`, and the callback will always see `null`.

### Solution 1: Ref to Latest Value

Use `useRef` to always read the latest value.

```pseudocode
STATE myUserId = null
REF myUserIdRef = useRef(null)

# Update ref whenever state changes
EFFECT when myUserId changes:
    myUserIdRef.current = myUserId

EFFECT on mount:
    websocket.onMessage = (msg) => {
        # Read from ref - always latest value
        IF msg.userId == myUserIdRef.current:
            handleMyChange()
    }
```

### Solution 2: Dependencies in useEffect

Include state values as dependencies, so the effect re-runs when they change.

```pseudocode
STATE myUserId = null

# Re-create callback when myUserId changes
EFFECT when myUserId changes:
    websocket.onMessage = (msg) => {
        # Fresh closure captures current myUserId
        IF msg.userId == myUserId:
            handleMyChange()
    }

    RETURN cleanup:
        websocket.onMessage = null
```

**Trade-off**: This recreates the callback on every change, which might cause issues if WebSocket is recreated frequently.

### Solution 3: Callback Pattern

Pass callbacks from parent instead of capturing state directly.

```pseudocode
# Parent component
FUNCTION DocumentProvider():
    STATE myUserId = null

    FUNCTION handleMessage(msg):
        # This function always has latest myUserId
        IF msg.userId == myUserId:
            handleMyChange()

    kolabpad = new Kolabpad({
        onMessage: handleMessage  # Pass callback
    })

# Kolabpad service
CLASS Kolabpad:
    CONSTRUCTOR(options):
        this.onMessage = options.onMessage

    handleWebSocketMessage(msg):
        this.onMessage(msg)  # Calls latest callback
```

**Kolabpad's approach**: Combination of all three:
- Uses refs for frequently-changing values
- Uses callbacks for event handlers
- Recreates Kolabpad instance when documentId changes (via remounting)

---

## Testing Strategy

### Testing Provider Remounting

**Test**: Verify DocumentProvider remounts when documentId changes.

```pseudocode
TEST "DocumentProvider remounts on document change":
    # Render with first document
    documentId = "abc123"
    RENDER <App /> with hash="#abc123"

    # Verify WebSocket connects to abc123
    EXPECT WebSocket.lastCall.url.includes("abc123")

    # Change document
    changeHash("#xyz789")

    # Verify old WebSocket closed
    EXPECT WebSocket.closed

    # Verify new WebSocket connects to xyz789
    EXPECT WebSocket.lastCall.url.includes("xyz789")
```

### Testing Broadcast Handlers

**Test**: Verify useLanguageSync handles broadcasts correctly.

```pseudocode
TEST "useLanguageSync shows confirmation for own change":
    myUserId = 0
    languageBroadcast = {
        language: "python",
        userId: 0,
        userName: "Alice"
    }

    RENDER <TestComponent> with useLanguageSync

    # Should show confirmation toast
    EXPECT toast.title == "Language updated"
    EXPECT toast.description.includes("All users")

TEST "useLanguageSync shows notification for other user":
    myUserId = 0
    languageBroadcast = {
        language: "python",
        userId: 1,
        userName: "Bob"
    }

    RENDER <TestComponent> with useLanguageSync

    # Should show notification toast
    EXPECT toast.description.includes("Bob")
```

### Testing Color Collision

**Test**: Verify useColorCollision adjusts hue on collision.

```pseudocode
TEST "useColorCollision changes hue when collision detected":
    currentHue = 180
    users = {
        1: { name: "Alice", hue: 180 },  # Collision!
        2: { name: "Bob", hue: 90 }
    }
    myUserId = 0
    connection = "connected"

    onHueChange = jest.fn()

    RENDER <TestComponent> with useColorCollision

    # Should detect collision and change hue
    EXPECT onHueChange.called
    EXPECT onHueChange.lastCall.arg != 180  # Different hue
```

### Testing Monaco Integration

**Test**: Verify operations apply correctly to Monaco.

```pseudocode
TEST "applyOperation inserts text":
    editor = createMockEditor()
    model.setValue("hello world")

    operation = [6, "cruel "]  # Insert at position 6

    applyOperation(operation)

    EXPECT model.getValue() == "hello cruel world"

TEST "applyOperation deletes text":
    model.setValue("hello cruel world")

    operation = [6, -6]  # Delete 6 chars at position 6

    applyOperation(operation)

    EXPECT model.getValue() == "hello world"
```

---

## Summary

Kolabpad's state synchronization architecture is built on three key patterns:

1. **Remounting for state reset**
   - Use `key={documentId}` to force provider remount
   - Automatic cleanup via useEffect cleanup functions
   - Impossible to forget to reset state

2. **userId-based broadcast filtering**
   - Server includes userId in every broadcast
   - Client compares to myUserId for UX (confirmation vs. notification)
   - Both sender and receivers apply the same state change
   - Deterministic, no race conditions

3. **Avoiding stale closures**
   - Use refs for frequently-changing values
   - Pass callbacks instead of capturing state
   - Remount components to get fresh closures

These patterns make it easy to:
- Add new broadcast types (follow the same userId pattern)
- Switch between documents (automatic cleanup)
- Test synchronization logic (deterministic, no timing dependencies)
- Debug state issues (clear data flow, no hidden flags)

For detailed information on specific topics:
- Frontend architecture overview: [frontend/01-frontend-architecture.md]
- WebSocket message protocol: [protocol/01-websocket-protocol.md]
- Backend broadcast implementation: [backend/02-broadcast-system.md]
- OT algorithm details: [architecture/02-operational-transformation.md]
