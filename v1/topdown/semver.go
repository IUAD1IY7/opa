// Copyright 2020 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package topdown

import (
	"fmt"

	"github.com/IUAD1IY7/opa/internal/semver"
	"github.com/IUAD1IY7/opa/v1/ast"
	"github.com/IUAD1IY7/opa/v1/topdown/builtins"
)

func builtinSemVerCompare(_ BuiltinContext, operands []*ast.Term, iter func(*ast.Term) error) error {
	versionStringA, err := builtins.StringOperand(operands[0].Value, 1)
	if err != nil {
		return err
	}

	versionStringB, err := builtins.StringOperand(operands[1].Value, 2)
	if err != nil {
		return err
	}

	versionA, err := semver.NewVersion(string(versionStringA))
	if err != nil {
		return fmt.Errorf("operand 1: string %s is not a valid SemVer", versionStringA)
	}
	versionB, err := semver.NewVersion(string(versionStringB))
	if err != nil {
		return fmt.Errorf("operand 2: string %s is not a valid SemVer", versionStringB)
	}

	result := versionA.Compare(*versionB)

	return iter(ast.InternedIntNumberTerm(result))
}

func builtinSemVerIsValid(_ BuiltinContext, operands []*ast.Term, iter func(*ast.Term) error) error {
	versionString, err := builtins.StringOperand(operands[0].Value, 1)
	if err != nil {
		return iter(ast.InternedBooleanTerm(false))
	}

	result := true

	_, err = semver.NewVersion(string(versionString))
	if err != nil {
		result = false
	}

	return iter(ast.InternedBooleanTerm(result))
}

func init() {
	RegisterBuiltinFunc(ast.SemVerCompare.Name, builtinSemVerCompare)
	RegisterBuiltinFunc(ast.SemVerIsValid.Name, builtinSemVerIsValid)
}
