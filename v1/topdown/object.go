// Copyright 2020 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package topdown

import (
	"github.com/IUAD1IY7/opa/internal/ref"
	"github.com/IUAD1IY7/opa/v1/ast"
	"github.com/IUAD1IY7/opa/v1/topdown/builtins"
)

func builtinObjectUnion(_ BuiltinContext, operands []*ast.Term, iter func(*ast.Term) error) error {
	objA, err := builtins.ObjectOperand(operands[0].Value, 1)
	if err != nil {
		return err
	}

	objB, err := builtins.ObjectOperand(operands[1].Value, 2)
	if err != nil {
		return err
	}

	if objA.Len() == 0 {
		return iter(operands[1])
	}
	if objB.Len() == 0 {
		return iter(operands[0])
	}
	if objA.Compare(objB) == 0 {
		return iter(operands[0])
	}

	r := mergeWithOverwrite(objA, objB)

	return iter(ast.NewTerm(r))
}

func builtinObjectUnionN(_ BuiltinContext, operands []*ast.Term, iter func(*ast.Term) error) error {
	arr, err := builtins.ArrayOperand(operands[0].Value, 1)
	if err != nil {
		return err
	}

	// Because we need merge-with-overwrite behavior, we can iterate
	// back-to-front, and get a mostly correct set of key assignments that
	// give us the "last assignment wins, with merges" behavior we want.
	// However, if a non-object overwrites an object value anywhere in the
	// chain of assignments for a key, we have to "freeze" that key to
	// prevent accidentally picking up nested objects that could merge with
	// it from earlier in the input array.
	// Example:
	//   Input: [{"a": {"b": 2}}, {"a": 4}, {"a": {"c": 3}}]
	//   Want Output: {"a": {"c": 3}}
	result := ast.NewObject()
	frozenKeys := map[*ast.Term]struct{}{}
	for i := arr.Len() - 1; i >= 0; i-- {
		o, ok := arr.Elem(i).Value.(ast.Object)
		if !ok {
			return builtins.NewOperandElementErr(1, arr, arr.Elem(i).Value, "object")
		}
		mergewithOverwriteInPlace(result, o, frozenKeys)
	}

	return iter(ast.NewTerm(result))
}

func builtinObjectRemove(_ BuiltinContext, operands []*ast.Term, iter func(*ast.Term) error) error {
	// Expect an object and an array/set/object of keys
	obj, err := builtins.ObjectOperand(operands[0].Value, 1)
	if err != nil {
		return err
	}

	// Build a set of keys to remove
	keysToRemove, err := getObjectKeysParam(operands[1].Value)
	if err != nil {
		return err
	}
	r := ast.NewObject()
	obj.Foreach(func(key *ast.Term, value *ast.Term) {
		if !keysToRemove.Contains(key) {
			r.Insert(key, value)
		}
	})

	return iter(ast.NewTerm(r))
}

func builtinObjectFilter(_ BuiltinContext, operands []*ast.Term, iter func(*ast.Term) error) error {
	// Expect an object and an array/set/object of keys
	obj, err := builtins.ObjectOperand(operands[0].Value, 1)
	if err != nil {
		return err
	}

	// Build a new object from the supplied filter keys
	keys, err := getObjectKeysParam(operands[1].Value)
	if err != nil {
		return err
	}

	filterObj := ast.NewObject()
	keys.Foreach(func(key *ast.Term) {
		filterObj.Insert(key, ast.InternedNullTerm)
	})

	// Actually do the filtering
	r, err := obj.Filter(filterObj)
	if err != nil {
		return err
	}

	return iter(ast.NewTerm(r))
}

