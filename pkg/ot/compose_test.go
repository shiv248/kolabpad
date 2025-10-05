package ot

import (
	"testing"
)

func TestCompose(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		opsA    func() *OperationSeq
		opsB    func(*OperationSeq) *OperationSeq
		expectS string
	}{
		{
			name: "two inserts",
			s:    "",
			opsA: func() *OperationSeq {
				o := NewOperationSeq()
				o.Insert("abc")
				return o
			},
			opsB: func(after *OperationSeq) *OperationSeq {
				o := NewOperationSeq()
				o.Retain(3)
				o.Insert("def")
				return o
			},
			expectS: "abcdef",
		},
		{
			name: "delete after insert",
			s:    "",
			opsA: func() *OperationSeq {
				o := NewOperationSeq()
				o.Insert("hello world")
				return o
			},
			opsB: func(after *OperationSeq) *OperationSeq {
				o := NewOperationSeq()
				o.Delete(6) // delete "hello "
				o.Retain(5) // keep "world"
				return o
			},
			expectS: "world",
		},
		{
			name: "retain and modify",
			s:    "abc",
			opsA: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(3)
				o.Insert("def")
				return o
			},
			opsB: func(after *OperationSeq) *OperationSeq {
				o := NewOperationSeq()
				o.Delete(3) // delete "abc"
				o.Retain(3) // keep "def"
				return o
			},
			expectS: "def",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.opsA()
			afterA, err := a.Apply(tt.s)
			if err != nil {
				t.Fatalf("Apply A failed: %v", err)
			}

			b := tt.opsB(a)
			afterB, err := b.Apply(afterA)
			if err != nil {
				t.Fatalf("Apply B failed: %v", err)
			}

			// Compose A and B
			ab, err := a.Compose(b)
			if err != nil {
				t.Fatalf("Compose failed: %v", err)
			}

			// Apply composed operation
			afterAB, err := ab.Apply(tt.s)
			if err != nil {
				t.Fatalf("Apply AB failed: %v", err)
			}

			// Results should match
			if afterAB != afterB {
				t.Errorf("expected %q, got %q", afterB, afterAB)
			}

			if afterAB != tt.expectS {
				t.Errorf("expected final result %q, got %q", tt.expectS, afterAB)
			}
		})
	}
}

func TestComposeProperty(t *testing.T) {
	// Property: apply(apply(S, A), B) = apply(S, compose(A, B))
	tests := []struct {
		s string
		a func() *OperationSeq
		b func(string) *OperationSeq
	}{
		{
			s: "hello",
			a: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(5)
				o.Insert(" world")
				return o
			},
			b: func(s string) *OperationSeq {
				o := NewOperationSeq()
				o.Retain(6)
				o.Insert("beautiful ")
				o.Retain(5)
				return o
			},
		},
		{
			s: "abcdef",
			a: func() *OperationSeq {
				o := NewOperationSeq()
				o.Delete(3)
				o.Retain(3)
				return o
			},
			b: func(s string) *OperationSeq {
				o := NewOperationSeq()
				o.Retain(3)
				o.Insert("xyz")
				return o
			},
		},
	}

	for i, tt := range tests {
		a := tt.a()
		afterA, err := a.Apply(tt.s)
		if err != nil {
			t.Fatalf("test %d: Apply A failed: %v", i, err)
		}

		b := tt.b(afterA)
		afterB, err := b.Apply(afterA)
		if err != nil {
			t.Fatalf("test %d: Apply B failed: %v", i, err)
		}

		ab, err := a.Compose(b)
		if err != nil {
			t.Fatalf("test %d: Compose failed: %v", i, err)
		}

		afterAB, err := ab.Apply(tt.s)
		if err != nil {
			t.Fatalf("test %d: Apply AB failed: %v", i, err)
		}

		if afterAB != afterB {
			t.Errorf("test %d: compose property failed: %q != %q", i, afterAB, afterB)
		}
	}
}
