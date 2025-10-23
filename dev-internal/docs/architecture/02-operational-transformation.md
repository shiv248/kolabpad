# Operational Transformation

**Purpose**: This document explains the Operational Transformation (OT) algorithm used in Kolabpad for resolving concurrent edits in real-time collaboration.

**Audience**: Developers implementing collaborative features, contributors debugging OT issues, architects evaluating conflict resolution strategies.

---

## Why Operational Transformation?

### OT vs CRDT Comparison

Kolabpad uses **Operational Transformation** instead of **Conflict-free Replicated Data Types** (CRDTs). This decision was made for several reasons:

#### Operational Transformation Advantages

**Sequential operations**: Operations have a total order defined by the server
```pseudocode
Server assigns revision numbers:
    operation 1 → revision 1
    operation 2 → revision 2
    operation 3 → revision 3

All clients see operations in same order
```

**Server authority**: Single source of truth eliminates ambiguity
- Server decides which operations conflict and how to resolve them
- Deterministic resolution: same inputs always produce same output
- Easier to debug: centralized decision making

**Smaller payloads**: OT operations are compact
```json
// OT operation (compact)
[10, "hello", 5]  // ~15 bytes

// CRDT operation (larger state)
{
  "siteId": "abc123",
  "counter": 42,
  "positions": [[0, 5], [1, 3]],
  "tombstones": [...],
  "value": "hello"
}  // ~100+ bytes
```

**Proven technology**: Used in production systems
- Google Docs (early versions used OT)
- Etherpad
- ShareDB
- CodeMirror's collaborative editing

#### CRDT Advantages (Why We Don't Use Them)

**Peer-to-peer**: Can work without server
- **Not relevant for Kolabpad**: We have a server-centric architecture

**Conflict-free**: No transformation needed
- **Not an advantage**: OT transformation is fast and well-understood

**Partition tolerance**: Works across network splits
- **Not needed**: Kolabpad is CP (Consistent + Partition-tolerant), not AP

**Complexity trade-off**:
- CRDTs avoid transform logic but add state management complexity
- Larger memory footprint (tracking tombstones, version vectors, etc.)
- Harder to reason about for debugging

### Decision Rationale

OT fits Kolabpad's design better because:
1. **Server is already required** for other features (OTP, persistence)
2. **Sequential operations easier to debug** than distributed CRDT state
3. **Smaller payloads** reduce bandwidth usage
4. **Simpler mental model** for contributors to understand

**Trade-off accepted**: Requires server to be online (CRDTs can work offline). This is acceptable for Kolabpad's ephemeral collaboration use case.

---

## OT Fundamentals

### Operation Types

There are three fundamental operations:

#### Retain(n)
**Meaning**: Keep `n` characters unchanged and move cursor forward

```pseudocode
APPLY Retain(5) to "hello world":
    cursor starts at position 0
    skip first 5 characters ("hello")
    cursor now at position 5 (before " world")
    document unchanged: "hello world"
```

#### Insert(text)
**Meaning**: Insert `text` at current cursor position

```pseudocode
APPLY Insert(" there") to "hello world" at position 5:
    document: "hello world"
    cursor at position 5
    insert " there"
    result: "hello there world"
```

#### Delete(n)
**Meaning**: Delete `n` characters at current cursor position

```pseudocode
APPLY Delete(6) to "hello world" at position 5:
    document: "hello world"
    cursor at position 5
    delete 6 characters (" world")
    result: "hello"
```

### Operation Sequences

Operations are combined into sequences to represent complex edits:

```pseudocode
SEQUENCE [Retain(6), Insert("beautiful "), Retain(5)]:
    applied to "hello world"

    Step 1: Retain(6)
        cursor: 0 → 6
        text: "hello world" (unchanged)

    Step 2: Insert("beautiful ")
        cursor: 6 (unchanged)
        text: "hello beautiful world"

    Step 3: Retain(5)
        cursor: 6 → 11
        text: "hello beautiful world" (unchanged)

    Final: "hello beautiful world"
```

### Base Length and Target Length

Every operation sequence has two important properties:

**Base Length**: Required length of input string
```pseudocode
[Retain(5), Delete(6), Insert(" there")]
    baseLen = 5 + 6 = 11  (Retain + Delete)
    Can only apply to strings with 11 characters
```

