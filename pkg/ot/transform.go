package ot

// Transform takes two concurrent operations A and B that happened on the same document state
// and produces two new operations A' and B' such that:
//
//	apply(apply(S, A), B') = apply(apply(S, B), A')
//
// This is the heart of Operational Transformation.
//
// Returns an error if the operations have incompatible base lengths.
//
// This is a direct port from Rust operational-transform:
// https://github.com/spebern/operational-transform-rs/blob/master/operational-transform/src/lib.rs#L335-L471
func (a *OperationSeq) Transform(b *OperationSeq) (*OperationSeq, *OperationSeq, error) {
	if a.baseLen != b.baseLen {
		return nil, nil, ErrIncompatibleLengths
	}

	aPrime := NewOperationSeq()
	bPrime := NewOperationSeq()

	ops1 := newOpIterator(a.ops)
	ops2 := newOpIterator(b.ops)

	op1 := ops1.next()
	op2 := ops2.next()

	for {
		// Both operations exhausted
		if op1 == nil && op2 == nil {
			break
		}

		// Handle Insert vs Insert - use string comparison for tie-breaking
		if ins1, ok1 := op1.(Insert); ok1 {
			if ins2, ok2 := op2.(Insert); ok2 {
				if ins1.Text < ins2.Text {
					aPrime.Insert(ins1.Text)
					bPrime.Retain(uint64(charCount(ins1.Text)))
					op1 = ops1.next()
				} else if ins1.Text == ins2.Text {
					aPrime.Insert(ins1.Text)
					aPrime.Retain(uint64(charCount(ins1.Text)))
					bPrime.Insert(ins2.Text)
					bPrime.Retain(uint64(charCount(ins2.Text)))
					op1 = ops1.next()
					op2 = ops2.next()
				} else {
					aPrime.Retain(uint64(charCount(ins2.Text)))
					bPrime.Insert(ins2.Text)
					op2 = ops2.next()
				}
				continue
			}
		}

		// Handle Insert from first operation
		if ins, ok := op1.(Insert); ok {
			aPrime.Insert(ins.Text)
			bPrime.Retain(uint64(charCount(ins.Text)))
			op1 = ops1.next()
			continue
		}

		// Handle Insert from second operation
		if ins, ok := op2.(Insert); ok {
			aPrime.Retain(uint64(charCount(ins.Text)))
			bPrime.Insert(ins.Text)
			op2 = ops2.next()
			continue
		}

		// One operation is nil but other isn't (after handling inserts)
		if op1 == nil || op2 == nil {
			return nil, nil, ErrIncompatibleLengths
		}

		// Handle Retain vs Retain
		if ret1, ok1 := op1.(Retain); ok1 {
			if ret2, ok2 := op2.(Retain); ok2 {
				if ret1.N < ret2.N {
					aPrime.Retain(ret1.N)
					bPrime.Retain(ret1.N)
					op2 = Retain{N: ret2.N - ret1.N}
					op1 = ops1.next()
				} else if ret1.N == ret2.N {
					aPrime.Retain(ret1.N)
					bPrime.Retain(ret1.N)
					op1 = ops1.next()
					op2 = ops2.next()
				} else {
					aPrime.Retain(ret2.N)
					bPrime.Retain(ret2.N)
					op1 = Retain{N: ret1.N - ret2.N}
					op2 = ops2.next()
				}
				continue
			}
		}

		// Handle Delete vs Delete
		if del1, ok1 := op1.(Delete); ok1 {
			if del2, ok2 := op2.(Delete); ok2 {
				if del1.N < del2.N {
					op2 = Delete{N: del2.N - del1.N}
					op1 = ops1.next()
				} else if del1.N == del2.N {
					op1 = ops1.next()
					op2 = ops2.next()
				} else {
					op1 = Delete{N: del1.N - del2.N}
					op2 = ops2.next()
				}
				continue
			}
		}

		// Handle Delete vs Retain
		if del, ok1 := op1.(Delete); ok1 {
			if ret, ok2 := op2.(Retain); ok2 {
				if del.N < ret.N {
					aPrime.Delete(del.N)
					op2 = Retain{N: ret.N - del.N}
					op1 = ops1.next()
				} else if del.N == ret.N {
					aPrime.Delete(del.N)
					op1 = ops1.next()
					op2 = ops2.next()
				} else {
					aPrime.Delete(ret.N)
					op1 = Delete{N: del.N - ret.N}
					op2 = ops2.next()
				}
				continue
			}
		}

		// Handle Retain vs Delete
		if ret, ok1 := op1.(Retain); ok1 {
			if del, ok2 := op2.(Delete); ok2 {
				if ret.N < del.N {
					bPrime.Delete(ret.N)
					op2 = Delete{N: del.N - ret.N}
					op1 = ops1.next()
				} else if ret.N == del.N {
					bPrime.Delete(ret.N)
					op1 = ops1.next()
					op2 = ops2.next()
				} else {
					bPrime.Delete(del.N)
					op1 = Retain{N: ret.N - del.N}
					op2 = ops2.next()
				}
				continue
			}
		}

		// Should never reach here if operations are valid
		return nil, nil, ErrIncompatibleLengths
	}

	return aPrime, bPrime, nil
}

// opIterator provides iteration over operations.
type opIterator struct {
	ops []Operation
	idx int
}

func newOpIterator(ops []Operation) *opIterator {
	return &opIterator{ops: ops, idx: 0}
}

func (it *opIterator) next() Operation {
	if it.idx >= len(it.ops) {
		return nil
	}
	op := it.ops[it.idx]
	it.idx++
	return op
}
