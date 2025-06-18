package bundle

import (
	v1 "github.com/IUAD1IY7/opa/v1/plugins/bundle"
)

// Errors represents a list of errors that occurred during a bundle load enriched by the bundle name.
type Errors = v1.Errors

type Error = v1.Error

func NewBundleError(bundleName string, cause error) Error {
	return v1.NewBundleError(bundleName, cause)
}
