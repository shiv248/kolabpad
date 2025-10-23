# Frontend Architecture

## Purpose

This document explains the frontend structure, state management layers, and hooks architecture. It serves as a guide for understanding how the TypeScript + React frontend is organized and why certain architectural decisions were made.

**Target Audience**: Frontend developers working on Kolabpad, contributors understanding the React architecture.

---

## Project Structure

The frontend is built with React, TypeScript, and Vite, with Monaco Editor for the code editing experience.

```
frontend/src/
├── contexts/           # React context providers
│   ├── SessionProvider.tsx      # Session-scoped state (name, hue, darkMode)
│   └── DocumentProvider.tsx     # Document-scoped state (connection, users, editor)
├── hooks/              # Custom React hooks
│   ├── useHash.ts               # URL hash parsing for document ID
│   ├── useLanguageSync.tsx      # Language broadcast synchronization
│   ├── useOTPSync.tsx           # OTP broadcast synchronization
│   └── useColorCollision.ts     # Color collision detection
├── api/                # REST API client
│   ├── client.ts                # Base API fetch wrapper
│   └── documents.ts             # Document-related endpoints (protect/unprotect)
├── services/           # WebSocket and business logic
│   └── kolabpad.ts              # WebSocket client, OT integration, Monaco integration
├── components/         # React UI components
│   ├── App.tsx                  # Root component with provider hierarchy
│   ├── document/                # Document editor components
│   ├── sidebar/                 # Sidebar components (language, OTP, users)
│   └── shared/                  # Shared UI components
├── types/              # TypeScript type definitions
│   ├── index.ts                 # Re-exports all types
│   ├── kolabpad.ts              # WebSocket message types
│   ├── broadcast.ts             # Broadcast types
│   └── api.ts                   # API request/response types
├── utils/              # Utility functions
│   ├── color.ts                 # Hue generation and collision detection
│   └── url.ts                   # URL parameter parsing
└── theme/              # UI theme constants
    ├── colors.ts
    ├── layout.ts
    └── zIndex.ts
```

---

## State Management Layers

Kolabpad uses a **layered state management approach** with React Context and custom hooks. No external state library (Redux, Zustand, MobX) is used.

### Layer 1: Session State

**Managed by**: `SessionProvider`

**Persists across**: Document changes, page refreshes

**Storage**: `localStorage`

```pseudocode
SessionProvider manages:
    name: string                # User display name (e.g., "Anonymous Panda")
    hue: number (0-359)         # User color for cursor/selection
    darkMode: boolean           # Theme preference

Storage:
    localStorage.name = "Anonymous Panda"
    localStorage.hue = 180
    localStorage.darkMode = false

Lifecycle:
    - Initialized on app mount
    - Never remounts during session
    - Survives document changes
    - Survives page refresh
```

**Why localStorage?**
- User preferences should persist across sessions
- No login system, so localStorage is simplest solution
- Values are non-sensitive (just display preferences)

### Layer 2: Document State

**Managed by**: `DocumentProvider`

**Resets when**: Document ID changes (provider remounts via `key` prop)

**Storage**: React state (in-memory only)

```pseudocode
DocumentProvider manages:
    connection: "connected" | "disconnected" | "desynchronized"
    users: Map<userId, UserInfo>        # Other users in document
    myUserId: number | null             # Assigned by server
    language: string                    # Current syntax highlighting mode
    languageBroadcast: LanguageBroadcast | undefined
    otpBroadcast: OTPBroadcast | undefined
    editor: MonacoEditor instance       # Monaco editor reference
    isAuthBlocked: boolean              # OTP validation failed

Lifecycle:
    - Created when document ID changes
    - Automatically remounts (React key prop pattern)
    - All state resets on remount
    - WebSocket disconnects/reconnects on remount
```

**Why remount on document change?**
- Automatic state reset (no manual cleanup needed)
- Impossible to forget to reset a variable
- All useEffect cleanups run automatically
- WebSocket automatically disconnects/reconnects
- No stale closures or refs

### Layer 3: Editor State

**Managed by**: Monaco Editor instance

**Storage**: Monaco's internal state

```pseudocode
Monaco editor manages:
    - Text content (the document being edited)
    - Decorations (remote user cursors and selections)
    - Language mode (syntax highlighting rules)
    - Undo/redo history (local to Monaco)
    - View state (scroll position, cursor position)
```

