package ot

import (
	"encoding/json"
	"errors"
	"fmt"
)

// JSON serialization format (matching Rust operational-transform):
//   - Retain(n) → positive integer n
//   - Delete(n) → negative integer -n
//   - Insert(s) → string "s"
//
// Example: [5, "hello", -3, 10]
//   = Retain(5), Insert("hello"), Delete(3), Retain(10)

// MarshalJSON implements json.Marshaler for OperationSeq.
func (o *OperationSeq) MarshalJSON() ([]byte, error) {
	if o == nil {
		return json.Marshal([]interface{}{})
	}

	result := make([]interface{}, len(o.ops))
	for i, op := range o.ops {
		switch v := op.(type) {
		case Retain:
			result[i] = v.N
		case Delete:
			result[i] = -int64(v.N)
		case Insert:
			result[i] = v.Text
		}
	}
	return json.Marshal(result)
}

// UnmarshalJSON implements json.Unmarshaler for OperationSeq.
func (o *OperationSeq) UnmarshalJSON(data []byte) error {
	var raw []interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*o = OperationSeq{
		ops:       make([]Operation, 0, len(raw)),
		baseLen:   0,
		targetLen: 0,
	}

	for _, item := range raw {
		switch v := item.(type) {
		case string:
			// String → Insert
			o.Insert(v)
		case float64:
			// JSON numbers are float64
			if v >= 0 {
				// Positive → Retain
				o.Retain(uint64(v))
			} else {
				// Negative → Delete
				o.Delete(uint64(-v))
			}
		default:
			return fmt.Errorf("invalid operation type: %T", item)
		}
	}

	return nil
}

// String returns a JSON representation of the operation sequence.
func (o *OperationSeq) String() string {
	data, err := json.Marshal(o)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return string(data)
}

// FromJSON parses an operation sequence from JSON.
func FromJSON(data string) (*OperationSeq, error) {
	var op OperationSeq
	if err := json.Unmarshal([]byte(data), &op); err != nil {
		return nil, err
	}
	return &op, nil
}

// MarshalJSON for Operation (for completeness, though not typically used directly)
func MarshalOperation(op Operation) ([]byte, error) {
	switch v := op.(type) {
	case Retain:
		return json.Marshal(v.N)
	case Delete:
		return json.Marshal(-int64(v.N))
	case Insert:
		return json.Marshal(v.Text)
	default:
		return nil, errors.New("unknown operation type")
	}
}
