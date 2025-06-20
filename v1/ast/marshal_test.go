package ast

import (
	"encoding/json"
	"testing"

	astJSON "github.com/IUAD1IY7/opa/v1/ast/json"
	"github.com/IUAD1IY7/opa/v1/util"
)

func resetJSONOptions() {
	astJSON.SetOptions(astJSON.Defaults())
}

func TestGeneric_MarshalWithLocationJSONOptions(t *testing.T) {
	testCases := map[string]struct {
		Term         *Term
		Options      astJSON.Options
		ExpectedJSON string
	}{
		"base case, no location options set": {
			Term: func() *Term {
				v, _ := InterfaceToValue("example")
				return &Term{
					Value:    v,
					Location: NewLocation([]byte{}, "example.rego", 1, 2),
				}
			}(),
			ExpectedJSON: `{"type":"string","value":"example"}`,
		},
		"location included, location text excluded": {
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Term: true,
					},
					IncludeLocationText: false,
				},
			},
			Term: func() *Term {
				v, _ := InterfaceToValue("example")
				return &Term{
					Value:    v,
					Location: NewLocation([]byte{}, "example.rego", 1, 2),
				}
			}(),
			ExpectedJSON: `{"location":{"file":"example.rego","row":1,"col":2},"type":"string","value":"example"}`,
		},
		"location included, location text also included": {
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Term: true,
					},
					IncludeLocationText: true,
				},
			},
			Term: func() *Term {
				v, _ := InterfaceToValue("example")
				t := &Term{
					Value:    v,
					Location: NewLocation([]byte("things"), "example.rego", 1, 2),
				}
				return t
			}(),
			ExpectedJSON: `{"location":{"file":"example.rego","row":1,"col":2,"text":"dGhpbmdz"},"type":"string","value":"example"}`,
		},
		"location included, location text included, file excluded": {
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Term: true,
					},
					IncludeLocationText: true,
					ExcludeLocationFile: true,
				},
			},
			Term: func() *Term {
				v, _ := InterfaceToValue("example")
				t := &Term{
					Value:    v,
					Location: NewLocation([]byte("things"), "example.rego", 1, 2),
				}
				return t
			}(),
			ExpectedJSON: `{"location":{"row":1,"col":2,"text":"dGhpbmdz"},"type":"string","value":"example"}`,
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			astJSON.SetOptions(data.Options)
			t.Cleanup(resetJSONOptions)

			bs := util.MustMarshalJSON(data.Term)
			got := string(bs)
			exp := data.ExpectedJSON

			if got != exp {
				t.Fatalf("expected:\n%s got\n%s", exp, got)
			}
		})
	}
}

func TestTerm_MarshalJSON(t *testing.T) {
	testCases := map[string]struct {
		Term         *Term
		Options      astJSON.Options
		ExpectedJSON string
	}{
		"base case": {
			Term: func() *Term {
				v, _ := InterfaceToValue("example")
				return &Term{
					Value:    v,
					Location: NewLocation([]byte{}, "example.rego", 1, 2),
				}
			}(),
			ExpectedJSON: `{"type":"string","value":"example"}`,
		},
		"location excluded": {
			Term: func() *Term {
				v, _ := InterfaceToValue("example")
				return &Term{
					Value:    v,
					Location: NewLocation([]byte{}, "example.rego", 1, 2),
				}
			}(),
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Term: false,
					},
				},
			},
			ExpectedJSON: `{"type":"string","value":"example"}`,
		},
		"location included": {
			Term: func() *Term {
				v, _ := InterfaceToValue("example")
				return &Term{
					Value:    v,
					Location: NewLocation([]byte{}, "example.rego", 1, 2),
				}
			}(),
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Term: true,
					},
				},
			},
			ExpectedJSON: `{"location":{"file":"example.rego","row":1,"col":2},"type":"string","value":"example"}`,
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			astJSON.SetOptions(data.Options)
			t.Cleanup(resetJSONOptions)

			bs := util.MustMarshalJSON(data.Term)
			got := string(bs)
			exp := data.ExpectedJSON

			if got != exp {
				t.Fatalf("expected:\n%s got\n%s", exp, got)
			}
		})
	}
}

