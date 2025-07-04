// Copyright 2022 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package test

import (
	"github.com/IUAD1IY7/opa/v1/storage/inmem"
	"github.com/IUAD1IY7/opa/v1/storage"
)

// New returns an inmem store with some common options set: opt-out of write
// roundtripping.
func New() storage.Store {
	return inmem.NewWithOpts(inmem.OptRoundTripOnWrite(false))
}

// NewFromObject returns an inmem store from the passed object, with some
// common options set: opt-out of write roundtripping.
func NewFromObject(x map[string]any) storage.Store {
	return inmem.NewFromObjectWithOpts(x, inmem.OptRoundTripOnWrite(false))
}

// NewFromObjectWithASTRead returns an inmem store from the passed object, with
// round-trip on write disabled and AST values returned on read.
func NewFromObjectWithASTRead(x map[string]any) storage.Store {
	return inmem.NewFromObjectWithOpts(x, inmem.OptRoundTripOnWrite(false), inmem.OptReturnASTValuesOnRead(true))
}