func builtinObjectGet(_ BuiltinContext, operands []*ast.Term, iter func(*ast.Term) error) error {
	object, err := builtins.ObjectOperand(operands[0].Value, 1)
	if err != nil {
		return err
	}

	// if the get key is not an array, attempt to get the top level key for the operand value in the object
	path, ok := operands[1].Value.(*ast.Array)
	if !ok {
		if ret := object.Get(operands[1]); ret != nil {
			return iter(ret)
		}

		return iter(operands[2])
	}

	// if the path is empty, then we skip selecting nested keys and return the whole object
	if path.Len() == 0 {
		return iter(operands[0])
	}

	// build an ast.Ref from the array and see if it matches within the object
	pathRef := ref.ArrayPath(path)
	value, err := object.Find(pathRef)
	if err != nil {
		return iter(operands[2])
	}

	return iter(ast.NewTerm(value))
}

func builtinObjectKeys(_ BuiltinContext, operands []*ast.Term, iter func(*ast.Term) error) error {
	object, err := builtins.ObjectOperand(operands[0].Value, 1)
	if err != nil {
		return err
	}
	if object.Len() == 0 {
		return iter(ast.InternedEmptySet)
	}

	return iter(ast.SetTerm(object.Keys()...))
}

// getObjectKeysParam returns a set of key values
// from a supplied ast array, object, set value
func getObjectKeysParam(arrayOrSet ast.Value) (ast.Set, error) {
	switch v := arrayOrSet.(type) {
	case *ast.Array:
		keys := ast.NewSet()
		v.Foreach(keys.Add)
		return keys, nil
	case ast.Set:
		return ast.NewSet(v.Slice()...), nil
	case ast.Object:
		return ast.NewSet(v.Keys()...), nil
	}

	return nil, builtins.NewOperandTypeErr(2, arrayOrSet, "object", "set", "array")
}

func mergeWithOverwrite(objA, objB ast.Object) ast.Object {
	merged, _ := objA.MergeWith(objB, func(v1, v2 *ast.Term) (*ast.Term, bool) {
		originalValueObj, ok2 := v1.Value.(ast.Object)
		updateValueObj, ok1 := v2.Value.(ast.Object)
		if !ok1 || !ok2 {
			// If we can't merge, stick with the right-hand value
			return v2, false
		}

		// Recursively update the existing value
		merged := mergeWithOverwrite(originalValueObj, updateValueObj)
		return ast.NewTerm(merged), false
	})
	return merged
}

// Modifies obj with any new keys from other, and recursively
// merges any keys where the values are both objects.
func mergewithOverwriteInPlace(obj, other ast.Object, frozenKeys map[*ast.Term]struct{}) {
	other.Foreach(func(k, v *ast.Term) {
		v2 := obj.Get(k)
		// The key didn't exist in other, keep the original value.
		if v2 == nil {
			nestedObj, ok := v.Value.(ast.Object)
			if !ok {
				// v is not an object
				obj.Insert(k, v)
			} else {
				// Copy the nested object so the original object would not be modified
				nestedObjCopy := nestedObj.Copy()
				obj.Insert(k, ast.NewTerm(nestedObjCopy))
			}

			return
		}
		// The key exists in both. Merge or reject change.
		updateValueObj, ok2 := v.Value.(ast.Object)
		originalValueObj, ok1 := v2.Value.(ast.Object)
		// Both are objects? Merge recursively.
		if ok1 && ok2 {
			// Check to make sure that this key isn't frozen before merging.
			if _, ok := frozenKeys[v2]; !ok {
				mergewithOverwriteInPlace(originalValueObj, updateValueObj, frozenKeys)
			}
		} else {
			// Else, original value wins. Freeze the key.
			frozenKeys[v2] = struct{}{}
		}
	})
}

func init() {
	RegisterBuiltinFunc(ast.ObjectUnion.Name, builtinObjectUnion)
	RegisterBuiltinFunc(ast.ObjectUnionN.Name, builtinObjectUnionN)
	RegisterBuiltinFunc(ast.ObjectRemove.Name, builtinObjectRemove)
	RegisterBuiltinFunc(ast.ObjectFilter.Name, builtinObjectFilter)
	RegisterBuiltinFunc(ast.ObjectGet.Name, builtinObjectGet)
	RegisterBuiltinFunc(ast.ObjectKeys.Name, builtinObjectKeys)
}