**Target Length**: Length of output string after applying operation
```pseudocode
[Retain(5), Delete(6), Insert(" there")]
    targetLen = 5 + 6 = 11  (Retain + Insert character count)
    Result will have 11 characters
```

---

## Operation Format

### JSON Wire Format

Operations are serialized as JSON arrays for transmission:

```json
// Example 1: Insert "hello" at position 10
[10, "hello"]

// Means: Retain(10), Insert("hello")
// Base length: 10, Target length: 15

// Example 2: Delete 5 characters at position 3
[3, -5]

// Means: Retain(3), Delete(5)
// Base length: 8, Target length: 3

// Example 3: Replace "world" with "there" at position 6
[6, -5, "there"]

// Means: Retain(6), Delete(5), Insert("there")
// Base length: 11, Target length: 11
```

**Format rules**:
- Positive number `n`: `Retain(n)`
- Negative number `-n`: `Delete(n)`
- String `s`: `Insert(s)`

**Optimization**: Consecutive operations of same type are merged
```json
// Instead of: [1, 1, 1, "h", "e", "l"]
// Merged to:  [3, "hel"]
```

### Internal Representation

In Go code, operations are represented as:

```go
type Operation interface {
    isOperation()
}

type Retain struct { N uint64 }
type Delete struct { N uint64 }
type Insert struct { Text string }

type OperationSeq struct {
    ops       []Operation
    baseLen   int
    targetLen int
}
```

---

## Transform Algorithm

### The Core Problem

When two users edit the same document simultaneously:

```pseudocode
SCENARIO: Two users edit "hello"

    Document: "hello" (revision 0)

    User A types " world" at end:
        Operation A: [5, " world"]
        A's expected result: "hello world"

    User B types "beautiful " after "hello" (simultaneously):
        Operation B: [5, "beautiful "]
        B's expected result: "hellobeautiful "

    Problem: Both operations based on same state (revision 0)
    Need to reconcile them!
```

### Transform Function

The transform function takes two concurrent operations and produces transformed versions:

```pseudocode
FUNCTION transform(opA, opB):
    // Given:
    //   - opA and opB are concurrent (same base state)
    //   - Both have same baseLen

    // Returns: (opA', opB') such that:
    //   apply(apply(state, opA), opB') == apply(apply(state, opB), opA')

    // This ensures convergence: both paths lead to same result
```

### Transform Cases

#### Case 1: Insert vs Insert

Both users insert at same position - use lexicographic ordering for determinism:

```pseudocode
TRANSFORM Insert("abc") vs Insert("xyz"):
    IF "abc" < "xyz":  // Lexicographic comparison
        opA' = Insert("abc")
        opB' = [3, "xyz"]  // Retain(3) to skip "abc", then Insert("xyz")
    ELSE:
        opA' = [3, "abc"]  // Retain(3) to skip "xyz", then Insert("abc")
        opB' = Insert("xyz")

    Result: Deterministic ordering regardless of network timing
```

#### Case 2: Insert vs Retain

Insert takes precedence - it shifts the retain forward:

```pseudocode
TRANSFORM Insert("hello") vs Retain(5):
    opA' = Insert("hello")  // Insert unchanged
    opB' = Retain(10)       // Retain shifted by insert length (5 → 10)
```

#### Case 3: Insert vs Delete

Insert happens "before" delete - shifts delete forward:

```pseudocode
TRANSFORM Insert("new") vs Delete(3):
    opA' = Insert("new")  // Insert unchanged
    opB' = [3, -3]        // Retain(3) to skip insert, then Delete(3)
```

#### Case 4: Delete vs Delete

Both delete same text - only one deletion needed:

```pseudocode
TRANSFORM Delete(5) vs Delete(5):
    // Both delete same characters
    opA' = <noop>  // Already deleted by opB
    opB' = <noop>  // Already deleted by opA

    // Implementation: Shorter delete wins
    IF lenA < lenB:
        opA' = <noop>
        opB' = Delete(lenB - lenA)
```

#### Case 5: Retain vs Retain

Both retain - advance both by minimum:

```pseudocode
TRANSFORM Retain(5) vs Retain(3):
    // Advance both by minimum (3)
    opA' = Retain(5)  // Still needs to retain all 5
    opB' = Retain(3)  // Already retained all 3

    // Implementation tracks partial consumption
```