func TestTerm_UnmarshalJSON(t *testing.T) {
	testCases := map[string]struct {
		JSON         string
		ExpectedTerm *Term
	}{
		"base case": {
			JSON: `{"type":"string","value":"example"}`,
			ExpectedTerm: func() *Term {
				v, _ := InterfaceToValue("example")
				return &Term{
					Value: v,
				}
			}(),
		},
		"location case": {
			JSON: `{"location":{"file":"example.rego","row":1,"col":2},"type":"string","value":"example"}`,
			ExpectedTerm: func() *Term {
				v, _ := InterfaceToValue("example")
				return &Term{
					Value:    v,
					Location: NewLocation([]byte{}, "example.rego", 1, 2),
				}
			}(),
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			var term Term
			err := json.Unmarshal([]byte(data.JSON), &term)
			if err != nil {
				t.Fatal(err)
			}

			if !term.Equal(data.ExpectedTerm) {
				t.Fatalf("expected:\n%#v got\n%#v", data.ExpectedTerm, term)
			}
			if data.ExpectedTerm.Location != nil {
				if !term.Location.Equal(data.ExpectedTerm.Location) {
					t.Fatalf("expected location:\n%#v got\n%#v", data.ExpectedTerm, term)
				}
			}
		})
	}
}

func TestPackage_MarshalJSON(t *testing.T) {
	testCases := map[string]struct {
		Package      *Package
		Options      astJSON.Options
		ExpectedJSON string
	}{
		"base case": {
			Package: &Package{
				Path: EmptyRef(),
			},
			ExpectedJSON: `{"path":[]}`,
		},
		"location excluded": {
			Package: &Package{
				Path:     EmptyRef(),
				Location: NewLocation([]byte{}, "example.rego", 1, 2),
			},
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Package: false,
					},
				},
			},
			ExpectedJSON: `{"path":[]}`,
		},
		"location included": {
			Package: &Package{
				Path:     EmptyRef(),
				Location: NewLocation([]byte{}, "example.rego", 1, 2),
			},
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Package: true,
					},
				},
			},
			ExpectedJSON: `{"location":{"file":"example.rego","row":1,"col":2},"path":[]}`,
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			astJSON.SetOptions(data.Options)
			t.Cleanup(resetJSONOptions)

			bs := util.MustMarshalJSON(data.Package)
			got := string(bs)
			exp := data.ExpectedJSON

			if got != exp {
				t.Fatalf("expected:\n%s got\n%s", exp, got)
			}
		})
	}
}

// TODO: Comment has inconsistent JSON field names starting with an upper case letter. Comment Location is
// also always included for legacy reasons
func TestComment_MarshalJSON(t *testing.T) {
	testCases := map[string]struct {
		Comment      *Comment
		Options      astJSON.Options
		ExpectedJSON string
	}{
		"base case": {
			Comment: &Comment{
				Text: []byte("comment"),
			},
			ExpectedJSON: `{"Text":"Y29tbWVudA==","Location":null}`,
		},
		"location excluded, still included for legacy reasons": {
			Comment: &Comment{
				Text:     []byte("comment"),
				Location: NewLocation([]byte{}, "example.rego", 1, 2),
			},
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Comment: false, // ignored
					},
				},
			},
			ExpectedJSON: `{"Text":"Y29tbWVudA==","Location":{"file":"example.rego","row":1,"col":2}}`,
		},
		"location included": {
			Comment: &Comment{
				Text:     []byte("comment"),
				Location: NewLocation([]byte{}, "example.rego", 1, 2),
			},
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Comment: true, // ignored
					},
				},
			},
			ExpectedJSON: `{"Text":"Y29tbWVudA==","Location":{"file":"example.rego","row":1,"col":2}}`,
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			astJSON.SetOptions(data.Options)
			t.Cleanup(resetJSONOptions)

			bs := util.MustMarshalJSON(data.Comment)
			got := string(bs)
			exp := data.ExpectedJSON

			if got != exp {
				t.Fatalf("expected:\n%s got\n%s", exp, got)
			}
		})
	}
}

