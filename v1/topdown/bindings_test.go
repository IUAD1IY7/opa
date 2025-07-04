// Copyright 2017 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package topdown

import (
	"strconv"
	"testing"

	"github.com/IUAD1IY7/opa/v1/ast"
)

func TestBindingsZeroValues(t *testing.T) {
	t.Parallel()

	var unifier *bindings

	// Plugging
	result := unifier.Plug(term("x"))
	exp := term("x")
	if !result.Equal(exp) {
		t.Fatalf("Expected %v but got %v", exp, result)
	}

	// String
	if unifier.String() != "()" {
		t.Fatalf("Expected empty binding list but got: %v", unifier.String())
	}
}

func term(s string) *ast.Term {
	return ast.MustParseTerm(s)
}

func TestBindingsArrayHashmap(t *testing.T) {
	t.Parallel()

	var bindings bindings
	b := newBindingsArrayHashmap()
	keys := make(map[int]ast.Var)

	for i := range maxLinearScan + 1 {
		b.Put(testBindingKey(i), testBindingValue(&bindings, i))
		keys[i] = testBindingKey(i).Value.(ast.Var)

		testBindingKeys(t, &bindings, &b, keys)
	}

	for i := range maxLinearScan + 1 {
		b.Delete(testBindingKey(i))
		delete(keys, i)

		testBindingKeys(t, &bindings, &b, keys)
	}
}

func testBindingKeys(t *testing.T, bindings *bindings, b *bindingsArrayHashmap, keys map[int]ast.Var) {
	t.Helper()

	for k := range keys {
		value := testBindingValue(bindings, k)
		if v, ok := b.Get(testBindingKey(k)); !ok {
			t.Errorf("value not found: %v", k)
		} else if !v.equal(&value) {
			t.Errorf("value not equal")
		}
	}

	var found []ast.Var
	b.Iter(func(k *ast.Term, v value) bool {
		key := k.Value.(ast.Var)
		if i, _ := strconv.Atoi(string(key)); !testBindingValue(bindings, i).equal(&v) {
			t.Errorf("iteration value note equal")
		}

		found = append(found, key)
		return false
	})

	if len(found) != len(keys) {
		t.Errorf("all keys not found")
	}

next:
	for _, a := range keys {
		for _, b := range found {
			if a == b {
				continue next
			}
		}

		t.Errorf("key not found")
	}
}

func testBindingKey(key int) *ast.Term {
	return ast.VarTerm(strconv.Itoa(key))
}

func testBindingValue(b *bindings, key int) value {
	return value{b, ast.IntNumberTerm(key)}
}
