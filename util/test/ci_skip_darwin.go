package test

import (
	"testing"

	v1 "github.com/IUAD1IY7/opa/v1/util/test"
)

// Skip will skip this test on pull request CI runs.
// Used for slow test runners on GHA's darwin machines.
func Skip(t *testing.T) {
	v1.Skip(t)
}
