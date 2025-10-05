package ot

import (
	"testing"
)

func TestTransform(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		opsA    func() *OperationSeq
		opsB    func() *OperationSeq
		expectS string
	}{
		{
			name: "concurrent inserts at different positions",
			s:    "abc",
			opsA: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(3)
				o.Insert("def")
				return o
			},
			opsB: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(3)
				o.Insert("ghi")
				return o
			},
			expectS: "abcdefghi", // or "abcghidef" depending on tie-breaking
		},
		{
			name: "concurrent inserts at same position",
			s:    "abc",
			opsA: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(2)
				o.Insert("X")
				o.Retain(1)
				return o
			},
			opsB: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(2)
				o.Insert("Y")
				o.Retain(1)
				return o
			},
			expectS: "abXYc", // or "abYXc" depending on tie-breaking
		},
		{
			name: "insert vs delete",
			s:    "hello world",
			opsA: func() *OperationSeq {
				o := NewOperationSeq()
				o.Delete(6) // delete "hello "
				o.Retain(5)
				return o
			},
			opsB: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(5)
				o.Insert("!") // insert "!" after "hello"
				o.Retain(6)
				return o
			},
			expectS: "world!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.opsA()
			b := tt.opsB()

			// Transform A and B
			aPrime, bPrime, err := a.Transform(b)
			if err != nil {
				t.Fatalf("Transform failed: %v", err)
			}

			// Apply A then B'
			afterA, err := a.Apply(tt.s)
			if err != nil {
				t.Fatalf("Apply A failed: %v", err)
			}
			afterAB, err := bPrime.Apply(afterA)
			if err != nil {
				t.Fatalf("Apply B' failed: %v", err)
			}

			// Apply B then A'
			afterB, err := b.Apply(tt.s)
			if err != nil {
				t.Fatalf("Apply B failed: %v", err)
			}
			afterBA, err := aPrime.Apply(afterB)
			if err != nil {
				t.Fatalf("Apply A' failed: %v", err)
			}

			// Results should be identical (convergence property)
			if afterAB != afterBA {
				t.Errorf("transform property failed:\n  A+B' = %q\n  B+A' = %q", afterAB, afterBA)
			}
		})
	}
}

func TestTransformProperty(t *testing.T) {
	// TP1: apply(apply(S, A), B') = apply(apply(S, B), A')
	// where (A', B') = transform(A, B)

	tests := []struct {
		s string
		a func() *OperationSeq
		b func() *OperationSeq
	}{
		{
			s: "hello",
			a: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(5)
				o.Insert(" world")
				return o
			},
			b: func() *OperationSeq {
				o := NewOperationSeq()
				o.Insert("Hi! ")
				o.Retain(5)
				return o
			},
		},
		{
			s: "abcdefgh",
			a: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(3)
				o.Delete(2)
				o.Retain(3)
				return o
			},
			b: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(5)
				o.Delete(3)
				return o
			},
		},
		{
			s: "test",
			a: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(2)
				o.Insert("XX")
				o.Retain(2)
				return o
			},
			b: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(2)
				o.Insert("YY")
				o.Retain(2)
				return o
			},
		},
	}

	for i, tt := range tests {
		a := tt.a()
		b := tt.b()

		// Transform
		aPrime, bPrime, err := a.Transform(b)
		if err != nil {
			t.Fatalf("test %d: Transform failed: %v", i, err)
		}

		// Path 1: A then B'
		afterA, err := a.Apply(tt.s)
		if err != nil {
			t.Fatalf("test %d: Apply A failed: %v", i, err)
		}
		path1, err := bPrime.Apply(afterA)
		if err != nil {
			t.Fatalf("test %d: Apply B' failed: %v", i, err)
		}

		// Path 2: B then A'
		afterB, err := b.Apply(tt.s)
		if err != nil {
			t.Fatalf("test %d: Apply B failed: %v", i, err)
		}
		path2, err := aPrime.Apply(afterB)
		if err != nil {
			t.Fatalf("test %d: Apply A' failed: %v", i, err)
		}

		// Must converge
		if path1 != path2 {
			t.Errorf("test %d: TP1 failed:\n  S=%q\n  A+B'=%q\n  B+A'=%q", i, tt.s, path1, path2)
		}
	}
}

func TestTransformSymmetry(t *testing.T) {
	// Transform(A, B) and Transform(B, A) should produce symmetric results
	s := "hello"

	a := NewOperationSeq()
	a.Retain(5)
	a.Insert(" world")

	b := NewOperationSeq()
	b.Insert("Hi! ")
	b.Retain(5)

	aPrime1, bPrime1, err := a.Transform(b)
	if err != nil {
		t.Fatalf("Transform(A, B) failed: %v", err)
	}

	bPrime2, aPrime2, err := b.Transform(a)
	if err != nil {
		t.Fatalf("Transform(B, A) failed: %v", err)
	}

	// Verify symmetry
	afterAB1, _ := aPrime1.Apply(s)
	afterBA1, _ := bPrime1.Apply(afterAB1)

	afterBA2, _ := bPrime2.Apply(s)
	afterAB2, _ := aPrime2.Apply(afterBA2)

	if afterBA1 != afterAB2 {
		t.Errorf("transform symmetry failed:\n  Transform(A,B) result: %q\n  Transform(B,A) result: %q",
			afterBA1, afterAB2)
	}
}