func TestImport_MarshalJSON(t *testing.T) {
	testCases := map[string]struct {
		Import       *Import
		Options      astJSON.Options
		ExpectedJSON string
	}{
		"base case": {
			Import: func() *Import {
				v, _ := InterfaceToValue("example")
				term := Term{
					Value:    v,
					Location: NewLocation([]byte{}, "example.rego", 1, 2),
				}
				return &Import{Path: &term}
			}(),
			ExpectedJSON: `{"path":{"type":"string","value":"example"}}`,
		},
		"location excluded": {
			Import: func() *Import {
				v, _ := InterfaceToValue("example")
				term := Term{
					Value:    v,
					Location: NewLocation([]byte{}, "example.rego", 1, 2),
				}
				return &Import{
					Path:     &term,
					Location: NewLocation([]byte{}, "example.rego", 1, 2),
				}
			}(),
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Import: false,
					},
				},
			},
			ExpectedJSON: `{"path":{"type":"string","value":"example"}}`,
		},
		"location included": {
			Import: func() *Import {
				v, _ := InterfaceToValue("example")
				term := Term{
					Value:    v,
					Location: NewLocation([]byte{}, "example.rego", 1, 2),
				}
				return &Import{
					Path:     &term,
					Location: NewLocation([]byte{}, "example.rego", 1, 2),
				}
			}(),
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Import: true,
					},
				},
			},
			ExpectedJSON: `{"location":{"file":"example.rego","row":1,"col":2},"path":{"type":"string","value":"example"}}`,
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			astJSON.SetOptions(data.Options)
			t.Cleanup(resetJSONOptions)

			bs := util.MustMarshalJSON(data.Import)
			got := string(bs)
			exp := data.ExpectedJSON

			if got != exp {
				t.Fatalf("expected:\n%s got\n%s", exp, got)
			}
		})
	}
}

func TestRule_MarshalJSON(t *testing.T) {
	rawModule := `
	package foo

	# comment

	allow if { true }
	`

	module, err := ParseModuleWithOpts("example.rego", rawModule, ParserOptions{AllFutureKeywords: true})
	if err != nil {
		t.Fatal(err)
	}

	rule := module.Rules[0]

	testCases := map[string]struct {
		Rule         *Rule
		Options      astJSON.Options
		ExpectedJSON string
	}{
		"base case": {
			Rule:         rule,
			ExpectedJSON: `{"body":[{"index":0,"terms":{"type":"boolean","value":true}}],"head":{"name":"allow","value":{"type":"boolean","value":true},"ref":[{"type":"var","value":"allow"}]}}`,
		},
		"location excluded": {
			Rule: rule,
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Rule: false,
					},
				},
			},
			ExpectedJSON: `{"body":[{"index":0,"terms":{"type":"boolean","value":true}}],"head":{"name":"allow","value":{"type":"boolean","value":true},"ref":[{"type":"var","value":"allow"}]}}`,
		},
		"location included": {
			Rule: rule,
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Rule: true,
					},
				},
			},
			ExpectedJSON: `{"body":[{"index":0,"terms":{"type":"boolean","value":true}}],"head":{"name":"allow","value":{"type":"boolean","value":true},"ref":[{"type":"var","value":"allow"}]},"location":{"file":"example.rego","row":6,"col":2}}`,
		},
		"annotations included": {
			Rule: func() *Rule {
				r := rule.Copy()
				r.Annotations = []*Annotations{{
					Scope:         "rule",
					Title:         "My rule",
					Entrypoint:    true,
					Organizations: []string{"org1"},
					Description:   "My desc",
					Custom: map[string]any{
						"foo": "bar",
					}}}
				return r
			}(),
			ExpectedJSON: `{"annotations":[{"custom":{"foo":"bar"},"description":"My desc","entrypoint":true,"organizations":["org1"],"scope":"rule","title":"My rule"}],"body":[{"index":0,"terms":{"type":"boolean","value":true}}],"head":{"name":"allow","value":{"type":"boolean","value":true},"ref":[{"type":"var","value":"allow"}]}}`,
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			astJSON.SetOptions(data.Options)
			t.Cleanup(resetJSONOptions)

			bs := util.MustMarshalJSON(data.Rule)
			got := string(bs)
			exp := data.ExpectedJSON

			if got != exp {
				t.Fatalf("expected:\n%s got\n%s", exp, got)
			}
		})
	}
}

