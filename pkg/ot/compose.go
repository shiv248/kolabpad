package ot

// Compose merges two consecutive operations into one operation while preserving
// the changes of both. For each input string S and consecutive operations A and B:
//
//	apply(apply(S, A), B) = apply(S, compose(A, B))
//
// Returns an error if the operations are not composable (A's target length != B's base length).
//
// This is a direct port from Rust operational-transform:
// https://github.com/spebern/operational-transform-rs/blob/master/operational-transform/src/lib.rs#L162-L273
func (a *OperationSeq) Compose(b *OperationSeq) (*OperationSeq, error) {
	if a.targetLen != b.baseLen {
		return nil, ErrIncompatibleLengths
	}

	result := NewOperationSeq()
	ops1 := newOpIterator(a.ops)
	ops2 := newOpIterator(b.ops)

	op1 := ops1.next()
	op2 := ops2.next()

	for {
		// Both operations exhausted
		if op1 == nil && op2 == nil {
			break
		}

		// Delete from first operation takes priority
		if del, ok := op1.(Delete); ok {
			result.Delete(del.N)
			op1 = ops1.next()
			continue
		}

		// Insert from second operation takes priority
		if ins, ok := op2.(Insert); ok {
			result.Insert(ins.Text)
			op2 = ops2.next()
			continue
		}

		// One operation is nil but other isn't
		if op1 == nil || op2 == nil {
			return nil, ErrIncompatibleLengths
		}

		// Handle Retain vs Retain
		if ret1, ok1 := op1.(Retain); ok1 {
			if ret2, ok2 := op2.(Retain); ok2 {
				if ret1.N < ret2.N {
					result.Retain(ret1.N)
					op2 = Retain{N: ret2.N - ret1.N}
					op1 = ops1.next()
				} else if ret1.N == ret2.N {
					result.Retain(ret1.N)
					op1 = ops1.next()
					op2 = ops2.next()
				} else {
					result.Retain(ret2.N)
					op1 = Retain{N: ret1.N - ret2.N}
					op2 = ops2.next()
				}
				continue
			}
		}

		// Handle Insert vs Delete
		if ins, ok1 := op1.(Insert); ok1 {
			if del, ok2 := op2.(Delete); ok2 {
				insLen := uint64(charCount(ins.Text))
				if insLen < del.N {
					op2 = Delete{N: del.N - insLen}
					op1 = ops1.next()
				} else if insLen == del.N {
					op1 = ops1.next()
					op2 = ops2.next()
				} else {
					// Delete part of the insert
					runes := []rune(ins.Text)
					op1 = Insert{Text: string(runes[del.N:])}
					op2 = ops2.next()
				}
				continue
			}
		}

		// Handle Insert vs Retain
		if ins, ok1 := op1.(Insert); ok1 {
			if ret, ok2 := op2.(Retain); ok2 {
				insLen := uint64(charCount(ins.Text))
				if insLen < ret.N {
					result.Insert(ins.Text)
					op2 = Retain{N: ret.N - insLen}
					op1 = ops1.next()
				} else if insLen == ret.N {
					result.Insert(ins.Text)
					op1 = ops1.next()
					op2 = ops2.next()
				} else {
					// Retain part of the insert
					runes := []rune(ins.Text)
					result.Insert(string(runes[:ret.N]))
					op1 = Insert{Text: string(runes[ret.N:])}
					op2 = ops2.next()
				}
				continue
			}
		}

		// Handle Retain vs Delete
		if ret, ok1 := op1.(Retain); ok1 {
			if del, ok2 := op2.(Delete); ok2 {
				if ret.N < del.N {
					result.Delete(ret.N)
					op2 = Delete{N: del.N - ret.N}
					op1 = ops1.next()
				} else if ret.N == del.N {
					result.Delete(del.N)
					op2 = ops2.next()
					op1 = ops1.next()
				} else {
					result.Delete(del.N)
					op1 = Retain{N: ret.N - del.N}
					op2 = ops2.next()
				}
				continue
			}
		}

		// Should never reach here if operations are valid
		return nil, ErrIncompatibleLengths
	}

	return result, nil
}
