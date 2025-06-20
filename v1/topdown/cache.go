// Copyright 2017 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package topdown

import (
	"slices"

	"github.com/IUAD1IY7/opa/v1/ast"
	"github.com/IUAD1IY7/opa/v1/util"
)

// VirtualCache defines the interface for a cache that stores the results of
// evaluated virtual documents (rules).
// The cache is a stack of frames, where each frame is a mapping from references
// to values.
type VirtualCache interface {
	// Push pushes a new, empty frame of value mappings onto the stack.
	Push()

	// Pop pops the top frame of value mappings from the stack, removing all associated entries.
	Pop()

	// Get returns the value associated with the given reference. The second return value
	// indicates whether the reference has a recorded 'undefined' result.
	Get(ref ast.Ref) (*ast.Term, bool)

	// Put associates the given reference with the given value. If the value is nil, the reference
	// is marked as having an 'undefined' result.
	Put(ref ast.Ref, value *ast.Term)

	// Keys returns the set of keys that have been cached for the active frame.
	Keys() []ast.Ref
}

// BaseCache defines the interface for a cache that stores cached base documents, i.e. data.
type BaseCache interface {
	Get(ast.Ref) ast.Value
	Put(ast.Ref, ast.Value)
}

type virtualCache struct {
	stack []*virtualCacheElem
}

type virtualCacheElem struct {
	value     *ast.Term
	children  *util.HasherMap[*ast.Term, *virtualCacheElem]
	undefined bool
}

func NewVirtualCache() VirtualCache {
	cache := &virtualCache{}
	cache.Push()
	return cache
}

func (c *virtualCache) Push() {
	c.stack = append(c.stack, newVirtualCacheElem())
}

func (c *virtualCache) Pop() {
	c.stack = c.stack[:len(c.stack)-1]
}

// Returns the resolved value of the AST term and a flag indicating if the value
// should be interpretted as undefined:
//
//	nil, true indicates the ref is undefined
//	ast.Term, false indicates the ref is defined
//	nil, false indicates the ref has not been cached
//	ast.Term, true is impossible
func (c *virtualCache) Get(ref ast.Ref) (*ast.Term, bool) {
	node := c.stack[len(c.stack)-1]
	for i := range ref {
		x, ok := node.children.Get(ref[i])
		if !ok {
			return nil, false
		}
		node = x
	}
	if node.undefined {
		return nil, true
	}

	return node.value, false
}

// If value is a nil pointer, set the 'undefined' flag on the cache element to
// indicate that the Ref has resolved to undefined.
func (c *virtualCache) Put(ref ast.Ref, value *ast.Term) {
	node := c.stack[len(c.stack)-1]
	for i := range ref {
		x, ok := node.children.Get(ref[i])
		if ok {
			node = x
		} else {
			next := newVirtualCacheElem()
			node.children.Put(ref[i], next)
			node = next
		}
	}
	if value != nil {
		node.value = value
	} else {
		node.undefined = true
	}
}

func (c *virtualCache) Keys() []ast.Ref {
	node := c.stack[len(c.stack)-1]
	return keysRecursive(nil, node)
}

func keysRecursive(root ast.Ref, node *virtualCacheElem) []ast.Ref {
	var keys []ast.Ref
	node.children.Iter(func(k *ast.Term, v *virtualCacheElem) bool {
		ref := root.Append(k)
		if v.value != nil {
			keys = append(keys, ref)
		}
		if v.children.Len() > 0 {
			keys = append(keys, keysRecursive(ref, v)...)
		}
		return false
	})
	return keys
}

func newVirtualCacheElem() *virtualCacheElem {
	return &virtualCacheElem{children: newVirtualCacheHashMap()}
}

func newVirtualCacheHashMap() *util.HasherMap[*ast.Term, *virtualCacheElem] {
	return util.NewHasherMap[*ast.Term, *virtualCacheElem](ast.TermValueEqual)
}

// baseCache implements a trie structure to cache base documents read out of
// storage. Values inserted into the cache may contain other values that were
// previously inserted. In this case, the previous values are erased from the
// structure.
type baseCache struct {
	root *baseCacheElem
}

func newBaseCache() *baseCache {
	return &baseCache{
		root: newBaseCacheElem(),
	}
}

func (c *baseCache) Get(ref ast.Ref) ast.Value {
	node := c.root
	for i := range ref {
		node = node.children[ref[i].Value]
		if node == nil {
			return nil
		} else if node.value != nil {
			if len(ref) == 1 && ast.IsScalar(node.value) {
				// If the node is a scalar, return the value directly
				// and avoid an allocation when calling Find.
				return node.value
			}

			result, err := node.value.Find(ref[i+1:])
			if err != nil {
				return nil
			}
			return result
		}
	}
	return nil
}

