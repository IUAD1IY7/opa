// Copyright 2023 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package pathwatcher

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/IUAD1IY7/opa/v1/util/test"
)

func TestWatchPaths(t *testing.T) {

	fs := map[string]string{
		"/foo/bar/baz.json": "true",
		"/foo/faz/baz.json": "true",
		"/foo/baz.json":     "true",
	}

	expected := []string{
		"/foo", "/foo/bar", "/foo/faz",
	}

	test.WithTempFS(fs, func(rootDir string) {
		paths, err := getWatchPaths([]string{"prefix:" + rootDir + "/foo"})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		result := []string{}
		for _, p := range paths {
			result = append(result, filepath.Clean(strings.TrimPrefix(p, rootDir)))
		}
		if !slices.Equal(expected, result) {
			t.Fatalf("Expected %q but got: %q", expected, result)
		}
	})
}
