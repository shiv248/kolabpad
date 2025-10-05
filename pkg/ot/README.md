# Operational Transform (OT) for Go

A Go port of the Rust [`operational-transform`](https://github.com/spebern/operational-transform-rs) library, which itself is a port of [ot.js](https://github.com/Operational-Transformation/ot.js).

## Overview

Operational Transformation (OT) is an algorithm for supporting real-time collaborative editing in distributed systems. It enables multiple users to concurrently edit the same document while maintaining consistency across all copies, without using locks or requiring users to wait for each other.

## Features

- ✅ **Direct port from Rust** - Maintains identical behavior to the battle-tested Rust implementation
- ✅ **UTF-8 character handling** - Correctly counts Unicode codepoints (not bytes)
- ✅ **JSON wire format compatibility** - Compatible with Rust and JavaScript implementations
- ✅ **Complete test coverage** - All tests ported from Rust pass
- ✅ **Type-safe operations** - Go's type system prevents invalid operations

## Operations

Three basic operations:

- **Retain(n)**: Move cursor forward n positions without changing anything
- **Delete(n)**: Delete n characters at current position
- **Insert(s)**: Insert string at current position

## Usage

```go
import "github.com/shiv/kolabpad/pkg/ot"

// Create operations
op := ot.NewOperationSeq()
op.Retain(5)
op.Insert("world")
op.Delete(3)

// Apply to text
result, err := op.Apply("hello123")
// result = "helloworld"

// Transform concurrent operations
a := ot.NewOperationSeq()
a.Insert("A")

b := ot.NewOperationSeq()
b.Insert("B")

aPrime, bPrime, err := a.Transform(b)
// Both clients converge to same result

// Compose sequential operations
c, err := a.Compose(b)
// c = single operation equivalent to a followed by b

// Invert for undo
inverse := op.Invert("original text")
```

## JSON Serialization

Compatible with Rust/JavaScript wire format:

```go
import "encoding/json"

op := ot.NewOperationSeq()
op.Retain(1)
op.Delete(1)
op.Insert("abc")

data, _ := json.Marshal(op)
// data = [1, -1, "abc"]

var op2 ot.OperationSeq
json.Unmarshal(data, &op2)
```

## Testing

```bash
go test ./pkg/ot/...
```

All tests ported from Rust operational-transform pass successfully.

## References

- [Rust operational-transform](https://github.com/spebern/operational-transform-rs)
- [ot.js](https://github.com/Operational-Transformation/ot.js)
- [OT FAQ](https://www3.ntu.edu.sg/scse/staff/czsun/projects/otfaq/)
- [Rustpad](https://github.com/ekzhang/rustpad)

## License

MIT - Same as the Rust and JavaScript implementations