func (c *baseCache) Put(ref ast.Ref, value ast.Value) {
	node := c.root
	for i := range ref {
		if child, ok := node.children[ref[i].Value]; ok {
			node = child
		} else {
			child := newBaseCacheElem()
			node.children[ref[i].Value] = child
			node = child
		}
	}
	node.set(value)
}

type baseCacheElem struct {
	value    ast.Value
	children map[ast.Value]*baseCacheElem
}

func newBaseCacheElem() *baseCacheElem {
	return &baseCacheElem{
		children: map[ast.Value]*baseCacheElem{},
	}
}

func (e *baseCacheElem) set(value ast.Value) {
	e.value = value
	e.children = map[ast.Value]*baseCacheElem{}
}

type refStack struct {
	sl []refStackElem
}

type refStackElem struct {
	refs []ast.Ref
}

func newRefStack() *refStack {
	return &refStack{}
}

func (s *refStack) Push(refs []ast.Ref) {
	s.sl = append(s.sl, refStackElem{refs: refs})
}

func (s *refStack) Pop() {
	if s == nil {
		return
	}
	s.sl = s.sl[:len(s.sl)-1]
}

func (s *refStack) Prefixed(ref ast.Ref) bool {
	if s != nil {
		for i := len(s.sl) - 1; i >= 0; i-- {
			if slices.ContainsFunc(s.sl[i].refs, ref.HasPrefix) {
				return true
			}
		}
	}
	return false
}

type comprehensionCache struct {
	stack []map[*ast.Term]*comprehensionCacheElem
}

type comprehensionCacheElem struct {
	value    *ast.Term
	children *util.HasherMap[*ast.Term, *comprehensionCacheElem]
}

func newComprehensionCache() *comprehensionCache {
	cache := &comprehensionCache{}
	cache.Push()
	return cache
}

func (c *comprehensionCache) Push() {
	c.stack = append(c.stack, map[*ast.Term]*comprehensionCacheElem{})
}

func (c *comprehensionCache) Pop() {
	c.stack = c.stack[:len(c.stack)-1]
}

func (c *comprehensionCache) Elem(t *ast.Term) (*comprehensionCacheElem, bool) {
	elem, ok := c.stack[len(c.stack)-1][t]
	return elem, ok
}

func (c *comprehensionCache) Set(t *ast.Term, elem *comprehensionCacheElem) {
	c.stack[len(c.stack)-1][t] = elem
}

func newComprehensionCacheElem() *comprehensionCacheElem {
	return &comprehensionCacheElem{children: newComprehensionCacheHashMap()}
}

func (c *comprehensionCacheElem) Get(key []*ast.Term) *ast.Term {
	node := c
	for i := range key {
		x, ok := node.children.Get(key[i])
		if !ok {
			return nil
		}
		node = x
	}
	return node.value
}

func (c *comprehensionCacheElem) Put(key []*ast.Term, value *ast.Term) {
	node := c
	for i := range key {
		x, ok := node.children.Get(key[i])
		if ok {
			node = x
		} else {
			next := newComprehensionCacheElem()
			node.children.Put(key[i], next)
			node = next
		}
	}
	node.value = value
}

func newComprehensionCacheHashMap() *util.HasherMap[*ast.Term, *comprehensionCacheElem] {
	return util.NewHasherMap[*ast.Term, *comprehensionCacheElem](ast.TermValueEqual)
}

type functionMocksStack struct {
	stack []*functionMocksElem
}

type functionMocksElem []frame

type frame map[string]*ast.Term

func newFunctionMocksStack() *functionMocksStack {
	stack := &functionMocksStack{}
	stack.Push()
	return stack
}

func newFunctionMocksElem() *functionMocksElem {
	return &functionMocksElem{}
}

func (s *functionMocksStack) Push() {
	s.stack = append(s.stack, newFunctionMocksElem())
}

func (s *functionMocksStack) Pop() {
	s.stack = s.stack[:len(s.stack)-1]
}

func (s *functionMocksStack) PopPairs() {
	current := s.stack[len(s.stack)-1]
	*current = (*current)[:len(*current)-1]
}

func (s *functionMocksStack) PutPairs(mocks [][2]*ast.Term) {
	el := frame{}
	for i := range mocks {
		el[mocks[i][0].Value.String()] = mocks[i][1]
	}
	s.Put(el)
}

func (s *functionMocksStack) Put(el frame) {
	current := s.stack[len(s.stack)-1]
	*current = append(*current, el)
}

func (s *functionMocksStack) Get(f ast.Ref) (*ast.Term, bool) {
	if s == nil {
		return nil, false
	}

	current := *s.stack[len(s.stack)-1]
	for i := len(current) - 1; i >= 0; i-- {
		if r, ok := current[i][f.String()]; ok {
			return r, true
		}
	}
	return nil, false
}
