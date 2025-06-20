// Copyright 2019 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package topdown

import (
	"fmt"
	"testing"

	"github.com/IUAD1IY7/opa/v1/ast"
)

func BenchmarkVirtualCache(b *testing.B) {

	n := 10
	max := n * n * n

	keys := make([]ast.Ref, 0, max)
	values := make([]*ast.Term, 0, max)

	for i := range n {
		k1 := ast.StringTerm(fmt.Sprintf("aaaa%v", i))
		for j := range n {
			k2 := ast.StringTerm(fmt.Sprintf("bbbb%v", j))
			for k := range n {
				k3 := ast.StringTerm(fmt.Sprintf("cccc%v", k))
				key := ast.Ref{ast.DefaultRootDocument, k1, k2, k3}
				value := ast.ArrayTerm(k1, k2, k3)
				keys = append(keys, key)
				values = append(values, value)
			}
		}
	}

	cache := NewVirtualCache()
	b.ResetTimer()

	for i := range b.N {
		idx := i % max
		cache.Put(keys[idx], values[idx])
		result, _ := cache.Get(keys[idx])
		if !result.Equal(values[idx]) {
			b.Fatal("expected equal")
		}
	}

}
