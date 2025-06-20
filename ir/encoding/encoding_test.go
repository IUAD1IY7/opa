package encoding

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/IUAD1IY7/opa/ast"
	"github.com/IUAD1IY7/opa/internal/planner"
	"github.com/IUAD1IY7/opa/ir"
)

func TestRoundTrip(t *testing.T) {

	// Note: v0 module
	c, err := ast.CompileModules(map[string]string{
		"test.rego": `
			package test

			p {
				input.foo == 7
			}
		`,
	})

	if err != nil {
		t.Fatal(err)
	}

	modules := []*ast.Module{}

	for _, m := range c.Modules {
		modules = append(modules, m)
	}

	planner := planner.New().
		WithQueries([]planner.QuerySet{
			{
				Name: "main",
				Queries: []ast.Body{
					ast.MustParseBody("data.test.p = true"),
				},
			},
		}).
		WithModules(modules).
		WithBuiltinDecls(ast.BuiltinMap)

	plan, err := planner.Plan()
	if err != nil {
		t.Fatal(err)
	}

	bs, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	var cpy ir.Policy
	err = json.Unmarshal(bs, &cpy)
	if err != nil {
		t.Fatal(err)
	}

	bs2, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(bs, bs2) {
		t.Fatal("expected bytes to be equal")
	}
}
