//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/shiv/kolabpad/pkg/ot"
)

// wrapOpSeq creates a JavaScript-compatible wrapper around Go OpSeq
func wrapOpSeq(op *ot.OperationSeq) js.Value {
	obj := make(map[string]interface{})

	// delete(n) - delete n characters
	obj["delete"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			op.Delete(uint64(args[0].Int()))
		}
		return nil
	})

	// insert(s) - insert string
	obj["insert"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			op.Insert(args[0].String())
		}
		return nil
	})

	// retain(n) - retain n characters
	obj["retain"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			op.Retain(uint64(args[0].Int()))
		}
		return nil
	})

	// compose(other) - compose with another operation
	obj["compose"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			return nil
		}
		otherOp := unwrapOpSeq(args[0])
		if otherOp == nil {
			return nil
		}
		result, err := op.Compose(otherOp)
		if err != nil {
			return nil
		}
		return wrapOpSeq(result)
	})

	// transform(other) - returns OpSeqPair with .first() and .second()
	obj["transform"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			return nil
		}
		otherOp := unwrapOpSeq(args[0])
		if otherOp == nil {
			return nil
		}
		aPrime, bPrime, err := op.Transform(otherOp)
		if err != nil {
			return nil
		}

		// Return object with first() and second() methods
		pair := make(map[string]interface{})
		pair["first"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			return wrapOpSeq(aPrime)
		})
		pair["second"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			return wrapOpSeq(bPrime)
		})
		return js.ValueOf(pair)
	})

	// apply(s) - apply operation to string
	obj["apply"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			return nil
		}
		result, err := op.Apply(args[0].String())
		if err != nil {
			return nil
		}
		return result
	})

	// invert(s) - invert operation
	obj["invert"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			return nil
		}
		inverted := op.Invert(args[0].String())
		return wrapOpSeq(inverted)
	})

	// is_noop() - check if operation is no-op
	obj["is_noop"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return op.IsNoop()
	})

	// base_len() - get base length
	obj["base_len"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return op.BaseLen()
	})

	// target_len() - get target length
	obj["target_len"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return op.TargetLen()
	})

	// transform_index(position) - transform cursor position
	obj["transform_index"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			return 0
		}
		// Use the transformIndex function from rustpad.go
		position := uint32(args[0].Int())
		newPos := transformIndex(op, position)
		return newPos
	})

	// to_string() - serialize to JSON
	obj["to_string"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		data, err := json.Marshal(op)
		if err != nil {
			return "{}"
		}
		return string(data)
	})

	return js.ValueOf(obj)
}

// unwrapOpSeq extracts OpSeq from JavaScript wrapper
func unwrapOpSeq(jsVal js.Value) *ot.OperationSeq {
	// Get the __opseq internal property
	if jsVal.Type() == js.TypeObject {
		internal := jsVal.Get("__opseq")
		if internal.Type() != js.TypeUndefined {
			// This is a wrapped OpSeq, extract the Go pointer
			// For now, we'll use a different approach - deserialize from JSON
			toString := jsVal.Get("to_string")
			if toString.Type() == js.TypeFunction {
				jsonStr := toString.Invoke().String()
				op, _ := ot.FromJSON(jsonStr)
				return op
			}
		}
	}
	return nil
}

// transformIndex transforms a cursor position through an operation
func transformIndex(operation *ot.OperationSeq, position uint32) uint32 {
	index := int32(position)
	newIndex := index

	for _, op := range operation.Ops() {
		switch v := op.(type) {
		case ot.Retain:
			index -= int32(v.N)
		case ot.Insert:
			charCount := int32(len([]rune(v.Text)))
			newIndex += charCount
		case ot.Delete:
			if index >= int32(v.N) {
				newIndex -= int32(v.N)
			} else if index > 0 {
				newIndex -= index
			}
			index -= int32(v.N)
		}

		if index < 0 {
			break
		}
	}

	if newIndex < 0 {
		return 0
	}
	return uint32(newIndex)
}

func main() {
	// Create OpSeq constructor object
	opseqConstructor := make(map[string]interface{})

	// OpSeq.new() - create new OpSeq
	opseqConstructor["new"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return wrapOpSeq(ot.NewOperationSeq())
	})

	// OpSeq.from_str(json) - deserialize from JSON
	opseqConstructor["from_str"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			return nil
		}
		jsonStr := args[0].String()
		op, err := ot.FromJSON(jsonStr)
		if err != nil {
			return nil
		}
		return wrapOpSeq(op)
	})

	// OpSeq.with_capacity(n) - create with capacity
	opseqConstructor["with_capacity"] = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		capacity := 0
		if len(args) > 0 {
			capacity = args[0].Int()
		}
		return wrapOpSeq(ot.WithCapacity(capacity))
	})

	// Export OpSeq to global scope
	js.Global().Set("OpSeq", js.ValueOf(opseqConstructor))

	println("Go WASM OT module loaded successfully!")

	// Keep the Go program running
	<-make(chan bool)
}
