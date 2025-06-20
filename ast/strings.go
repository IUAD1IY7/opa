// Copyright 2016 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package ast

import (
	v1 "github.com/IUAD1IY7/opa/v1/ast"
)

// TypeName returns a human readable name for the AST element type.
func TypeName(x any) string {
	return v1.TypeName(x)
}