**Integration**:
- Kolabpad service receives reference to editor
- Applies OT operations by calling Monaco's `pushEditOperations`
- Listens to Monaco's `onDidChangeModelContent` for local edits
- Updates decorations via `deltaDecorations` for remote cursors

---

## Provider Hierarchy

The provider hierarchy is carefully structured to separate session concerns from document concerns.

```pseudocode
App:
    <SessionProvider>                     # Session state (never remounts)
        name, hue, darkMode from localStorage

        <DocumentProvider key={documentId}> # Document state (remounts on docId change)
            connection, users, myUserId
            WebSocket to server
            Monaco editor instance

            <DocumentEditor>              # UI components
                <Sidebar>
                    <LanguageSelector />
                    <OTPManager />
                    <UserList />
                </Sidebar>
                <Editor />                # Monaco editor
            </DocumentEditor>
        </DocumentProvider>
    </SessionProvider>
```

**Key insight**: The `key={documentId}` prop on `DocumentProvider` forces React to completely unmount and remount the provider when the document ID changes. This is the core mechanism for automatic state reset.

---

## Why This Architecture?

### 1. Automatic State Reset

**Problem**: When switching documents (URL hash changes from `#abc123` to `#xyz789`), we need to reset ALL document-specific state:
- Clear user list
- Disconnect from old WebSocket
- Clear Monaco editor content
- Reset language to new document's language
- Clear OTP state

**Old approach** (manual resets):
```pseudocode
FUNCTION switchDocument(newDocId):
    setUsers({})                    # Easy to forget one!
    setConnection("disconnected")
    setMyUserId(null)
    setLanguage("plaintext")
    setOtpBroadcast(undefined)
    websocket.close()
    editor.clear()
    # ... many more manual resets
```

**New approach** (remounting):
```pseudocode
<DocumentProvider key={documentId} documentId={documentId}>
    # When documentId changes:
    # 1. React unmounts old DocumentProvider
    # 2. All hooks cleanup (useEffect cleanups run)
    # 3. All state destroyed
    # 4. React mounts NEW DocumentProvider
    # 5. All state starts fresh
```

**Benefits**:
- No manual state reset needed
- Impossible to forget to reset a variable
- All useEffect cleanup functions run automatically
- WebSocket automatically disconnects and reconnects
- Much harder to introduce bugs

### 2. Clear Boundaries

**Session state**: User identity (name, color, theme)
- Persists across documents
- Stored in localStorage
- Managed by SessionProvider

**Document state**: Collaboration state
- Resets on document change
- In-memory only
- Managed by DocumentProvider

This clear separation makes it obvious where each piece of state belongs.

### 3. No Stale Closures

**Problem**: WebSocket callbacks capture state values when created, leading to stale values.

**Solution**: Hooks encapsulate state and callbacks together, ensuring fresh values:
```pseudocode
# Bad (stale closure):
websocket.onMessage = (msg) => {
    IF msg.userId == myUserId:    # myUserId might be stale!
        handleMyChange()
}

# Good (hook ensures fresh values):
useLanguageSync({
    languageBroadcast,            # Always fresh from DocumentProvider
    myUserId,                     # Always fresh from DocumentProvider
    onLanguageChange: setLanguage # Stable setter
})
```

### 4. Testable

Each provider and hook can be tested independently:
- `SessionProvider`: Test localStorage persistence
- `DocumentProvider`: Test WebSocket initialization and cleanup
- `useLanguageSync`: Test broadcast handling with mock props
- `useColorCollision`: Test collision detection algorithm

No need to mock entire app hierarchy to test a single hook.

---

## Custom Hooks Architecture

Custom hooks encapsulate complex state synchronization logic and keep components clean.

### useLanguageSync

**Purpose**: Handle language broadcast synchronization and toast notifications.

**Location**: `frontend/src/hooks/useLanguageSync.tsx`

