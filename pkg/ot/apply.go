package ot

import (
	"strings"
)

// Apply applies an operation sequence to a string, returning the transformed string.
//
// Returns an error if the operation's base length doesn't match the string length.
//
// This is a direct port from Rust operational-transform:
// https://github.com/spebern/operational-transform-rs/blob/master/operational-transform/src/lib.rs#L473-L503
func (o *OperationSeq) Apply(s string) (string, error) {
	if charCount(s) != o.baseLen {
		return "", ErrIncompatibleLengths
	}

	var result strings.Builder
	runes := []rune(s)
	idx := 0

	for _, op := range o.ops {
		switch v := op.(type) {
		case Retain:
			// Copy n characters from input
			for i := uint64(0); i < v.N && idx < len(runes); i++ {
				result.WriteRune(runes[idx])
				idx++
			}
		case Delete:
			// Skip n characters from input
			idx += int(v.N)
		case Insert:
			// Add the inserted text
			result.WriteString(v.Text)
		}
	}

	return result.String(), nil
}

// Invert computes the inverse of an operation. The inverse reverts the effects
// of the operation. For example:
//   - insert("hello") → delete(5)
//   - delete(5) → insert("hello")
//   - retain(n) → retain(n)
//
// The inverse is useful for implementing undo functionality.
//
// This is a direct port from Rust operational-transform:
// https://github.com/spebern/operational-transform-rs/blob/master/operational-transform/src/lib.rs#L505-L530
func (o *OperationSeq) Invert(s string) *OperationSeq {
	inverse := NewOperationSeq()
	runes := []rune(s)
	idx := 0

	for _, op := range o.ops {
		switch v := op.(type) {
		case Retain:
			inverse.Retain(v.N)
			idx += int(v.N)
		case Insert:
			inverse.Delete(uint64(charCount(v.Text)))
		case Delete:
			// Insert the deleted characters back
			deleted := string(runes[idx : idx+int(v.N)])
			inverse.Insert(deleted)
			idx += int(v.N)
		}
	}

	return inverse
}