func TestHead_MarshalJSON(t *testing.T) {
	rawModule := `
	package foo

	# comment

	allow if { true }
	`

	module, err := ParseModuleWithOpts("example.rego", rawModule, ParserOptions{AllFutureKeywords: true})
	if err != nil {
		t.Fatal(err)
	}

	head := module.Rules[0].Head

	testCases := map[string]struct {
		Head         *Head
		Options      astJSON.Options
		ExpectedJSON string
	}{
		"base case": {
			Head:         head.Copy(),
			ExpectedJSON: `{"name":"allow","value":{"type":"boolean","value":true},"ref":[{"type":"var","value":"allow"}]}`,
		},
		"location excluded": {
			Head: head,
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Head: false,
					},
				},
			},
			ExpectedJSON: `{"name":"allow","value":{"type":"boolean","value":true},"ref":[{"type":"var","value":"allow"}]}`,
		},
		"location included": {
			Head: head,
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Head: true,
					},
				},
			},
			ExpectedJSON: `{"name":"allow","value":{"type":"boolean","value":true},"ref":[{"type":"var","value":"allow"}],"location":{"file":"example.rego","row":6,"col":2}}`,
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			astJSON.SetOptions(data.Options)
			t.Cleanup(resetJSONOptions)

			bs := util.MustMarshalJSON(data.Head)
			got := string(bs)
			exp := data.ExpectedJSON

			if got != exp {
				t.Fatalf("expected:\n%s got\n%s", exp, got)
			}
		})
	}
}

func TestRuleHeadRefWithTermLocations_MarshalJSON(t *testing.T) {
	policy := `package test

import rego.v1

ref.head[rule].test contains "value" if {
	rule := "rule"
}`

	astJSON.SetOptions(astJSON.Options{
		MarshalOptions: astJSON.MarshalOptions{
			IncludeLocation: astJSON.NodeToggle{
				Head: true,
				Term: true,
			},
		},
	})
	t.Cleanup(resetJSONOptions)

	module, err := ParseModuleWithOpts("test.rego", policy, ParserOptions{})
	if err != nil {
		t.Fatal(err)
	}

	bs, err := json.Marshal(module.Rules[0].Head)
	if err != nil {
		t.Fatal(err)
	}

	// Ensure marshalled JSON includes location for any term
	expectedJSON := `{"key":{"location":{"file":"test.rego","row":5,"col":30},"type":"string","value":"value"},"ref":[{"location":{"file":"test.rego","row":5,"col":1},"type":"var","value":"ref"},{"location":{"file":"test.rego","row":5,"col":5},"type":"string","value":"head"},{"location":{"file":"test.rego","row":5,"col":10},"type":"var","value":"rule"},{"location":{"file":"test.rego","row":5,"col":16},"type":"string","value":"test"}],"location":{"file":"test.rego","row":5,"col":1}}`

	if string(bs) != expectedJSON {
		t.Errorf("expected %s but got %s", expectedJSON, string(bs))
	}
}

func TestExpr_MarshalJSON(t *testing.T) {
	rawModule := `
	package foo

	# comment

	allow if { true }
	`

	module, err := ParseModuleWithOpts("example.rego", rawModule, ParserOptions{AllFutureKeywords: true})
	if err != nil {
		t.Fatal(err)
	}

	expr := module.Rules[0].Body[0]

	testCases := map[string]struct {
		Expr         *Expr
		Options      astJSON.Options
		ExpectedJSON string
	}{
		"base case": {
			Expr:         expr,
			ExpectedJSON: `{"index":0,"terms":{"type":"boolean","value":true}}`,
		},
		"location excluded": {
			Expr: expr,
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Expr: false,
					},
				},
			},
			ExpectedJSON: `{"index":0,"terms":{"type":"boolean","value":true}}`,
		},
		"location included": {
			Expr: expr,
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Expr: true,
					},
				},
			},
			ExpectedJSON: `{"index":0,"location":{"file":"example.rego","row":6,"col":13},"terms":{"type":"boolean","value":true}}`,
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			astJSON.SetOptions(data.Options)
			t.Cleanup(resetJSONOptions)

			bs := util.MustMarshalJSON(data.Expr)
			got := string(bs)
			exp := data.ExpectedJSON

			if got != exp {
				t.Fatalf("expected:\n%s got\n%s", exp, got)
			}
		})
	}
}