---

## Operation Lifecycle

### Client-Side Flow

```pseudocode
USER types "hello" in editor:
    currentCursor = editor.getCursorPosition()  // e.g., 10

    // Create operation
    operation = [currentCursor, "hello"]

    // Check if there are pending operations
    IF pendingOperations is not empty:
        // Compose with pending ops for efficiency
        operation = compose(pendingOperations, operation)
        pendingOperations = []

    // Send to server
    SEND {
        "Edit": {
            "revision": myRevision,  // Client's current revision
            "operation": operation
        }
    }

    // Add to pending buffer (awaiting server acknowledgment)
    pendingOperations.push(operation)
    myRevision += 1
```

### Server-Side Flow

```pseudocode
SERVER receives Edit message from clientA:
    receivedRevision = message.revision
    serverRevision = document.revision()

    operation = message.operation

    // Transform if client is behind
    IF receivedRevision < serverRevision:
        missedOperations = document.operations[receivedRevision : serverRevision]

        FOR EACH missedOp IN missedOperations:
            operation, missedOp' = transform(operation, missedOp)

        // Now operation is transformed to apply at current revision

    // Validate operation
    IF operation.baseLen != document.textLength():
        REJECT "Operation base length mismatch"
        RETURN

    // Apply operation to document
    newText = operation.apply(document.text)

    IF len(newText) > maxDocumentSize:
        REJECT "Document too large"
        RETURN

    document.text = newText
    document.operations.append({
        userId: clientA.userId,
        operation: operation
    })
    document.revision += 1
    document.lastEditTime = now()

    // Broadcast to ALL clients (including sender)
    BROADCAST {
        "History": {
            "start": serverRevision,
            "operations": [
                {
                    "id": clientA.userId,
                    "operation": operation
                }
            ]
        }
    }
```

### Client Acknowledgment Flow

```pseudocode
CLIENT receives History message:
    FOR EACH operation IN message.operations:
        IF operation.userId == myUserId:
            // This is my operation coming back (acknowledgment)
            pendingOperations.remove_first()
            // No need to apply - already in editor

        ELSE:
            // Another user's operation

            // Transform against pending operations
            FOR EACH pendingOp IN pendingOperations:
                operation, pendingOp' = transform(operation, pendingOp)
                pendingOp = pendingOp'  // Update pending op

            // Apply to editor
            applyToMonacoEditor(operation)

        myRevision += 1
```

---

## Unicode and Codepoint Handling

### The Problem

Different systems count string positions differently:

```javascript
// JavaScript (UTF-16):
"hello 😀".length  // 8 (emoji counts as 2 UTF-16 code units)

// Go (UTF-8 runes):
utf8.RuneCountInString("hello 😀")  // 7 (emoji is 1 rune)

// OT operations:
"hello 😀" has 7 positions (Unicode codepoints)
```

### Solution: Unicode Codepoints

Kolabpad **always** uses Unicode codepoint offsets:

```pseudocode
TEXT: "hello 😀 world"

Positions (codepoint offsets):
    h     e     l     l     o          😀          w     o     r     l     d
    0     1     2     3     4     5     6     7     8     9    10    11

Operation [6, "beautiful "]:
    Retain 6 codepoints (up to and including 😀)
    Insert "beautiful "
    Result: "hello 😀beautiful  world"
```

### Conversion Functions

When interfacing with Monaco editor (uses UTF-16):

```pseudocode
FUNCTION codepointOffsetToUTF16(text, codepointOffset):
    runes = text.toRunes()
    utf16Offset = 0

    FOR i = 0 TO codepointOffset - 1:
        rune = runes[i]
        IF rune > 0xFFFF:  // Surrogate pair in UTF-16
            utf16Offset += 2
        ELSE:
            utf16Offset += 1

    RETURN utf16Offset

FUNCTION utf16OffsetToCodepoint(text, utf16Offset):
    runes = text.toRunes()
    codepointOffset = 0
    currentUTF16 = 0

    WHILE currentUTF16 < utf16Offset:
        rune = runes[codepointOffset]
        IF rune > 0xFFFF:
            currentUTF16 += 2
        ELSE:
            currentUTF16 += 1
        codepointOffset += 1

    RETURN codepointOffset
```

---

## Compose: Operation Merging

### Why Compose?

