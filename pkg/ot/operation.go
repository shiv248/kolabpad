// Package ot implements Operational Transformation for collaborative editing.
//
// This is a direct port of the Rust operational-transform crate:
// https://github.com/spebern/operational-transform-rs
//
// Operational transformation (OT) is a technology for supporting real-time
// collaborative editing. When multiple users edit the same document concurrently,
// OT resolves conflicts to ensure all users converge to the same final state.
//
// The basic operations are:
//   - Retain(n): Move cursor n positions forward
//   - Delete(n): Delete n characters at current position
//   - Insert(s): Insert string s at current position
package ot

import (
	"errors"
	"unicode/utf8"
)

var (
	// ErrIncompatibleLengths is returned when operations have incompatible lengths
	ErrIncompatibleLengths = errors.New("incompatible lengths")
)

// Operation represents a single operation in a document.
// This is modeled as an interface to match Go idioms, but has three concrete types.
type Operation interface {
	isOperation()
}

// Retain moves the cursor n positions forward without modifying the document.
type Retain struct {
	N uint64
}

func (Retain) isOperation() {}

// Delete removes n characters at the current cursor position.
type Delete struct {
	N uint64
}

func (Delete) isOperation() {}

// Insert adds text at the current cursor position.
type Insert struct {
	Text string
}

func (Insert) isOperation() {}

// charCount returns the number of UTF-8 characters (runes) in a string.
// This is critical for compatibility - we count Unicode codepoints, not bytes.
func charCount(s string) int {
	return utf8.RuneCountInString(s)
}

// OperationSeq is a sequence of operations on text.
// It tracks both the required input length (baseLen) and the resulting output length (targetLen).
type OperationSeq struct {
	ops       []Operation
	baseLen   int // Required length of input string
	targetLen int // Length of string after applying operations
}

// NewOperationSeq creates a new empty operation sequence.
func NewOperationSeq() *OperationSeq {
	return &OperationSeq{
		ops:       make([]Operation, 0),
		baseLen:   0,
		targetLen: 0,
	}
}

// WithCapacity creates a new operation sequence with pre-allocated capacity.
func WithCapacity(capacity int) *OperationSeq {
	return &OperationSeq{
		ops:       make([]Operation, 0, capacity),
		baseLen:   0,
		targetLen: 0,
	}
}

// BaseLen returns the required length of a string these operations can be applied to.
func (o *OperationSeq) BaseLen() int {
	return o.baseLen
}

// TargetLen returns the length of the resulting string after operations are applied.
func (o *OperationSeq) TargetLen() int {
	return o.targetLen
}

// Ops returns the underlying slice of operations.
func (o *OperationSeq) Ops() []Operation {
	return o.ops
}

// IsNoop returns true if this operation has no effect.
func (o *OperationSeq) IsNoop() bool {
	if len(o.ops) == 0 {
		return true
	}
	if len(o.ops) == 1 {
		if _, ok := o.ops[0].(Retain); ok {
			return true
		}
	}
	return false
}

// Insert adds text at the current cursor position.
// This merges with the previous Insert operation if possible.
func (o *OperationSeq) Insert(s string) {
	if s == "" {
		return
	}

	o.targetLen += charCount(s)

	n := len(o.ops)
	if n == 0 {
		o.ops = append(o.ops, Insert{Text: s})
		return
	}

	// Try to merge with last operation
	if insert, ok := o.ops[n-1].(Insert); ok {
		o.ops[n-1] = Insert{Text: insert.Text + s}
		return
	}

	// Check if we need to swap with Delete and merge with previous Insert
	if n >= 2 {
		if _, ok := o.ops[n-1].(Delete); ok {
			if insert, ok := o.ops[n-2].(Insert); ok {
				o.ops[n-2] = Insert{Text: insert.Text + s}
				return
			}
		}
	}

	// If last operation is Delete, we need to insert the Insert before it
	if del, ok := o.ops[n-1].(Delete); ok {
		o.ops[n-1] = Insert{Text: s}
		o.ops = append(o.ops, del)
		return
	}

	// Default: just append
	o.ops = append(o.ops, Insert{Text: s})
}

// Delete removes n characters at the current cursor position.
// This merges with the previous Delete operation if possible.
func (o *OperationSeq) Delete(n uint64) {
	if n == 0 {
		return
	}

	o.baseLen += int(n)

	if len(o.ops) > 0 {
		if del, ok := o.ops[len(o.ops)-1].(Delete); ok {
			o.ops[len(o.ops)-1] = Delete{N: del.N + n}
			return
		}
	}

	o.ops = append(o.ops, Delete{N: n})
}

// Retain moves the cursor n positions forward.
// This merges with the previous Retain operation if possible.
func (o *OperationSeq) Retain(n uint64) {
	if n == 0 {
		return
	}

	o.baseLen += int(n)
	o.targetLen += int(n)

	if len(o.ops) > 0 {
		if ret, ok := o.ops[len(o.ops)-1].(Retain); ok {
			o.ops[len(o.ops)-1] = Retain{N: ret.N + n}
			return
		}
	}

	o.ops = append(o.ops, Retain{N: n})
}

// add is an internal helper to add any operation type.
func (o *OperationSeq) add(op Operation) {
	switch v := op.(type) {
	case Retain:
		o.Retain(v.N)
	case Delete:
		o.Delete(v.N)
	case Insert:
		o.Insert(v.Text)
	}
}