func TestExpr_UnmarshalJSON(t *testing.T) {
	rawModule := `
	package foo

	# comment

	allow if { true }
	`

	module, err := ParseModuleWithOpts("example.rego", rawModule, ParserOptions{AllFutureKeywords: true})
	if err != nil {
		t.Fatal(err)
	}

	expr := module.Rules[0].Body[0]
	// text is not marshalled to JSON so we just drop it in our examples
	expr.Location.Text = nil

	testCases := map[string]struct {
		JSON         string
		ExpectedExpr *Expr
	}{
		"base case": {
			JSON: `{"index":0,"terms":{"type":"boolean","value":true}}`,
			ExpectedExpr: func() *Expr {
				e := expr.Copy()
				e.Location = nil
				return e
			}(),
		},
		"location case": {
			JSON:         `{"index":0,"location":{"file":"example.rego","row":6,"col":13},"terms":{"type":"boolean","value":true}}`,
			ExpectedExpr: expr,
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			var expr Expr
			err := json.Unmarshal([]byte(data.JSON), &expr)
			if err != nil {
				t.Fatal(err)
			}

			if !expr.Equal(data.ExpectedExpr) {
				t.Fatalf("expected:\n%#v got\n%#v", data.ExpectedExpr, expr)
			}
			if data.ExpectedExpr.Location != nil {
				if !expr.Location.Equal(data.ExpectedExpr.Location) {
					t.Fatalf("expected location:\n%#v got\n%#v", data.ExpectedExpr.Location, expr.Location)
				}
			}
		})
	}
}

func TestSomeDecl_MarshalJSON(t *testing.T) {
	v, _ := InterfaceToValue("example")
	term := &Term{
		Value:    v,
		Location: NewLocation([]byte{}, "example.rego", 1, 2),
	}

	testCases := map[string]struct {
		SomeDecl     *SomeDecl
		Options      astJSON.Options
		ExpectedJSON string
	}{
		"base case": {
			SomeDecl: &SomeDecl{
				Symbols:  []*Term{term},
				Location: NewLocation([]byte{}, "example.rego", 1, 2),
			},
			ExpectedJSON: `{"symbols":[{"type":"string","value":"example"}]}`,
		},
		"location excluded": {
			SomeDecl: &SomeDecl{
				Symbols:  []*Term{term},
				Location: NewLocation([]byte{}, "example.rego", 1, 2),
			},
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{IncludeLocation: astJSON.NodeToggle{SomeDecl: false}},
			},
			ExpectedJSON: `{"symbols":[{"type":"string","value":"example"}]}`,
		},
		"location included": {
			SomeDecl: &SomeDecl{
				Symbols:  []*Term{term},
				Location: NewLocation([]byte{}, "example.rego", 1, 2),
			},
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{IncludeLocation: astJSON.NodeToggle{SomeDecl: true}},
			},
			ExpectedJSON: `{"location":{"file":"example.rego","row":1,"col":2},"symbols":[{"type":"string","value":"example"}]}`,
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			astJSON.SetOptions(data.Options)
			t.Cleanup(resetJSONOptions)

			bs := util.MustMarshalJSON(data.SomeDecl)
			got := string(bs)
			exp := data.ExpectedJSON

			if got != exp {
				t.Fatalf("expected:\n%s got\n%s", exp, got)
			}
		})
	}
}

