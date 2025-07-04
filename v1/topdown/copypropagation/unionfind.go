// Copyright 2020 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package copypropagation

import (
	"fmt"

	"github.com/IUAD1IY7/opa/v1/ast"
	"github.com/IUAD1IY7/opa/v1/util"
)

type rankFunc func(*unionFindRoot, *unionFindRoot) (*unionFindRoot, *unionFindRoot)

type unionFind struct {
	roots   *util.HasherMap[ast.Value, *unionFindRoot]
	parents *ast.ValueMap
	rank    rankFunc
}

func newUnionFind(rank rankFunc) *unionFind {
	return &unionFind{
		roots:   util.NewHasherMap[ast.Value, *unionFindRoot](ast.ValueEqual),
		parents: ast.NewValueMap(),
		rank:    rank,
	}
}

func (uf *unionFind) MakeSet(v ast.Value) *unionFindRoot {

	root, ok := uf.Find(v)
	if ok {
		return root
	}

	root = newUnionFindRoot(v)
	uf.parents.Put(v, v)
	uf.roots.Put(v, root)
	return root
}

func (uf *unionFind) Find(v ast.Value) (*unionFindRoot, bool) {

	parent := uf.parents.Get(v)
	if parent == nil {
		return nil, false
	}

	if parent.Compare(v) == 0 {
		r, ok := uf.roots.Get(v)
		return r, ok
	}

	return uf.Find(parent)
}

func (uf *unionFind) Merge(a, b ast.Value) (*unionFindRoot, bool) {

	r1 := uf.MakeSet(a)
	r2 := uf.MakeSet(b)

	if r1 != r2 {

		r1, r2 = uf.rank(r1, r2)

		uf.parents.Put(r2.key, r1.key)
		uf.roots.Delete(r2.key)

		// Sets can have at most one constant value associated with them. When
		// unioning, we must preserve this invariant. If a set has two constants,
		// there will be no way to prove the query.
		if r1.constant != nil && r2.constant != nil && !r1.constant.Equal(r2.constant) {
			return nil, false
		} else if r1.constant == nil {
			r1.constant = r2.constant
		}
	}

	return r1, true
}

func (uf *unionFind) String() string {
	o := struct {
		Roots   map[string]any
		Parents map[string]ast.Value
	}{
		map[string]any{},
		map[string]ast.Value{},
	}

	uf.roots.Iter(func(k ast.Value, v *unionFindRoot) bool {
		o.Roots[k.String()] = struct {
			Constant *ast.Term
			Key      ast.Value
		}{
			v.constant,
			v.key,
		}
		return true
	})

	uf.parents.Iter(func(k ast.Value, v ast.Value) bool {
		o.Parents[k.String()] = v
		return true
	})

	return string(util.MustMarshalJSON(o))
}

type unionFindRoot struct {
	key      ast.Value
	constant *ast.Term
}

func newUnionFindRoot(key ast.Value) *unionFindRoot {
	return &unionFindRoot{
		key: key,
	}
}

func (r *unionFindRoot) Value() ast.Value {
	if r.constant != nil {
		return r.constant.Value
	}
	return r.key
}

func (r *unionFindRoot) String() string {
	return fmt.Sprintf("{key: %s, constant: %s", r.key, r.constant)
}
