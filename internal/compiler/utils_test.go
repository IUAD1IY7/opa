// Copyright 2023 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package compiler

import (
	"fmt"
	"strings"
	"testing"

	"github.com/IUAD1IY7/opa/v1/ast"
)

func TestVerifyAuthorizationPolicySchema(t *testing.T) {

	module1 := `
	package policy

	default allow := false

	allow if {
	   input.identity = "foo"
	}

	allow if {
	   input.client_certificates[0] = {"foo": "bar"}
	}

	allow if {
	   input.method = "GET"
	}

	allow if {
	   input.path = ["foo", "bar"]
	}

    allow if {
	   "foo" in input.path
	}

	allow if {
	   input.params = {"foo": "bar"}
	}

	allow if {
	   input.headers = {"foo": "bar"}
	}

	allow if {
	   input.body.input.stock = "ACME"
	}`

	module2 := `
	package policy

	default allow := false

	allow if {
	   input.identty = "foo"
	}

	allow if {
	   input.path = "foo"
	}`

	module3 := `
	package policy

	default allow := false

	allow if {
	   input.path = [1, 2, 3]
	}`

	module4 := `
    package policy

    default allow := false

    allow if {
       input.client_certificates[0] = "foo"
    }`

	tests := []struct {
		note    string
		modules []string
		wantErr bool
		errs    []string
	}{
		{note: "no rules", modules: []string{}},
		{note: "no error", modules: []string{module1}},
		{note: "multiple errors", modules: []string{module2}, wantErr: true, errs: []string{"match error", "undefined ref: input.identty"}},
		{note: "wrong item type path", modules: []string{module3}, wantErr: true, errs: []string{"match error"}},
		{note: "wrong item type certs", modules: []string{module4}, wantErr: true, errs: []string{"match error"}},
	}

	for _, tc := range tests {
		t.Run(tc.note, func(t *testing.T) {
			modules := map[string]*ast.Module{}

			for i, module := range tc.modules {
				mod, err := ast.ParseModuleWithOpts(fmt.Sprintf("test%d.rego", i+1), module,
					ast.ParserOptions{AllFutureKeywords: true})
				if err != nil {
					t.Fatal(err)
				}

				modules[fmt.Sprintf("test%d.rego", i+1)] = mod
			}

			c := ast.NewCompiler()
			c.Compile(modules)
			if c.Failed() {
				t.Fatal("unexpected error:", c.Errors)
			}

			err := VerifyAuthorizationPolicySchema(c, ast.MustParseRef("data.policy.allow"))

			if tc.wantErr {
				if err == nil {
					t.Fatal("Expected error but got nil")
				}

				for _, e := range tc.errs {
					if !strings.Contains(err.Error(), e) {
						t.Errorf("Expected error %v not found", e)
					}
				}
			} else if err != nil {
				t.Fatalf("Unexpected error %v", err)
			}
		})
	}
}