func TestEvery_MarshalJSON(t *testing.T) {

	rawModule := `
package foo

allow if {
	every e in [1,2,3] {
		e == 1
    }
}
`

	module, err := ParseModuleWithOpts("example.rego", rawModule, ParserOptions{AllFutureKeywords: true})
	if err != nil {
		t.Fatal(err)
	}

	every, ok := module.Rules[0].Body[0].Terms.(*Every)
	if !ok {
		t.Fatal("expected every term")
	}

	testCases := map[string]struct {
		Every        *Every
		Options      astJSON.Options
		ExpectedJSON string
	}{
		"base case": {
			Every:        every,
			ExpectedJSON: `{"body":[{"index":0,"terms":[{"type":"ref","value":[{"type":"var","value":"equal"}]},{"type":"var","value":"e"},{"type":"number","value":1}]}],"domain":{"type":"array","value":[{"type":"number","value":1},{"type":"number","value":2},{"type":"number","value":3}]},"key":null,"value":{"type":"var","value":"e"}}`,
		},
		"location excluded": {
			Every: every,
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{IncludeLocation: astJSON.NodeToggle{Every: false}},
			},
			ExpectedJSON: `{"body":[{"index":0,"terms":[{"type":"ref","value":[{"type":"var","value":"equal"}]},{"type":"var","value":"e"},{"type":"number","value":1}]}],"domain":{"type":"array","value":[{"type":"number","value":1},{"type":"number","value":2},{"type":"number","value":3}]},"key":null,"value":{"type":"var","value":"e"}}`,
		},
		"location included": {
			Every:        every,
			Options:      astJSON.Options{MarshalOptions: astJSON.MarshalOptions{IncludeLocation: astJSON.NodeToggle{Every: true}}},
			ExpectedJSON: `{"body":[{"index":0,"terms":[{"type":"ref","value":[{"type":"var","value":"equal"}]},{"type":"var","value":"e"},{"type":"number","value":1}]}],"domain":{"type":"array","value":[{"type":"number","value":1},{"type":"number","value":2},{"type":"number","value":3}]},"key":null,"location":{"file":"example.rego","row":5,"col":2},"value":{"type":"var","value":"e"}}`,
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			astJSON.SetOptions(data.Options)
			t.Cleanup(resetJSONOptions)

			bs := util.MustMarshalJSON(data.Every)
			got := string(bs)
			exp := data.ExpectedJSON

			if got != exp {
				t.Fatalf("expected:\n%s got\n%s", exp, got)
			}
		})
	}
}

func TestWith_MarshalJSON(t *testing.T) {

	rawModule := `
package foo

a if {input}

b if {
	a with input as 1
}
`

	module, err := ParseModuleWithOpts("example.rego", rawModule, ParserOptions{AllFutureKeywords: true})
	if err != nil {
		t.Fatal(err)
	}

	with := module.Rules[1].Body[0].With[0]

	testCases := map[string]struct {
		Options      astJSON.Options
		ExpectedJSON string
	}{
		"base case": {
			ExpectedJSON: `{"target":{"type":"ref","value":[{"type":"var","value":"input"}]},"value":{"type":"number","value":1}}`,
		},
		"location excluded": {
			Options:      astJSON.Options{MarshalOptions: astJSON.MarshalOptions{IncludeLocation: astJSON.NodeToggle{With: false}}},
			ExpectedJSON: `{"target":{"type":"ref","value":[{"type":"var","value":"input"}]},"value":{"type":"number","value":1}}`,
		},
		"location included": {
			Options:      astJSON.Options{MarshalOptions: astJSON.MarshalOptions{IncludeLocation: astJSON.NodeToggle{With: true}}},
			ExpectedJSON: `{"location":{"file":"example.rego","row":7,"col":4},"target":{"type":"ref","value":[{"type":"var","value":"input"}]},"value":{"type":"number","value":1}}`,
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			astJSON.SetOptions(data.Options)
			t.Cleanup(resetJSONOptions)

			bs := util.MustMarshalJSON(with)
			got := string(bs)
			exp := data.ExpectedJSON

			if got != exp {
				t.Fatalf("expected:\n%s got\n%s", exp, got)
			}
		})
	}
}