```pseudocode
HOOK useLanguageSync(languageBroadcast, myUserId, onLanguageChange):
    EFFECT when languageBroadcast changes:
        IF languageBroadcast is undefined:
            RETURN early  # No broadcast yet

        isMyChange = languageBroadcast.userId == myUserId
        isInitialState = languageBroadcast.userId == SYSTEM_USER_ID

        # Update language state via callback
        onLanguageChange(languageBroadcast.language)

        # Show appropriate toast (skip for initial state)
        IF NOT isInitialState:
            IF isMyChange:
                showToast("Language updated to {language}")
            ELSE:
                showToast("{userName} changed language to {language}")
```

**Key features**:
- Dependency on `languageBroadcast` only (not `myUserId`)
- Reads latest `myUserId` inside effect but doesn't depend on it
- Differentiates between "I changed it" and "someone else changed it"
- Skips toast for initial state (server sends language on connect)

### useOTPSync

**Purpose**: Manage OTP protection state with server synchronization.

**Location**: `frontend/src/hooks/useOTPSync.tsx`

```pseudocode
HOOK useOTPSync(documentId, currentUserId, currentUserName, otpBroadcast):
    STATE otp: string | null = null
    STATE otpEnabled: boolean = false
    STATE isToggling: boolean = false

    # Initialize from URL on mount
    EFFECT on mount:
        otpFromUrl = getOtpFromUrl()
        IF otpFromUrl:
            setOtp(otpFromUrl)
            setOtpEnabled(true)

    # Sync from server broadcasts
    EFFECT when otpBroadcast changes:
        IF otpBroadcast is undefined:
            RETURN early

        isMyChange = otpBroadcast.userId == currentUserId

        IF otpBroadcast.otp:
            # OTP enabled
            setOtp(otpBroadcast.otp)
            setOtpEnabled(true)
            updateURL("#${documentId}?otp=${otp}")

            showToast(
                isMyChange ? "OTP Protection Enabled"
                           : "Protection enabled by {userName}"
            )
        ELSE:
            # OTP disabled
            setOtp(null)
            setOtpEnabled(false)
            updateURL("#${documentId}")

            showToast(
                isMyChange ? "OTP Protection Disabled"
                           : "Protection disabled by {userName}"
            )

    # API call to toggle OTP
    ASYNC FUNCTION toggleOTP(enabled):
        setIsToggling(true)
        TRY:
            IF enabled:
                AWAIT protectDocument(documentId, userId, userName)
            ELSE:
                AWAIT unprotectDocument(documentId, userId, userName, currentOtp)
            # State will update from broadcast (don't update here)
        CATCH error:
            showToast("Error: {error}")
            setOtpEnabled(NOT enabled)  # Revert optimistic update
        FINALLY:
            setIsToggling(false)

    RETURN { otp, otpEnabled, isToggling, documentUrl, toggleOTP }
```

**Key features**:
- Initializes from URL parameter on mount
- Updates from server broadcasts (not from API responses)
- Generates shareable URL with OTP included
- Handles API errors gracefully
- Reverts state on error

### useColorCollision

**Purpose**: Detect and resolve color collisions when joining a document.

**Location**: `frontend/src/hooks/useColorCollision.ts`

```pseudocode
HOOK useColorCollision(connection, myUserId, users, currentHue, onHueChange):
    REF collisionCheckDone = false

    EFFECT when connection/myUserId/users/currentHue changes:
        # Only check once when initially connected
        IF connection != "connected" OR myUserId is null OR collisionCheckDone:
            RETURN early

        # Extract hues from other users (excluding self)
        existingHues = users
            .filter(id != myUserId)
            .map(user => user.hue)

        # Check for collision
        IF hasHueCollision(currentHue, existingHues):
            newHue = generateHue(existingHues)
            onHueChange(newHue)
            showToast("Color changed to avoid collision")

        # Mark as done for this document session
        collisionCheckDone = true
```

**Key features**:
- Only runs once per document session (using `useRef` flag)
- Excludes own user ID when checking collisions
- Generates new hue that doesn't collide with existing users
- Automatically updates session state (persists to localStorage)

---

## WebSocket Service Layer

The `Kolabpad` service handles all WebSocket communication and Monaco editor integration.

**Location**: `frontend/src/services/kolabpad.ts`

