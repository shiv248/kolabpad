package ot

import (
	"encoding/json"
	"testing"
)

// Tests ported from Rust operational-transform:
// https://github.com/spebern/operational-transform-rs/blob/master/operational-transform/src/lib.rs#L558-L741

func TestLengths(t *testing.T) {
	o := NewOperationSeq()
	if o.baseLen != 0 || o.targetLen != 0 {
		t.Errorf("expected baseLen=0, targetLen=0, got %d, %d", o.baseLen, o.targetLen)
	}

	o.Retain(5)
	if o.baseLen != 5 || o.targetLen != 5 {
		t.Errorf("after Retain(5): expected baseLen=5, targetLen=5, got %d, %d", o.baseLen, o.targetLen)
	}

	o.Insert("abc")
	if o.baseLen != 5 || o.targetLen != 8 {
		t.Errorf("after Insert(abc): expected baseLen=5, targetLen=8, got %d, %d", o.baseLen, o.targetLen)
	}

	o.Retain(2)
	if o.baseLen != 7 || o.targetLen != 10 {
		t.Errorf("after Retain(2): expected baseLen=7, targetLen=10, got %d, %d", o.baseLen, o.targetLen)
	}

	o.Delete(2)
	if o.baseLen != 9 || o.targetLen != 10 {
		t.Errorf("after Delete(2): expected baseLen=9, targetLen=10, got %d, %d", o.baseLen, o.targetLen)
	}
}

func TestSequence(t *testing.T) {
	o := NewOperationSeq()
	o.Retain(5)
	o.Retain(0) // Should be ignored
	o.Insert("lorem")
	o.Insert("") // Should be ignored
	o.Delete(3)
	o.Delete(0) // Should be ignored

	if len(o.ops) != 3 {
		t.Errorf("expected 3 ops, got %d", len(o.ops))
	}
}

func TestEmptyOps(t *testing.T) {
	o := NewOperationSeq()
	o.Retain(0)
	o.Insert("")
	o.Delete(0)

	if len(o.ops) != 0 {
		t.Errorf("expected 0 ops, got %d", len(o.ops))
	}
}

func TestEq(t *testing.T) {
	o1 := NewOperationSeq()
	o1.Delete(1)
	o1.Insert("lo")
	o1.Retain(2)
	o1.Retain(3)

	o2 := NewOperationSeq()
	o2.Delete(1)
	o2.Insert("l")
	o2.Insert("o")
	o2.Retain(5)

	// They should be equal (operations get merged)
	if len(o1.ops) != len(o2.ops) {
		t.Errorf("expected same length, got %d vs %d", len(o1.ops), len(o2.ops))
	}
}

func TestOpsMerging(t *testing.T) {
	o := NewOperationSeq()
	if len(o.ops) != 0 {
		t.Errorf("expected 0 ops, got %d", len(o.ops))
	}

	o.Retain(2)
	if len(o.ops) != 1 {
		t.Errorf("expected 1 op, got %d", len(o.ops))
	}
	if ret, ok := o.ops[0].(Retain); !ok || ret.N != 2 {
		t.Errorf("expected Retain(2), got %v", o.ops[0])
	}

	o.Retain(3)
	if len(o.ops) != 1 {
		t.Errorf("expected 1 op (merged), got %d", len(o.ops))
	}
	if ret, ok := o.ops[0].(Retain); !ok || ret.N != 5 {
		t.Errorf("expected Retain(5), got %v", o.ops[0])
	}

	o.Insert("abc")
	if len(o.ops) != 2 {
		t.Errorf("expected 2 ops, got %d", len(o.ops))
	}
	if ins, ok := o.ops[1].(Insert); !ok || ins.Text != "abc" {
		t.Errorf("expected Insert(abc), got %v", o.ops[1])
	}

	o.Insert("xyz")
	if len(o.ops) != 2 {
		t.Errorf("expected 2 ops (merged), got %d", len(o.ops))
	}
	if ins, ok := o.ops[1].(Insert); !ok || ins.Text != "abcxyz" {
		t.Errorf("expected Insert(abcxyz), got %v", o.ops[1])
	}

	o.Delete(1)
	if len(o.ops) != 3 {
		t.Errorf("expected 3 ops, got %d", len(o.ops))
	}
	if del, ok := o.ops[2].(Delete); !ok || del.N != 1 {
		t.Errorf("expected Delete(1), got %v", o.ops[2])
	}

	o.Delete(1)
	if len(o.ops) != 3 {
		t.Errorf("expected 3 ops (merged), got %d", len(o.ops))
	}
	if del, ok := o.ops[2].(Delete); !ok || del.N != 2 {
		t.Errorf("expected Delete(2), got %v", o.ops[2])
	}
}

