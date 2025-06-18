// Copyright 2020 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package types

import (
	v1 "github.com/IUAD1IY7/opa/v1/types"
)

// Unmarshal deserializes bs and returns the resulting type.
func Unmarshal(bs []byte) (result Type, err error) {
	return v1.Unmarshal(bs)
}