func TestAnnotations_MarshalJSON(t *testing.T) {

	testCases := map[string]struct {
		Annotations  *Annotations
		Options      astJSON.Options
		ExpectedJSON string
	}{
		"base case": {
			Annotations: &Annotations{
				Scope:         "rule",
				Title:         "My rule",
				Entrypoint:    true,
				Organizations: []string{"org1"},
				Description:   "My desc",
				Custom: map[string]any{
					"foo": "bar",
				},
				Location: NewLocation([]byte{}, "example.rego", 1, 4),
			},
			ExpectedJSON: `{"custom":{"foo":"bar"},"description":"My desc","entrypoint":true,"organizations":["org1"],"scope":"rule","title":"My rule"}`,
		},
		"location excluded": {
			Annotations: &Annotations{
				Scope:         "rule",
				Title:         "My rule",
				Entrypoint:    true,
				Organizations: []string{"org1"},
				Description:   "My desc",
				Custom: map[string]any{
					"foo": "bar",
				},
				Location: NewLocation([]byte{}, "example.rego", 1, 4),
			},
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{Annotations: false},
				},
			},
			ExpectedJSON: `{"custom":{"foo":"bar"},"description":"My desc","entrypoint":true,"organizations":["org1"],"scope":"rule","title":"My rule"}`,
		},
		"location included": {
			Annotations: &Annotations{
				Scope:         "rule",
				Title:         "My rule",
				Entrypoint:    true,
				Organizations: []string{"org1"},
				Description:   "My desc",
				Custom: map[string]any{
					"foo": "bar",
				},
				Location: NewLocation([]byte{}, "example.rego", 1, 4),
			},
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{Annotations: true},
				},
			},
			ExpectedJSON: `{"custom":{"foo":"bar"},"description":"My desc","entrypoint":true,"location":{"file":"example.rego","row":1,"col":4},"organizations":["org1"],"scope":"rule","title":"My rule"}`,
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			astJSON.SetOptions(data.Options)
			t.Cleanup(resetJSONOptions)

			bs := util.MustMarshalJSON(data.Annotations)
			got := string(bs)
			exp := data.ExpectedJSON

			if got != exp {
				t.Fatalf("expected:\n%s got\n%s", exp, got)
			}
		})
	}
}

func TestAnnotationsRef_MarshalJSON(t *testing.T) {

	testCases := map[string]struct {
		AnnotationsRef *AnnotationsRef
		Options        astJSON.Options
		ExpectedJSON   string
	}{
		"base case": {
			AnnotationsRef: &AnnotationsRef{
				Path: []*Term{},
				// using an empty annotations object here since Annotations marshalling is tested separately
				Annotations: &Annotations{},
				Location:    NewLocation([]byte{}, "example.rego", 1, 4),
			},
			ExpectedJSON: `{"annotations":{"scope":""},"path":[]}`,
		},
		"location excluded": {
			AnnotationsRef: &AnnotationsRef{
				Path:        []*Term{},
				Annotations: &Annotations{},
				Location:    NewLocation([]byte{}, "example.rego", 1, 4),
			},
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{AnnotationsRef: false},
				},
			},
			ExpectedJSON: `{"annotations":{"scope":""},"path":[]}`,
		},
		"location included": {
			AnnotationsRef: &AnnotationsRef{
				Path:        []*Term{},
				Annotations: &Annotations{},
				Location:    NewLocation([]byte{}, "example.rego", 1, 4),
			},
			Options: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{AnnotationsRef: true},
				},
			},
			ExpectedJSON: `{"annotations":{"scope":""},"location":{"file":"example.rego","row":1,"col":4},"path":[]}`,
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			astJSON.SetOptions(data.Options)
			t.Cleanup(resetJSONOptions)

			bs := util.MustMarshalJSON(data.AnnotationsRef)
			got := string(bs)
			exp := data.ExpectedJSON

			if got != exp {
				t.Fatalf("expected:\n%s got\n%s", exp, got)
			}
		})
	}
}