```pseudocode
CLASS Kolabpad:
    CONSTRUCTOR(options):
        this.uri = options.uri                  # WebSocket URL
        this.editor = options.editor            # Monaco editor instance
        this.callbacks = options.callbacks      # Event handlers

        this.me = -1                            # User ID (assigned by server)
        this.revision = 0                       # Current document revision
        this.outstanding = null                 # Pending operation
        this.buffer = null                      # Buffered operations
        this.users = {}                         # Connected users
        this.userCursors = {}                   # Remote cursor positions

        # Start connection attempts
        this.connect()
        scheduleReconnect(every 2 seconds)

    FUNCTION connect():
        websocket = new WebSocket(this.uri)

        websocket.onOpen:
            this.callbacks.onConnected()
            sendUserInfo()
            sendCursorData()
            IF this.outstanding:
                resendOperation(this.outstanding)  # Recover pending op

        websocket.onMessage(message):
            this.handleMessage(message)

        websocket.onClose:
            this.callbacks.onDisconnected()
            IF recentFailures >= MAX_FAILURES:
                this.dispose()
                this.callbacks.onDesynchronized()

    FUNCTION handleMessage(msg):
        IF msg.Identity:
            this.me = msg.Identity
            this.callbacks.onIdentity(this.me)

        ELSE IF msg.History:
            # Apply operations from server
            FOR EACH operation IN msg.History.operations:
                this.revision++
                IF operation.userId == this.me:
                    # Our operation acknowledged
                    this.serverAck()
                ELSE:
                    # Remote operation - transform and apply
                    this.applyServer(operation)

        ELSE IF msg.Language:
            this.callbacks.onChangeLanguage(
                msg.Language.language,
                msg.Language.user_id,
                msg.Language.user_name
            )

        ELSE IF msg.OTP:
            this.callbacks.onChangeOTP(
                msg.OTP.otp,
                msg.OTP.user_id,
                msg.OTP.user_name
            )

        ELSE IF msg.UserInfo:
            IF msg.UserInfo.info:
                # User joined
                this.users[msg.UserInfo.id] = msg.UserInfo.info
            ELSE:
                # User left
                DELETE this.users[msg.UserInfo.id]
                DELETE this.userCursors[msg.UserInfo.id]

            this.updateCursors()
            this.callbacks.onChangeUsers(this.users)

        ELSE IF msg.UserCursor:
            this.userCursors[msg.UserCursor.id] = msg.UserCursor.data
            this.updateCursors()

    FUNCTION applyServer(operation):
        # Transform against outstanding and buffered operations
        IF this.outstanding:
            pair = this.outstanding.transform(operation)
            this.outstanding = pair.first()
            operation = pair.second()

            IF this.buffer:
                bufferPair = this.buffer.transform(operation)
                this.buffer = bufferPair.first()
                operation = bufferPair.second()

        # Apply to Monaco editor
        this.applyOperation(operation)
        this.transformCursors(operation)

    FUNCTION applyClient(operation):
        IF NOT this.outstanding:
            # No pending operation - send immediately
            this.sendOperation(operation)
            this.outstanding = operation
        ELSE IF NOT this.buffer:
            # Outstanding exists - buffer this one
            this.buffer = operation
        ELSE:
            # Both outstanding and buffer exist - compose
            this.buffer = this.buffer.compose(operation)

        this.transformCursors(operation)

    FUNCTION updateCursors():
        decorations = []

        FOR EACH userId, cursorData IN this.userCursors:
            IF userId in this.users:
                user = this.users[userId]

                # Add cursor decorations
                FOR EACH cursor IN cursorData.cursors:
                    position = unicodePosition(cursor)
                    decorations.add({
                        range: position,
                        className: "remote-cursor-${user.hue}",
                        zIndex: EDITOR_CURSOR
                    })

                # Add selection decorations
                FOR EACH selection IN cursorData.selections:
                    startPos = unicodePosition(selection.start)
                    endPos = unicodePosition(selection.end)
                    decorations.add({
                        range: [startPos, endPos],
                        className: "remote-selection-${user.hue}",
                        hoverMessage: user.name,
                        zIndex: EDITOR_SELECTION
                    })

        # Update Monaco decorations
        editor.deltaDecorations(oldDecorations, decorations)

    FUNCTION dispose():
        this.disposed = true
        websocket.close()
        clearInterval(reconnectTimer)
        # Clean up CSS styles to prevent memory leak
        IF this.styleElement:
            this.styleElement.remove()
```