Composing operations improves efficiency:

```pseudocode
WITHOUT compose:
    User types "hello" (5 keystrokes)
    → 5 separate operations sent to server
    → 5 broadcast messages to all clients

WITH compose:
    User types "hello" (5 keystrokes)
    → Batched into 1 operation
    → 1 broadcast message to all clients
```

### Compose Algorithm

```pseudocode
FUNCTION compose(op1, op2):
    // Combines two sequential operations into one
    // Requirement: op1.targetLen == op2.baseLen

    result = new OperationSeq()

    iter1 = iterator(op1)
    iter2 = iterator(op2)

    comp1 = iter1.next()
    comp2 = iter2.next()

    WHILE comp1 OR comp2:
        // Delete from first operation
        IF comp1 is Delete:
            result.delete(comp1.n)
            comp1 = iter1.next()

        // Insert from second operation
        ELSE IF comp2 is Insert:
            result.insert(comp2.text)
            comp2 = iter2.next()

        // Retain in both operations
        ELSE IF comp1 is Retain AND comp2 is Retain:
            min = minimum(comp1.n, comp2.n)
            result.retain(min)
            comp1 = consume(comp1, min)
            comp2 = consume(comp2, min)

        // Delete in second operation
        ELSE IF comp1 is Retain AND comp2 is Delete:
            result.delete(comp2.n)
            comp1 = consume(comp1, comp2.n)
            comp2 = iter2.next()

        // Insert from first operation, anything from second
        ELSE IF comp1 is Insert AND comp2 is Delete:
            // Insert is immediately deleted - cancel out
            comp1 = consume(comp1, comp2.n)
            comp2 = iter2.next()

        ELSE IF comp1 is Insert AND comp2 is Retain:
            result.insert(comp1.text)
            comp2 = consume(comp2, comp1.text.length)
            comp1 = iter1.next()

    RETURN result
```

---

## Why OT History Doesn't Persist

### In-Memory History

While a document is active, the server keeps the full operation history:

```pseudocode
Document {
    operations: [
        { userId: 1, op: [0, "hello"] },
        { userId: 2, op: [5, " world"] },
        { userId: 1, op: [11, "!"] }
    ],
    text: "hello world!",
    revision: 3
}
```

**Purpose**: Needed for transforming late-arriving operations

### Persisted State

When writing to database, **only the final text** is stored:

```sql
INSERT INTO document (id, text, language, otp)
VALUES ('abc123', 'hello world!', 'plaintext', NULL)
```

**History is discarded** - only the reconstructed state matters

### Rationale

**History only needed for active collaboration**:
- When all users disconnect, no more concurrent operations arrive
- Future operations will be based on the final text, not old revisions

**On reload from database**:
```pseudocode
persistedDoc = database.load("abc123")

document = new Document()
document.text = persistedDoc.text
document.operations = [
    {
        userId: SYSTEM_USER,
        operation: [0, persistedDoc.text]  // Single insert of entire text
    }
]
document.revision = 1
```

**Benefits**:
- Smaller database (store ~5KB instead of ~5MB of history)
- Faster persistence (write one string, not thousands of operations)
- Simpler recovery (no need to replay operations)

---

## Convergence Guarantees

### Eventual Consistency

All clients converge to the same state:

```pseudocode
INVARIANT after all operations applied:
    FOR EACH pair of clients (A, B):
        client_A.text == client_B.text
        client_A.revision == client_B.revision
```

**Proof sketch**:
1. Server assigns total order to operations (revisions)
2. Transform ensures commutativity: `apply(apply(S, opA), opB') == apply(apply(S, opB), opA')`
3. All clients receive operations in same order
4. Therefore all clients reach same final state

### Deterministic Resolution

Same operations always produce same result:

```pseudocode
PROPERTY: Determinism
    transform(opA, opB) always returns same (opA', opB')

    Therefore:
        IF client_1 transforms opA against opB
        AND client_2 transforms opA against opB
        THEN client_1.result == client_2.result
```

This is guaranteed by:
- Lexicographic ordering for Insert vs Insert
- Fixed precedence rules for all other cases
- No randomness in transform algorithm

### No Split-Brain Scenarios

Server is single authority:

```pseudocode
SCENARIO: Network partition

    ClientA --- X (disconnected)
    ClientB --- X (disconnected)
            \  /
           Server (still running)

    Problem with peer-to-peer: A and B could diverge

    Kolabpad solution:
        ClientA cannot edit (WebSocket disconnected)
        ClientB cannot edit (WebSocket disconnected)
        No divergence possible - server has authoritative state

    On reconnect:
        ClientA reconnects → receives full history
        ClientB reconnects → receives full history
        Both converge to server's state
```

---

## Performance Characteristics

### Transform Complexity

Time complexity: **O(n + m)** where n = length of op1, m = length of op2

```pseudocode
WORST CASE:
    op1 = [1, 1, 1, 1, ...]  // n operations
    op2 = [1, 1, 1, 1, ...]  // m operations

    transform must iterate through all components
    → O(n + m) time
```

Space complexity: **O(n + m)** for result operations

### Apply Complexity

Time complexity: **O(n + t)** where n = operation length, t = text length

```pseudocode
WORST CASE:
    operation = [1, 1, 1, ..., -1000000]  // Many small operations
    text = very large string

    Must iterate through operation components and build result string
    → O(n + t) time
```

### Compose Complexity

Time complexity: **O(n + m)** where n = length of op1, m = length of op2

Similar to transform, must iterate through both operation sequences.

---

## Common Pitfalls and Edge Cases

### Empty Documents

```pseudocode
EDGE CASE: First insert into empty document

    Document: ""
    Operation: [0, "hello"]

    baseLen = 0 ✓
    targetLen = 5 ✓
    Result: "hello" ✓
```

### Deleting More Than Exists

```pseudocode
INVALID operation:
    Document: "hello" (length 5)
    Operation: [0, -10]  // Try to delete 10 characters

    baseLen = 10, but document length = 5
    → Rejected with ErrIncompatibleLengths
```

### Concurrent Inserts at Same Position

```pseudocode
SCENARIO:
    Document: "hello"
    UserA: [5, " world"]
    UserB: [5, "!"]

    Transform:
        "!" < " world" (lexicographic)
        → UserA' = [5, " world"]
        → UserB' = [6, "!"]  // Shifted by insert length

    Result: "hello! world" (deterministic)
```

---

## Testing OT Implementation

### Test Cases

Critical tests in `pkg/ot/*_test.go`:

```pseudocode
TEST transform_insert_vs_insert:
    op1 = [0, "a"]
    op2 = [0, "b"]
    (op1', op2') = transform(op1, op2)

    ASSERT apply(apply("", op1), op2') == apply(apply("", op2), op1')

TEST transform_delete_vs_delete:
    op1 = [0, -5]
    op2 = [0, -3]
    document = "hello"
    (op1', op2') = transform(op1, op2)

    ASSERT convergence

TEST compose_sequential_operations:
    op1 = [0, "hello"]
    op2 = [5, " world"]
    composed = compose(op1, op2)

    ASSERT apply(apply("", op1), op2) == apply("", composed)
```

---

## References and Further Reading

**Implementation**:
- Go OT library: `pkg/ot/`
- Transform logic: `pkg/ot/transform.go`
- Apply logic: `pkg/ot/apply.go`
- Compose logic: `pkg/ot/compose.go`
- Tests: `pkg/ot/*_test.go`

**Related Documentation**:
- [01-system-overview.md]: High-level architecture
- [03-persistence-strategy.md]: Why operation history isn't persisted
- [../backend/01-server-architecture.md]: Server-side operation handling
- [../frontend/02-state-synchronization.md]: Client-side operation handling
- [../protocol/01-websocket-protocol.md]: Wire format for operations

**External References**:
- Original Rust OT library: https://github.com/spebern/operational-transform-rs
- OT research: "High Latency, Low Bandwidth Windowing in the Jupiter Collaboration System" (1995)
- ShareDB OT documentation: https://github.com/share/sharedb

---

## Summary

Operational Transformation in Kolabpad:
- **Three operations**: Retain, Insert, Delete
- **Transform function**: Resolves concurrent edits deterministically
- **Server authority**: Single source of truth prevents conflicts
- **Unicode codepoints**: Consistent position handling across platforms
- **Composition**: Batches operations for efficiency
- **Eventual consistency**: All clients converge to same state
- **No persistence of history**: Only final text stored in database

For practical usage, see test files and the server's operation handling in `pkg/server/connection.go`.
