# Go WASM OT Bridge

Bridges the Go OT library (`pkg/ot`) to the browser by compiling to WebAssembly and exposing a JavaScript API.

## Architecture

Unlike the original Rustpad which used a Rust WASM npm package (`rustpad-wasm`), Kolabpad uses **Go WASM** that exposes a global JavaScript API.

```
┌─────────────────────────────────────────────────┐
│  Browser (JavaScript/TypeScript)                │
│                                                  │
│  ┌────────────────┐      ┌──────────────────┐  │
│  │  index.html    │─────▶│  Go WASM Runtime │  │
│  │  loads WASM    │      │  (wasm_exec.js)  │  │
│  └────────────────┘      └──────────────────┘  │
│                                   │              │
│                          exposes global         │
│                                   ▼              │
│                          ┌──────────────────┐  │
│                          │  OpSeq (global)  │  │
│                          └──────────────────┘  │
│                                   │              │
│                          used by  │              │
│                                   ▼              │
│                          ┌──────────────────┐  │
│                          │  kolabpad.ts     │  │
│                          │  (TypeScript)    │  │
│                          └──────────────────┘  │
└─────────────────────────────────────────────────┘
```

## Building

```bash
# Build the WASM module
GOOS=js GOARCH=wasm go build -o ot.wasm ./cmd/ot-wasm-bridge/

# Copy Go's WASM runtime
cp $(go env GOROOT)/misc/wasm/wasm_exec.js ./
```

In production (Dockerfile), these files are built and copied to `frontend/public/`.

## Loading in HTML

The WASM module is loaded via `index.html`:

```html
<!-- Load Go WASM runtime -->
<script src="/wasm_exec.js"></script>
<script type="module">
  import { logger } from '/src/logger.ts';

  // Load and initialize Go WASM module
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch('/ot.wasm'), go.importObject)
    .then(result => {
      go.run(result.instance);
      logger.info('WASM OT module loaded');
    })
    .catch(err => {
      logger.error('Failed to load WASM:', err);
    });
</script>
```

After loading, Go WASM exposes a **global `OpSeq` object** to JavaScript.

## TypeScript Usage

### Declaring the Global

In TypeScript files that use the WASM module, declare the global:

```typescript
// kolabpad.ts
declare const OpSeq: any;
```

### Creating Operations

```typescript
// Create new operation sequence
const operation = OpSeq.new();

// Build operation
operation.retain(5);        // Keep 5 characters
operation.insert("hello");  // Insert text
operation.delete(3);        // Delete 3 characters
operation.retain(10);       // Keep 10 more characters
```

### Operation Methods

```typescript
// Serialize to JSON (for sending to server)
const json = operation.to_string();  // Returns JSON string

// Parse from JSON (received from server)
const op = OpSeq.from_str(jsonString);

// Get operation lengths
const baseLen = operation.base_len();      // Original document length
const targetLen = operation.target_len();  // Resulting document length

// Apply to text
const newText = operation.apply(text);  // Throws on error

// Transform two concurrent operations (OT magic)
const pair = opA.transform(opB);
const aPrime = pair.first();   // Transformed A
const bPrime = pair.second();  // Transformed B

// Compose sequential operations
const composed = op1.compose(op2);

// Invert operation
const inverted = operation.invert(text);

// Transform cursor index through operation
const newIndex = operation.transform_index(cursorPos);

// Check if operation is a no-op
const isNoop = operation.is_noop();
```

## API Contract

### `OpSeq` (Global Object)

| Method | Parameters | Returns | Description |
|--------|-----------|---------|-------------|
| `new()` | none | `OpSeq` | Create empty operation |
| `from_str(json)` | `string` | `OpSeq` | Parse from JSON |

### `OpSeq` Instance Methods

| Method | Parameters | Returns | Description |
|--------|-----------|---------|-------------|
| `insert(text)` | `string` | `void` | Insert text |
| `delete(n)` | `number` | `void` | Delete n characters |
| `retain(n)` | `number` | `void` | Retain n characters |
| `to_string()` | none | `string` | Serialize to JSON |
| `apply(text)` | `string` | `string` | Apply to document |
| `transform(other)` | `OpSeq` | `Pair` | OT transform |
| `compose(other)` | `OpSeq` | `OpSeq` | Compose operations |
| `invert(text)` | `string` | `OpSeq` | Invert operation |
| `transform_index(pos)` | `number` | `number` | Transform cursor |
| `base_len()` | none | `number` | Input length |
| `target_len()` | none | `number` | Output length |
| `is_noop()` | none | `boolean` | Check if no-op |

### `Pair` Object (from `transform`)

```typescript
const pair = opA.transform(opB);
const aPrime = pair.first();   // Transformed A
const bPrime = pair.second();  // Transformed B
```

## JSON Format

Operations serialize as arrays with:
- **Positive numbers**: Retain N characters
- **Negative numbers**: Delete N characters
- **Strings**: Insert text

```json
[5, "hello", -3, 10]
```

This means:
1. Retain 5 characters
2. Insert "hello"
3. Delete 3 characters
4. Retain 10 characters

## Implementation Details

### Go → JavaScript Bridge

The Go WASM module uses `syscall/js` to expose functions:

```go
// main.go
js.Global().Set("OpSeq", opSeqConstructor)
```

Each `OpSeq` instance in JavaScript is backed by a Go `*ot.OperationSeq` stored in a registry with an ID.

### Memory Management

- Go objects are stored in a registry to prevent garbage collection
- JavaScript wrappers hold IDs to reference Go objects
- Currently no explicit cleanup (relies on page refresh)

### Error Handling

Go panics are caught and converted to JavaScript exceptions:

```typescript
try {
  const result = operation.apply(text);
} catch (err) {
  console.error('OT operation failed:', err);
}
```

## Differences from Rustpad WASM

| Feature | Rustpad (Rust) | Kolabpad (Go) |
|---------|----------------|---------------|
| Distribution | npm package | Direct WASM load |
| Import | `import * as wasm from "rustpad-wasm"` | Global `OpSeq` |
| Loading | Vite bundles it | `<script>` in HTML |
| API Style | Module exports | Global object |
| Memory | Auto GC | Registry-based |

## Development

### Testing Locally

```bash
# Terminal 1: Build WASM
GOOS=js GOARCH=wasm go build -o frontend/public/ot.wasm ./cmd/ot-wasm-bridge/
cp $(go env GOROOT)/misc/wasm/wasm_exec.js frontend/public/

# Terminal 2: Start dev server
cd frontend
npm run dev
```

### Debugging

Enable WASM debug logs in browser console:

```javascript
// In browser console
localStorage.setItem('LOG_LEVEL', 'debug');
location.reload();
```

## Future Improvements

- [ ] Add TypeScript type definitions (`opseq.d.ts`)
- [ ] Implement proper memory cleanup (registry cleanup)
- [ ] Add WASM unit tests (Go test with js/wasm build tag)
- [ ] Profile and optimize WASM performance
- [ ] Consider using TinyGo for smaller binary size