**Key features**:
- Handles WebSocket lifecycle (connect, reconnect, disconnect)
- Implements OT client logic (outstanding, buffer, transform)
- Integrates with Monaco editor (apply operations, update decorations)
- Manages remote user cursors and selections
- Automatically reconnects on disconnect
- Detects desynchronization and notifies app
- Cleans up resources on dispose (prevents memory leaks)

**Unicode handling**:
- JavaScript uses UTF-16 encoding (2 bytes for emoji)
- OT operations use Unicode codepoint offsets (1 per emoji)
- Conversion functions (`unicodeLength`, `unicodeOffset`, `unicodePosition`) handle translation

---

## API Client Layer

The API client handles REST endpoints for administrative operations.

**Location**: `frontend/src/api/`

### Base Client

**File**: `client.ts`

```pseudocode
FUNCTION apiFetch(url, options):
    response = FETCH(url, {
        method: options.method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(options.body)
    })

    IF NOT response.ok:
        error = AWAIT response.json()
        THROW ApiError(error.message)

    IF response.status == 204:
        RETURN undefined  # No content

    RETURN AWAIT response.json()
```

### Document Endpoints

**File**: `documents.ts`

```pseudocode
ASYNC FUNCTION protectDocument(documentId, userId, userName):
    RETURN apiFetch(`/api/document/${documentId}/protect`, {
        method: "POST",
        body: { user_id: userId, user_name: userName }
    })
    # Returns: { otp: "abc123" }

ASYNC FUNCTION unprotectDocument(documentId, userId, userName, currentOtp):
    RETURN apiFetch(`/api/document/${documentId}/protect`, {
        method: "DELETE",
        body: { user_id: userId, user_name: userName, otp: currentOtp }
    })
    # Returns: void (204 No Content)
```

**Why REST for OTP?**
- OTP is administrative/security feature, not collaborative
- REST enables future authentication middleware
- REST enables rate limiting per endpoint
- REST enables audit logging more easily
- WebSocket still used for broadcasting (real-time updates to all users)

See [protocol/02-rest-api.md] for full REST API specification.

---

## Why Not Redux/Zustand/MobX?

**Decision**: Use React Context + custom hooks instead of external state library.

**Rationale**:

1. **Sufficient complexity**: Kolabpad's state management needs are well-served by Context
   - Session state: Simple key-value pairs (name, hue, darkMode)
   - Document state: Mostly driven by WebSocket messages
   - No complex state derivations or cross-cutting concerns

2. **Fewer dependencies**:
   - Smaller bundle size
   - Fewer security vulnerabilities to monitor
   - Less upgrade churn

3. **Easier onboarding**:
   - Standard React patterns (Context, hooks)
   - No library-specific concepts to learn
   - Easier for contributors to understand

4. **Custom hooks provide exactly what we need**:
   - `useLanguageSync`: Broadcast handling with toast notifications
   - `useOTPSync`: OTP state + API calls + URL synchronization
   - `useColorCollision`: Collision detection on join
   - These are domain-specific and wouldn't benefit from generic state library

5. **No performance issues**:
   - Re-renders are minimal (state changes infrequent)
   - Context values are stable (providers don't remount unnecessarily)
   - Monaco editor handles text content (not React state)

**When would we reconsider?**
- If app grows to 10+ connected state concerns
- If we need time-travel debugging
- If we need complex state derivations with memoization
- If we need better DevTools integration

For now, Context + hooks is the right choice for Kolabpad's scope.

---

## Summary

The frontend architecture prioritizes:
- **Automatic state reset** via provider remounting
- **Clear state boundaries** between session and document concerns
- **Custom hooks** for complex synchronization logic
- **No external state library** (React Context is sufficient)
- **Clean WebSocket integration** with OT and Monaco

This architecture makes it easy to:
- Add new broadcast types (create new hook)
- Test individual pieces (providers and hooks are isolated)
- Onboard new contributors (standard React patterns)
- Debug state issues (clear data flow)

For detailed information on specific topics:
- State synchronization patterns: [frontend/02-state-synchronization.md]
- WebSocket protocol: [protocol/01-websocket-protocol.md]
- REST API: [protocol/02-rest-api.md]
- Backend broadcast system: [backend/02-broadcast-system.md]