func TestIsNoop(t *testing.T) {
	o := NewOperationSeq()
	if !o.IsNoop() {
		t.Error("expected noop")
	}

	o.Retain(5)
	if !o.IsNoop() {
		t.Error("expected noop after Retain")
	}

	o.Retain(3)
	if !o.IsNoop() {
		t.Error("expected noop after multiple Retains")
	}

	o.Insert("lorem")
	if o.IsNoop() {
		t.Error("expected not noop after Insert")
	}
}

func TestApply(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		ops    func() *OperationSeq
		expect string
	}{
		{
			name: "simple insert",
			s:    "",
			ops: func() *OperationSeq {
				o := NewOperationSeq()
				o.Insert("hello")
				return o
			},
			expect: "hello",
		},
		{
			name: "retain and insert",
			s:    "world",
			ops: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(5)
				o.Insert("!")
				return o
			},
			expect: "world!",
		},
		{
			name: "delete",
			s:    "hello world",
			ops: func() *OperationSeq {
				o := NewOperationSeq()
				o.Delete(6)
				o.Retain(5)
				return o
			},
			expect: "world",
		},
		{
			name: "complex",
			s:    "hello",
			ops: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(2)
				o.Delete(1)
				o.Insert("n")
				o.Retain(2)
				return o
			},
			expect: "henlo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.ops().Apply(tt.s)
			if err != nil {
				t.Fatalf("Apply failed: %v", err)
			}
			if result != tt.expect {
				t.Errorf("expected %q, got %q", tt.expect, result)
			}
		})
	}
}

func TestInvert(t *testing.T) {
	tests := []struct {
		name string
		s    string
		ops  func() *OperationSeq
	}{
		{
			name: "simple insert",
			s:    "abc",
			ops: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(3)
				o.Insert("def")
				return o
			},
		},
		{
			name: "delete",
			s:    "abcdef",
			ops: func() *OperationSeq {
				o := NewOperationSeq()
				o.Delete(3)
				o.Retain(3)
				return o
			},
		},
		{
			name: "complex",
			s:    "hello world",
			ops: func() *OperationSeq {
				o := NewOperationSeq()
				o.Retain(5)
				o.Insert(" beautiful")
				o.Retain(6)
				return o
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := tt.ops()
			inverted := o.Invert(tt.s)

			// Apply operation then inverted operation
			after, err := o.Apply(tt.s)
			if err != nil {
				t.Fatalf("Apply failed: %v", err)
			}

			restored, err := inverted.Apply(after)
			if err != nil {
				t.Fatalf("Apply inverted failed: %v", err)
			}

			if restored != tt.s {
				t.Errorf("expected %q, got %q", tt.s, restored)
			}

			// Check lengths
			if o.baseLen != inverted.targetLen {
				t.Errorf("baseLen mismatch: %d != %d", o.baseLen, inverted.targetLen)
			}
			if o.targetLen != inverted.baseLen {
				t.Errorf("targetLen mismatch: %d != %d", o.targetLen, inverted.baseLen)
			}
		})
	}
}

func TestSerde(t *testing.T) {
	// Test simple case
	jsonStr := `[1,-1,"abc"]`
	var o OperationSeq
	if err := json.Unmarshal([]byte(jsonStr), &o); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	expected := NewOperationSeq()
	expected.Retain(1)
	expected.Delete(1)
	expected.Insert("abc")

	if len(o.ops) != len(expected.ops) {
		t.Errorf("expected %d ops, got %d", len(expected.ops), len(o.ops))
	}

	// Test round-trip
	data, err := json.Marshal(&o)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var o2 OperationSeq
	if err := json.Unmarshal(data, &o2); err != nil {
		t.Fatalf("Unmarshal round-trip failed: %v", err)
	}

	if len(o2.ops) != len(o.ops) {
		t.Errorf("round-trip: expected %d ops, got %d", len(o.ops), len(o2.ops))
	}
}
