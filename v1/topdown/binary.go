// Copyright 2017 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package topdown

import (
	"github.com/IUAD1IY7/opa/v1/ast"
	"github.com/IUAD1IY7/opa/v1/topdown/builtins"
)

func builtinBinaryAnd(_ BuiltinContext, operands []*ast.Term, iter func(*ast.Term) error) error {

	s1, err := builtins.SetOperand(operands[0].Value, 1)
	if err != nil {
		return err
	}

	s2, err := builtins.SetOperand(operands[1].Value, 2)
	if err != nil {
		return err
	}

	i := s1.Intersect(s2)
	if i.Len() == 0 {
		return iter(ast.InternedEmptySet)
	}

	return iter(ast.NewTerm(i))
}

func builtinBinaryOr(_ BuiltinContext, operands []*ast.Term, iter func(*ast.Term) error) error {

	s1, err := builtins.SetOperand(operands[0].Value, 1)
	if err != nil {
		return err
	}

	s2, err := builtins.SetOperand(operands[1].Value, 2)
	if err != nil {
		return err
	}

	return iter(ast.NewTerm(s1.Union(s2)))
}

func init() {
	RegisterBuiltinFunc(ast.And.Name, builtinBinaryAnd)
	RegisterBuiltinFunc(ast.Or.Name, builtinBinaryOr)
}
