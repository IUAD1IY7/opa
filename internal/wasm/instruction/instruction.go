// Copyright 2018 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

// Package instruction defines WASM instruction types.
package instruction

import (
	"github.com/IUAD1IY7/opa/internal/wasm/opcode"
	"github.com/IUAD1IY7/opa/internal/wasm/types"
)

// NoImmediateArgs indicates the instruction has no immediate arguments.
type NoImmediateArgs struct {
}

// ImmediateArgs returns the immedate arguments of an instruction.
func (NoImmediateArgs) ImmediateArgs() []any {
	return nil
}

// Instruction represents a single WASM instruction.
type Instruction interface {
	Op() opcode.Opcode
	ImmediateArgs() []any
}

// StructuredInstruction represents a structured control instruction like br_if.
type StructuredInstruction interface {
	Instruction
	BlockType() *types.ValueType
	Instructions() []Instruction
}
