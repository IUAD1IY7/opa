// Copyright 2018 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package bundle

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"time"

	"github.com/IUAD1IY7/opa/v1/ast"
	"github.com/IUAD1IY7/opa/v1/download"
	"github.com/IUAD1IY7/opa/v1/metrics"
	"github.com/IUAD1IY7/opa/v1/server/types"
)

const (
	errCode = "bundle_error"
)

// Status represents the status of processing a bundle.
type Status struct {
	Name                     string          `json:"name"`
	ActiveRevision           string          `json:"active_revision,omitempty"`
	LastSuccessfulActivation time.Time       `json:"last_successful_activation,omitempty"`
	Type                     string          `json:"type,omitempty"`
	Size                     int             `json:"size,omitempty"`
	LastSuccessfulDownload   time.Time       `json:"last_successful_download,omitempty"`
	LastSuccessfulRequest    time.Time       `json:"last_successful_request,omitempty"`
	LastRequest              time.Time       `json:"last_request,omitempty"`
	Code                     string          `json:"code,omitempty"`
	Message                  string          `json:"message,omitempty"`
	Errors                   []error         `json:"errors,omitempty"`
	Metrics                  metrics.Metrics `json:"metrics,omitempty"`
	HTTPCode                 json.Number     `json:"http_code,omitempty"`
}

// SetActivateSuccess updates the status object to reflect a successful
// activation.
func (s *Status) SetActivateSuccess(revision string) {
	s.LastSuccessfulActivation = time.Now().UTC()
	s.ActiveRevision = revision
}

// SetDownloadSuccess updates the status object to reflect a successful
// download.
func (s *Status) SetDownloadSuccess() {
	s.LastSuccessfulDownload = time.Now().UTC()
}

// SetRequest updates the status object to reflect a download attempt.
func (s *Status) SetRequest() {
	s.LastRequest = time.Now().UTC()
}

func (s *Status) SetBundleSize(size int) {
	s.Size = size
}

// SetError updates the status object to reflect a failure to download or
// activate. If err is nil, the error status is cleared.
func (s *Status) SetError(err error) {
	var (
		astErrors ast.Errors
		httpError download.HTTPError
	)
	switch {
	case err == nil:
		s.Code = ""
		s.HTTPCode = ""
		s.Message = ""
		s.Errors = nil

	case errors.As(err, &astErrors):
		s.Code = errCode
		s.HTTPCode = ""
		s.Message = types.MsgCompileModuleError
		s.Errors = make([]error, len(astErrors))
		for i := range astErrors {
			s.Errors[i] = astErrors[i]
		}

	case errors.As(err, &httpError):
		s.Code = errCode
		s.HTTPCode = json.Number(strconv.Itoa(httpError.StatusCode))
		s.Message = err.Error()
		s.Errors = nil

	default:
		s.Code = errCode
		s.HTTPCode = ""
		s.Message = err.Error()
		s.Errors = nil
	}
}

func (s *Status) Equal(other *Status) bool {
	if s == nil || other == nil {
		return s == nil && other == nil
	}

	equal := s.Name == other.Name &&
		s.Type == other.Type &&
		s.Size == other.Size &&
		s.Code == other.Code &&
		s.Message == other.Message &&
		s.HTTPCode == other.HTTPCode &&
		s.ActiveRevision == other.ActiveRevision &&
		s.LastSuccessfulActivation.Equal(other.LastSuccessfulActivation) &&
		s.LastSuccessfulDownload.Equal(other.LastSuccessfulDownload) &&
		s.LastSuccessfulRequest.Equal(other.LastSuccessfulRequest) &&
		s.LastRequest.Equal(other.LastRequest)

	if !equal {
		return false
	}

	if len(s.Errors) != len(other.Errors) {
		return false
	}
	for i := range s.Errors {
		if s.Errors[i].Error() != other.Errors[i].Error() {
			return false
		}
	}

	if s.Metrics != nil && other.Metrics != nil && s.Metrics.All() != nil && other.Metrics.All() != nil {
		return reflect.DeepEqual(s.Metrics.All(), other.Metrics.All())
	}

	return s.Metrics == nil && other.Metrics == nil
}
