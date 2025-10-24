# Operational Transformation in Kolabpad

> **Algorithm Details**: For deep-dives into OT algorithms, see [operational-transformation-go documentation](https://github.com/shiv248/operational-transformation-go/tree/main/dev-internals/docs)

**Purpose**: This document explains how Kolabpad integrates Operational Transformation for real-time collaborative editing.

**Audience**: Developers implementing collaborative features, contributors working on WebSocket protocol, architects evaluating the system design.

---

## Table of Contents

1. [Why OT?](#why-ot)
2. [Integration Architecture](#integration-architecture)
3. [WASM Bridge](#wasm-bridge)
4. [Message Flow](#message-flow)
5. [Server-Side OT](#server-side-ot)
6. [Client-Side OT](#client-side-ot)
7. [Testing Integration](#testing-integration)

---

## Why OT?

Kolabpad uses **Operational Transformation** from the [operational-transformation-go](https://github.com/shiv248/operational-transformation-go) library for conflict resolution.

### Key Benefits for Kolabpad

**Server authority**: Single source of truth
- Server assigns sequential revision numbers
- Deterministic conflict resolution
- Simplified debugging with centralized decision-making

**Compact wire format**: Efficient real-time sync
```json
// Typical operation: ~15-50 bytes
[10, "hello", -3, 5]
```

**Proven technology**: Battle-tested in production systems
- Google Docs (early versions)
- Etherpad
- ShareDB

**Simple client model**: No complex merge logic on frontend
- Clients send operations at their revision
- Server transforms and broadcasts
- Clients apply transformed operations

---

## Integration Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     Kolabpad System                     │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌──────────────┐         ┌──────────────┐            │
│  │   Browser    │         │  Go Backend  │            │
│  │              │         │              │            │
│  │  TypeScript  │◄───────►│  WebSocket   │            │
│  │    Client    │  JSON   │    Server    │            │
│  │              │         │              │            │
│  │      │       │         │      │       │            │
│  │      ▼       │         │      ▼       │            │
│  │  ┌────────┐  │         │  ┌────────┐  │            │
│  │  │ WASM   │  │         │  │   OT   │  │            │
│  │  │   OT   │◄─┼─────────┼─►│  Lib   │  │            │
│  │  │ Bridge │  │         │  │        │  │            │
│  │  └────────┘  │         │  └────────┘  │            │
│  │      │       │         │      │       │            │
│  │      ▼       │         │      ▼       │            │
│  │  operation-  │         │  operation-  │            │
│  │  transforma- │         │  transforma- │            │
│  │  tion-go     │         │  tion-go     │            │
│  │  (WASM)      │         │  (native)    │            │
│  └──────────────┘         └──────────────┘            │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### Three Integration Points

1. **Server (Native Go)**
   - Location: `pkg/server/kolabpad.go`
   - Import: `github.com/shiv248/operational-transformation-go`
   - Role: Authority for all transformations

2. **WASM Bridge (Go → Browser)**
   - Location: `cmd/ot-wasm-bridge/`
   - Compiles OT library to WebAssembly
   - Exposes JavaScript API for frontend

3. **Frontend (TypeScript)**
   - Location: `frontend/src/services/kolabpad.ts`
   - Uses WASM OT for local predictions
   - Sends/receives operations via WebSocket

---

## WASM Bridge

The WASM bridge allows the browser to run the exact same OT code as the server, ensuring consistency.

### Architecture

```
┌─────────────────────────────────────────────────┐
│  cmd/ot-wasm-bridge/main.go                    │
├─────────────────────────────────────────────────┤
│                                                 │
│  1. Imports operational-transformation-go       │
│  2. Exposes global JS API via syscall/js       │
│  3. Compiles to ot.wasm                         │
│                                                 │
└─────────────────────────────────────────────────┘
                      │
                      │ GOOS=js GOARCH=wasm
                      ▼
┌─────────────────────────────────────────────────┐
│  frontend/public/ot.wasm                        │
│  +                                              │
│  frontend/public/wasm_exec.js                   │
└─────────────────────────────────────────────────┘
                      │
                      │ Loaded by browser
                      ▼
┌─────────────────────────────────────────────────┐
│  window.ot_wasm                                 │
│  ├─ OpSeq.new()                                 │
│  ├─ OpSeq.insert(text)                          │
│  ├─ OpSeq.delete(n)                             │
│  ├─ OpSeq.retain(n)                             │
│  ├─ transform(opA, opB)                         │
│  └─ apply(text, op)                             │
└─────────────────────────────────────────────────┘
```

### Key Files

- **Bridge**: [cmd/ot-wasm-bridge/main.go](../../cmd/ot-wasm-bridge/main.go)
- **Build**: `Makefile` target `wasm-build`
- **Frontend usage**: [frontend/src/services/kolabpad.ts](../../frontend/src/services/kolabpad.ts)

### Building WASM

```bash
make wasm-build
```

This compiles the OT library to `frontend/public/ot.wasm`.

---

## Message Flow

### Edit Flow with OT

```
Client A (rev=5)                Server (rev=5)            Client B (rev=5)
     │                               │                          │
     │  1. User types "hi"           │                          │
     ├──────────────────────────────►│                          │
     │  {type:"edit",                │                          │
     │   revision:5,                 │                          │
     │   operation:[0,"hi"]}         │                          │
     │                               │                          │
     │                          2. Server applies                │
     │                             operation                    │
     │                             revision → 6                 │
     │                               │                          │
     │  3. Ack                       │  4. Broadcast           │
     │◄──────────────────────────────┤──────────────────────────►
     │  {type:"ack",                 │  {type:"operation",      │
     │   revision:6}                 │   revision:6,            │
     │                               │   userId:1,              │
     │                               │   operation:[0,"hi"]}    │
```

### Concurrent Edits with Transform

```
Client A (rev=5)                Server (rev=5)            Client B (rev=5)
     │                               │                          │
     │  1. Types "A" at 0            │  2. Types "B" at 0      │
     ├──────────────────────────────►│◄──────────────────────────┤
     │  [0,"A"]                      │  [0,"B"]                 │
     │                               │                          │
     │                          3. Applies opA                  │
     │                             rev → 6                      │
     │                          4. Transforms opB               │
     │                             opB' = transform(opB, opA)   │
     │                             opB' = [1,"B"]              │
     │                          5. Applies opB'                 │
     │                             rev → 7                      │
     │                               │                          │
     │  6. Ack rev=6                 │  7. Broadcast opA        │
     │◄──────────────────────────────┤──────────────────────────►
     │                               │                          │
     │  8. Broadcast opB'            │  9. Ack rev=7           │
     │◄──────────────────────────────┼──────────────────────────►
     │                               │                          │
     │  Result: "AB"                 │  Result: "AB"            │
```

**Key Point**: Server performs the transform to ensure all clients converge to "AB".

---

## Server-Side OT

The server is the single source of truth and performs all transformations.

### Implementation

**Location**: [pkg/server/kolabpad.go](../../pkg/server/kolabpad.go)

**Key Function**: `applyEdit()`

```go
func (s *State) applyEdit(userId int, revision uint64, operation *ot.OperationSeq) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // 1. Validate revision
    if revision != s.CurrentRevision {
        return fmt.Errorf("invalid revision: got %d, current is %d", revision, s.CurrentRevision)
    }

    // 2. Apply operation to document text
    newText, err := operation.Apply(s.Text)
    if err != nil {
        return fmt.Errorf("apply operation: %w", err)
    }

    // 3. Update state
    s.Text = newText
    s.CurrentRevision++
    s.Operations = append(s.Operations, protocol.UserOperation{
        UserId:    userId,
        Revision:  s.CurrentRevision,
        Operation: operation,
    })

    return nil
}
```

### Transform for Concurrent Edits

When Client B's operation arrives at an old revision:

```go
// Client B's operation is at revision 5, but server is now at revision 7
// Transform through revisions 6 and 7

transformedOp := clientOp
for _, historicalOp := range s.Operations[clientRevision:currentRevision] {
    transformedOp, _ = ot.Transform(transformedOp, historicalOp)
}

// Apply transformed operation
s.applyEdit(userId, currentRevision, transformedOp)
```

---

## Client-Side OT

The frontend uses WASM OT for optimistic local edits and transformation.

### Implementation

**Location**: [frontend/src/services/kolabpad.ts](../../frontend/src/services/kolabpad.ts)

### Optimistic Updates

```typescript
// User types → immediately apply locally
const operation = window.ot_wasm.OpSeq.new()
operation.retain(cursorPos)
operation.insert(text)

// Apply optimistically
const newText = window.ot_wasm.apply(currentText, operation)
editor.setValue(newText)

// Send to server
this.sendOperation(operation, currentRevision)
```

### Handling Server Operations

```typescript
onServerOperation(serverOp: Operation, revision: number) {
  // Transform pending local operations
  for (const pendingOp of this.pendingOps) {
    const [pendingOp', serverOp'] = window.ot_wasm.transform(pendingOp, serverOp)
    pendingOp = pendingOp'
    serverOp = serverOp'
  }

  // Apply server operation
  this.currentText = window.ot_wasm.apply(this.currentText, serverOp)
  this.currentRevision = revision
}
```

---

## Testing Integration

Since OT algorithm tests are in the library, Kolabpad focuses on **integration testing**.

### End-to-End OT Tests

**Location**: [pkg/server/server_test.go](../../pkg/server/server_test.go)

**Test**: `TestConcurrentEdits`

```go
func TestConcurrentEdits(t *testing.T) {
    server := testServer(t)

    // Connect two clients
    client1 := connectClient(t, server, "doc1")
    client2 := connectClient(t, server, "doc1")

    // Both type at position 0 simultaneously
    sendEdit(client1, 0, [0, "A"])
    sendEdit(client2, 0, [0, "B"])

    // Both should receive operations and converge
    op1 := receiveOperation(client1)
    op2 := receiveOperation(client2)

    // Verify convergence
    text1 := applyOperations("", [op1, op2])
    text2 := applyOperations("", [op2, op1])

    assert.Equal(t, text1, text2) // Both converge to "AB" or "BA"
}
```

### What We Test

1. **Message protocol**: Correct JSON serialization/deserialization
2. **Revision tracking**: Server maintains correct revision numbers
3. **Broadcast behavior**: All clients receive operations
4. **Convergence**: Multiple clients reach same final state
5. **Edge cases**: Empty operations, Unicode, rapid typing

### What We Don't Test

Algorithm correctness (transform, compose, apply) — tested in [operational-transformation-go](https://github.com/shiv248/operational-transformation-go).

---

## References

### OT Library

- **Repository**: [shiv248/operational-transformation-go](https://github.com/shiv248/operational-transformation-go)
- **Algorithm docs**: [OT docs](https://github.com/shiv248/operational-transformation-go/tree/main/dev-internals/docs)
- **API reference**: See library README

### Kolabpad Components

- **Server integration**: [pkg/server/kolabpad.go](../../pkg/server/kolabpad.go)
- **WASM bridge**: [cmd/ot-wasm-bridge/](../../cmd/ot-wasm-bridge/)
- **Frontend client**: [frontend/src/services/kolabpad.ts](../../frontend/src/services/kolabpad.ts)
- **Protocol spec**: [01-websocket-protocol.md](../protocol/01-websocket-protocol.md)

### Related Documentation

- [System Overview](01-system-overview.md)
- [WebSocket Protocol](../protocol/01-websocket-protocol.md)
- [Testing Strategy](../development/02-testing-strategy.md)
