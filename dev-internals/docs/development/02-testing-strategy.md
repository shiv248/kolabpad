# Testing Strategy

**Purpose**: Explain testing philosophy, patterns, and procedures for maintaining quality in Kolabpad.

**Audience**: Contributors writing tests, maintainers reviewing PRs, QA engineers.

---

## Table of Contents

1. [Testing Philosophy](#testing-philosophy)
2. [Test Organization](#test-organization)
3. [Backend Testing](#backend-testing)
4. [Frontend Testing](#frontend-testing)
5. [Integration Testing](#integration-testing)
6. [Test Data Patterns](#test-data-patterns)
7. [Coverage Goals](#coverage-goals)
8. [Running Tests](#running-tests)
9. [Manual Testing](#manual-testing)
10. [Debugging Failed Tests](#debugging-failed-tests)

---

## Testing Philosophy

### What We Test

Kolabpad prioritizes testing **critical paths** that ensure correctness of collaborative editing:

1. **Operational Transformation** (highest priority)
   - Transform correctness (concurrent operations converge to same state)
   - Compose correctness (combining operations doesn't break convergence)
   - Apply correctness (operations modify text as expected)
   - Edge cases (empty documents, unicode, large operations)

2. **Persistence Logic** (high priority)
   - Document save/load correctness
   - OTP storage and validation
   - Database migrations
   - Concurrent access safety

3. **State Synchronization** (high priority)
   - WebSocket message handling
   - Broadcast to all clients
   - Document state reset on navigation
   - Hook-based UI updates

4. **UI Components** (lower priority)
   - Manual testing preferred for UX validation
   - Automated tests for complex hooks only

### Testing Trade-offs

**Why this approach?**

- OT bugs cause data loss or desynchronization (catastrophic)
- Persistence bugs lose user data (very bad)
- State bugs cause UI inconsistencies (bad)
- UI bugs are visible and easy to catch manually (less critical)

**Resources allocation**:

```
Time spent testing:
    50% - OT algorithm (comprehensive test suite)
    25% - Backend integration (WebSocket flows, persistence)
    15% - Frontend hooks and state management
    10% - Manual testing and exploratory QA
```

---

## Test Organization

### Backend Tests

Go tests live alongside source code with `_test.go` suffix:

```
pkg/
├── ot/
│   ├── operation.go
│   ├── operation_test.go      # Unit tests for operations
│   ├── transform.go
│   ├── transform_test.go      # Transform algorithm tests
│   ├── compose.go
│   └── compose_test.go        # Compose algorithm tests
├── database/
│   ├── database.go
│   └── database_test.go       # Database operations tests
└── server/
    ├── server.go
    └── server_test.go         # HTTP/WebSocket integration tests
```

### Frontend Tests

Tests live alongside source code with `.test.ts` or `.test.tsx` suffix:

```
frontend/src/
├── hooks/
│   ├── useLanguageSync.tsx
│   ├── useLanguageSync.test.tsx    # Hook behavior tests
│   ├── useOTPSync.tsx
│   ├── useOTPSync.test.tsx
│   ├── useColorCollision.ts
│   └── useColorCollision.test.ts
├── api/
│   ├── client.ts
│   └── client.test.ts              # API client tests
└── utils/
    ├── color.ts
    └── color.test.ts               # Utility function tests
```

---

## Backend Testing

> **Note**: OT algorithm tests are maintained in the external [operational-transformation-go](https://github.com/shiv248/operational-transformation-go) library.

### Persistence Tests

**Test file**: `pkg/database/database_test.go`

**Example test structure**:

```pseudocode
TEST save and load document:
    db = createTestDatabase()  // In-memory SQLite

    // Save document
    doc = Document{
        ID: "test123",
        Text: "hello world",
        Language: "javascript",
        OTP: "secret",
    }

    error = db.SaveDocument(doc)
    ASSERT error == nil

    // Load document
    loaded = db.LoadDocument("test123")
    ASSERT loaded.ID == "test123"
    ASSERT loaded.Text == "hello world"
    ASSERT loaded.Language == "javascript"
    ASSERT loaded.OTP == "secret"
```

**Key test scenarios**:

1. **CRUD operations**:
   - Create new document
   - Read existing document
   - Update document text
   - Delete expired document

2. **OTP operations**:
   - Save document with OTP
   - Load document with OTP
   - Update OTP (enable protection)
   - Remove OTP (disable protection)
   - Validate OTP on cold start

3. **Edge cases**:
   - Document doesn't exist (cold start)
   - Concurrent writes (SQLite handles this)
   - Large documents (near MAX_DOCUMENT_SIZE)
   - Special characters in text

4. **Migrations**:
   - Fresh database (runs all migrations)
   - Upgrade from old schema
   - Idempotent migrations (safe to run twice)

**Run database tests**:

```bash
go test ./pkg/database/... -v
```

### Server Integration Tests

**Test file**: `pkg/server/server_test.go`

**Example test structure**:

```pseudocode
TEST WebSocket connection and editing flow:
    // Start test server
    server = startTestServer()
    defer server.Close()

    // Connect client A
    clientA = connectWebSocket(server.URL + "/api/socket/test123")

    // Receive Identity message
    identity = clientA.readMessage()
    ASSERT identity.Identity == 0  // First user gets ID 0

    // Receive initial History message (empty document)
    history = clientA.readMessage()
    ASSERT history.History.operations == []

    // Send edit operation
    edit = EditMsg{
        Revision: 0,
        Operation: [["hello"]],  // Insert "hello" at start
    }
    clientA.sendMessage(edit)

    // Receive echo History message
    history = clientA.readMessage()
    ASSERT history.History.operations[0].operation == [["hello"]]

    // Connect client B
    clientB = connectWebSocket(server.URL + "/api/socket/test123")

    // Client B receives current state
    identityB = clientB.readMessage()
    ASSERT identityB.Identity == 1  // Second user gets ID 1

    historyB = clientB.readMessage()
    ASSERT historyB.History.operations[0].operation == [["hello"]]

    // Client A edit should broadcast to client B
    clientA.sendMessage(EditMsg{Revision: 1, Operation: [[5, " world"]]})

    msgA = clientA.readMessage()  // Echo
    msgB = clientB.readMessage()  // Broadcast

    ASSERT msgA == msgB  // Both receive same History message
```

**Key test scenarios**:

1. **Connection lifecycle**:
   - Single user connect
   - Multiple users connect
   - User disconnect
   - Reconnection after disconnect

2. **OTP validation**:
   - Connect without OTP (should accept)
   - Enable OTP via REST API
   - Connect with correct OTP (should accept)
   - Connect with wrong OTP (should reject)
   - Cold start OTP validation (check DB before loading doc)

3. **Broadcasting**:
   - Edit broadcasts to all users
   - Language change broadcasts to all users
   - OTP change broadcasts to all users
   - User join/leave broadcasts

4. **Graceful shutdown**:
   - All documents flushed to database
   - All WebSocket connections closed cleanly
   - Server exits with status 0

**Run server tests**:

```bash
go test ./pkg/server/... -v
```

---

## Frontend Testing

Frontend uses **Vitest** (fast, Vite-native test runner) with **React Testing Library**.

### Hook Tests

Custom hooks are the most important frontend tests because they contain business logic.

**Test file**: `frontend/src/hooks/useLanguageSync.test.tsx`

**Example test structure**:

```typescript
import { renderHook } from '@testing-library/react';
import { useLanguageSync } from './useLanguageSync';

describe('useLanguageSync', () => {
  it('shows confirmation toast when user initiates change', () => {
    const mockToast = vi.fn();
    const mockSetLanguage = vi.fn();

    const languageBroadcast = {
      language: 'python',
      userId: 42,
      userName: 'Alice',
    };

    renderHook(() =>
      useLanguageSync(languageBroadcast, 42, mockSetLanguage, mockToast)
    );

    // When userId matches myUserId, show confirmation toast
    expect(mockToast).toHaveBeenCalledWith({
      title: 'Language updated to python',
      status: 'info',
    });

    expect(mockSetLanguage).toHaveBeenCalledWith('python');
  });

  it('shows notification toast when other user changes language', () => {
    const mockToast = vi.fn();
    const mockSetLanguage = vi.fn();

    const languageBroadcast = {
      language: 'go',
      userId: 99,
      userName: 'Bob',
    };

    renderHook(() =>
      useLanguageSync(languageBroadcast, 42, mockSetLanguage, mockToast)
    );

    // When userId differs, show notification with username
    expect(mockToast).toHaveBeenCalledWith({
      title: 'Bob changed language to go',
      status: 'info',
    });

    expect(mockSetLanguage).toHaveBeenCalledWith('go');
  });
});
```

**Key test scenarios for hooks**:

1. **useLanguageSync**:
   - User initiates change (confirmation toast)
   - Other user initiates change (notification toast with name)
   - Language applied to editor in both cases

2. **useOTPSync**:
   - User enables OTP (confirmation + URL update)
   - Other user enables OTP (notification + URL update)
   - User disables OTP (confirmation + URL cleaned)
   - Other user disables OTP (notification + URL cleaned)

3. **useColorCollision**:
   - User color collides with existing user (changes to unused hue)
   - User color unique (no change)
   - Multiple collisions (finds truly unused hue)

**Run frontend tests**:

```bash
cd frontend

# Run all tests
npm test

# Watch mode (reruns on file change)
npm test -- --watch

# Run specific test file
npm test -- useLanguageSync

# With UI (browser-based test runner)
npm run test:ui
```

### API Client Tests

**Test file**: `frontend/src/api/client.test.ts`

Test HTTP fetch logic with mocked responses:

```typescript
describe('API client', () => {
  it('protects document via POST request', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ otp: 'abc123' }),
    });

    const result = await protectDocument('doc123', 1, 'Alice');

    expect(result.otp).toBe('abc123');
    expect(global.fetch).toHaveBeenCalledWith(
      '/api/document/doc123/protect',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ user_id: 1, user_name: 'Alice' }),
      })
    );
  });
});
```

### Utility Tests

**Test file**: `frontend/src/utils/color.test.ts`

Simple unit tests for pure functions:

```typescript
describe('color utilities', () => {
  it('calculates hue from user ID', () => {
    expect(calculateHue(0, 10)).toBe(0);
    expect(calculateHue(5, 10)).toBe(180);
    expect(calculateHue(9, 10)).toBe(324);
  });
});
```

---

## Integration Testing

Integration tests verify end-to-end flows across backend and frontend.

### WebSocket Flow Test

```pseudocode
TEST complete collaborative editing flow:
    // Setup
    server = startTestServer()
    db = server.database

    // User A connects
    wsA = connectWebSocket(server, "doc123")
    ASSERT receiveMessage(wsA).Identity == 0

    // User A sends edit
    sendMessage(wsA, EditMsg{revision: 0, operation: [["hello"]]})

    // User A receives echo
    msgA = receiveMessage(wsA)
    ASSERT msgA.History.operations[0].operation == [["hello"]]

    // User B connects
    wsB = connectWebSocket(server, "doc123")
    ASSERT receiveMessage(wsB).Identity == 1

    // User B receives current state
    historyB = receiveMessage(wsB)
    ASSERT historyB.History.operations[0].operation == [["hello"]]

    // User B sends edit
    sendMessage(wsB, EditMsg{revision: 1, operation: [[5, " world"]]})

    // Both users receive broadcast
    ASSERT receiveMessage(wsA).History.operations[0].operation == [[5, " world"]]
    ASSERT receiveMessage(wsB).History.operations[0].operation == [[5, " world"]]

    // Verify final document state
    doc = db.LoadDocument("doc123")
    ASSERT doc.Text == "hello world"
```

### OTP Protection Flow Test

```pseudocode
TEST OTP protection lifecycle:
    server = startTestServer()

    // Enable OTP via REST API
    response = POST(server.URL + "/api/document/test/protect", {
        user_id: 1,
        user_name: "Alice",
    })

    otp = response.otp
    ASSERT otp != ""

    // Connect without OTP - should reject
    ASSERT connectWebSocket(server, "test") == ERROR

    // Connect with wrong OTP - should reject
    ASSERT connectWebSocket(server, "test", otp="wrong") == ERROR

    // Connect with correct OTP - should accept
    ws = connectWebSocket(server, "test", otp=otp)
    ASSERT receiveMessage(ws).Identity == 0

    // Disable OTP via REST API (requires current OTP)
    response = DELETE(server.URL + "/api/document/test/protect", {
        user_id: 1,
        user_name: "Alice",
        otp: otp,
    })
    ASSERT response.status == 204

    // Connect without OTP - should now accept
    ws2 = connectWebSocket(server, "test")
    ASSERT receiveMessage(ws2).Identity == 1
```

---

## Test Data Patterns

### Mocking WebSocket

**Backend**: Create test WebSocket connections using `nhooyr.io/websocket` test helpers.

**Frontend**: Mock WebSocket class for unit tests:

```typescript
class MockWebSocket {
  onopen: (() => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: (() => void) | null = null;

  send(data: string) {
    // Capture sent messages for assertions
    this.sentMessages.push(JSON.parse(data));
  }

  simulateMessage(data: any) {
    if (this.onmessage) {
      this.onmessage({ data: JSON.stringify(data) } as MessageEvent);
    }
  }

  sentMessages: any[] = [];
}
```

### Test Databases

Use in-memory SQLite for speed and isolation:

```go
func createTestDB(t *testing.T) *database.Database {
    db, err := database.New(":memory:")  // In-memory database
    if err != nil {
        t.Fatalf("Failed to create test database: %v", err)
    }

    // Each test gets fresh database
    t.Cleanup(func() { db.Close() })

    return db
}
```

### Test Documents

Use predictable test data:

```go
var testDocuments = []struct {
    id       string
    text     string
    language string
    otp      string
}{
    {"test-empty", "", "plaintext", ""},
    {"test-hello", "hello world", "javascript", ""},
    {"test-protected", "secret data", "python", "abc123"},
    {"test-unicode", "Hello 👋 世界", "plaintext", ""},
}
```

---

## Coverage Goals

### Target Coverage

- **OT algorithm**: 100% (critical path, no exceptions)
- **Persistence logic**: 90%+ (cover all CRUD operations)
- **Broadcast system**: 80%+ (cover all message types)
- **Frontend hooks**: 80%+ (cover main behavior paths)
- **UI components**: Manual testing (unit tests optional)

### Measuring Coverage

**Backend**:

```bash
# Generate coverage report
make test.coverage

# Opens coverage.html in browser
# Green = covered, red = not covered
```

**Frontend**:

```bash
cd frontend

# Generate coverage report
npm run test:coverage

# Output: coverage/ directory with HTML report
```

### Coverage Exemptions

Some code doesn't need 100% coverage:

- **Error handling**: Difficult to trigger (e.g., malloc failure)
- **Logging**: Side effects only, no logic
- **Configuration parsing**: Simple getEnv wrappers
- **Debug code**: Development-only paths

---

## Running Tests

### All Tests

```bash
# Backend + Frontend
make test.all

# Equivalent to:
make test             # Backend
make test.frontend    # Frontend
```

### Watch Mode

**Frontend** (reruns on file change):

```bash
cd frontend
npm test -- --watch

# Or with UI
npm run test:ui
```

**Backend** (requires manual setup):

```bash
# Install: go install github.com/cespare/reflex@latest

# Run with auto-rerun
reflex -r '\.go$' -s go test ./...
```

### Continuous Integration

Tests run automatically on GitHub Actions (or similar CI):

```yaml
# Example .github/workflows/test.yml
name: Tests
on: [push, pull_request]

jobs:
  backend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.23'
      - run: go test -v ./...

  frontend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-node@v3
        with:
          node-version: '18'
      - run: cd frontend && npm ci && npm test
```

---

## Manual Testing

Automated tests don't catch everything. Manual testing is essential for:

- **User experience**: Does it feel right?
- **Edge cases**: Unusual user behavior
- **Visual bugs**: Layout issues, color problems
- **Browser compatibility**: Works in Chrome, Firefox, Safari?

### Basic Smoke Test

Run before every release:

```
✓ Create new document (auto-redirect to random ID)
✓ Type text and see it appear
✓ Open same document in second tab
✓ See cursor and selections from other tab
✓ Type in both tabs simultaneously
✓ Verify text converges to same state
✓ Change language and see syntax highlighting
✓ Enable OTP protection
✓ Copy URL with OTP
✓ Open in incognito window (should connect)
✓ Open without OTP (should reject)
✓ Disable OTP protection
✓ Disconnect and reconnect (state preserved)
✓ Switch to different document (state resets)
```

### Edge Case Testing

Test unusual scenarios:

```
✓ Very large document (near 256KB limit)
✓ Rapid typing (buffering and composition)
✓ Network interruption (disconnect and reconnect)
✓ Many users (10+ tabs on same document)
✓ Long idle (30+ minutes, websocket timeout)
✓ Unicode characters (emoji, RTL text, CJK)
✓ Special characters (null bytes, control chars)
✓ Concurrent language changes
✓ Concurrent OTP enable/disable
✓ Browser back/forward navigation
✓ Page refresh
```

### Browser Compatibility

Test on multiple browsers:

- **Chrome/Edge** (Chromium engine)
- **Firefox** (Gecko engine)
- **Safari** (WebKit engine)

Known issues:
- Safari sometimes slower with large documents
- Firefox stricter CORS enforcement (good for testing)

### Performance Testing

Monitor performance with many users:

```bash
# Terminal 1: Start server with debug logging
LOG_LEVEL=debug make dev-backend

# Terminal 2: Connect many clients
for i in {1..20}; do
  open "http://localhost:5173/#test123"
done

# Observe:
# - Memory usage (should stay under 100MB for 20 users)
# - CPU usage (should stay under 50% on modern CPU)
# - Message latency (under 50ms typical)
# - No errors in server logs
```

---

## Debugging Failed Tests

### Backend Test Failures

**View detailed output**:

```bash
go test -v ./pkg/server/... -run TestConcurrentEdits
```

**Run single test**:

```bash
go test -v ./pkg/server/... -run TestEditBroadcast
```

**Add debug logging**:

```go
func TestEditBroadcast(t *testing.T) {
    // ...
    t.Logf("Operation: %+v", operation)
    t.Logf("User ID: %d", userID)
    t.Logf("Result: %+v", result)
    // ...
}
```

**Use debugger** (Delve):

```bash
# Install: go install github.com/go-delve/delve/cmd/dlv@latest

# Run test with debugger
dlv test ./pkg/server/... -- -test.run TestConcurrentEdits

# Set breakpoint
(dlv) break kolabpad.go:100
(dlv) continue
```

### Frontend Test Failures

**View detailed output**:

```bash
cd frontend
npm test -- --reporter=verbose useLanguageSync
```

**Debug in browser**:

```bash
# Start UI mode (opens browser with DevTools)
npm run test:ui

# Click on failed test
# Use browser debugger (breakpoints, console.log)
```

**Inspect hook state**:

```typescript
it('debugs hook behavior', () => {
  const { result, rerender } = renderHook(() =>
    useLanguageSync(broadcast, myUserId)
  );

  console.log('Hook result:', result.current);

  // Trigger re-render with new props
  rerender();

  console.log('After rerender:', result.current);
});
```

---

## Best Practices

### Test Naming

**Good test names** describe behavior:

```go
// Good
func TestTransformConcurrentInsertsAtSamePosition(t *testing.T)
func TestDatabaseLoadReturnsNotFoundForMissingDocument(t *testing.T)

// Avoid vague names
func TestTransform1(t *testing.T)
func TestDatabase(t *testing.T)
```

### Test Independence

Each test should run independently:

```go
// Good: Creates fresh database per test
func TestSaveDocument(t *testing.T) {
    db := createTestDB(t)  // Fresh DB
    // ... test logic
}

// Bad: Shares state between tests
var globalDB *database.Database  // Avoid!

func TestSaveDocument(t *testing.T) {
    globalDB.Save(...)  // Breaks if other test modified DB
}
```

### Test Readability

Use the **Arrange-Act-Assert** pattern:

```go
func TestTransform(t *testing.T) {
    // Arrange: Set up test data
    doc := "hello"
    opA := Insert("X", 5)
    opB := Insert("Y", 5)

    // Act: Perform operation
    opA', opB' := transform(opA, opB)

    // Assert: Verify results
    assert.Equal(t, expectedA, opA')
    assert.Equal(t, expectedB, opB')
}
```

---

## Next Steps

- **Development**: See [01-development-workflow.md](./01-development-workflow.md) for setup and workflow
- **Deployment**: See [03-deployment-monitoring.md](./03-deployment-monitoring.md) for production testing
- **Architecture**: See [../architecture/02-operational-transformation.md](../architecture/02-operational-transformation.md) for OT algorithm details

---

**Testing is documentation**: Good tests serve as examples of how code should be used. Write tests that future developers can learn from.