func TestNewAnnotationsRef_JSONOptions(t *testing.T) {
	tests := []struct {
		note        string
		module      string
		expected    []string
		options     ParserOptions
		jsonOptions astJSON.Options
	}{
		{
			note: "all JSON marshaller options set to true",
			module: `# METADATA
# title: pkg
# description: pkg
# organizations:
# - pkg
# related_resources:
# - https://pkg
# authors:
# - pkg
# schemas:
# - input.foo: {"type": "boolean"}
# custom:
#  pkg: pkg
package test

# METADATA
# scope: document
# title: doc
# description: doc
# organizations:
# - doc
# related_resources:
# - https://doc
# authors:
# - doc
# schemas:
# - input.bar: {"type": "integer"}
# custom:
#  doc: doc

# METADATA
# title: rule
# description: rule
# organizations:
# - rule
# related_resources:
# - https://rule
# authors:
# - rule
# schemas:
# - input.baz: {"type": "string"}
# custom:
#  rule: rule
p = 1`,
			options: ParserOptions{
				ProcessAnnotation: true,
			},
			jsonOptions: astJSON.Options{
				MarshalOptions: astJSON.MarshalOptions{
					IncludeLocation: astJSON.NodeToggle{
						Term:           true,
						Package:        true,
						Comment:        true,
						Import:         true,
						Rule:           true,
						Head:           true,
						Expr:           true,
						SomeDecl:       true,
						Every:          true,
						With:           true,
						Annotations:    true,
						AnnotationsRef: true,
					},
				},
			},
			expected: []string{
				`{"annotations":{"authors":[{"name":"pkg"}],"custom":{"pkg":"pkg"},"description":"pkg","location":{"file":"","row":1,"col":1},"organizations":["pkg"],"related_resources":[{"ref":"https://pkg"}],"schemas":[{"path":[{"type":"var","value":"input"},{"type":"string","value":"foo"}],"definition":{"type":"boolean"}}],"scope":"package","title":"pkg"},"location":{"file":"","row":14,"col":1},"path":[{"location":{"file":"","row":14,"col":9},"type":"var","value":"data"},{"location":{"file":"","row":14,"col":9},"type":"string","value":"test"}]}`,
				`{"annotations":{"authors":[{"name":"doc"}],"custom":{"doc":"doc"},"description":"doc","location":{"file":"","row":16,"col":1},"organizations":["doc"],"related_resources":[{"ref":"https://doc"}],"schemas":[{"path":[{"type":"var","value":"input"},{"type":"string","value":"bar"}],"definition":{"type":"integer"}}],"scope":"document","title":"doc"},"location":{"file":"","row":44,"col":1},"path":[{"location":{"file":"","row":14,"col":9},"type":"var","value":"data"},{"location":{"file":"","row":14,"col":9},"type":"string","value":"test"},{"location":{"file":"","row":44,"col":1},"type":"string","value":"p"}]}`,
				`{"annotations":{"authors":[{"name":"rule"}],"custom":{"rule":"rule"},"description":"rule","location":{"file":"","row":31,"col":1},"organizations":["rule"],"related_resources":[{"ref":"https://rule"}],"schemas":[{"path":[{"type":"var","value":"input"},{"type":"string","value":"baz"}],"definition":{"type":"string"}}],"scope":"rule","title":"rule"},"location":{"file":"","row":44,"col":1},"path":[{"location":{"file":"","row":14,"col":9},"type":"var","value":"data"},{"location":{"file":"","row":14,"col":9},"type":"string","value":"test"},{"location":{"file":"","row":44,"col":1},"type":"string","value":"p"}]}`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.note, func(t *testing.T) {
			astJSON.SetOptions(tc.jsonOptions)
			t.Cleanup(resetJSONOptions)

			module := MustParseModuleWithOpts(tc.module, tc.options)

			if len(tc.expected) != len(module.Annotations) {
				t.Fatalf("expected %d annotations got %d", len(tc.expected), len(module.Annotations))
			}

			for i, a := range module.Annotations {
				ref := NewAnnotationsRef(a)

				bytes, err := json.Marshal(ref)
				if err != nil {
					t.Fatal(err)
				}

				got := string(bytes)
				expected := tc.expected[i]

				if got != expected {
					t.Fatalf("expected:\n%s got\n%s", expected, got)
				}
			}

		})
	}
}
