package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"reflect"
	"strings"
	"testing"

	"github.com/IUAD1IY7/opa/v1/util"
	"github.com/IUAD1IY7/opa/v1/util/test"
)

func TestOracleFindDefinition(t *testing.T) {
	cases := []struct {
		note         string
		v0Compatible bool
		onDiskModule string
		stdin        string
		paths        []string
	}{
		{
			note:         "v0",
			v0Compatible: true,
			onDiskModule: `package test

p { r }

r = true`,
			stdin: `package test

p { q }

q = true`,
			paths: []string{
				"test.rego:10",
				"test.rego:15",
				"test.rego:18",
			},
		},
		{
			note: "v1",
			onDiskModule: `package test

p if { r }

r = true`,
			stdin: `package test

p if { q }

q = true`,
			paths: []string{
				"test.rego:10",
				"test.rego:15",
				"test.rego:21",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.note, func(t *testing.T) {
			stdin := bytes.NewBufferString(tc.stdin)

			files := map[string]string{
				"test.rego":    tc.onDiskModule,
				"document.txt": "this should not be included",
				"ignore.json":  `{"neither": "should this"}`,
			}

			test.WithTempFS(files, func(rootDir string) {

				params := findDefinitionParams{
					bundlePaths: repeatedStringFlag{
						v:     []string{rootDir},
						isSet: true,
					},
					stdinBuffer:  true,
					v0Compatible: tc.v0Compatible,
				}

				stdout := bytes.NewBuffer(nil)

				err := dofindDefinition(params, stdin, stdout, []string{path.Join(rootDir, tc.paths[0])})
				expectJSON(t, err, stdout, `{"error": {"code": "oracle_no_match_found"}}`)

				err = dofindDefinition(params, stdin, stdout, []string{path.Join(rootDir, tc.paths[1])})
				expectJSON(t, err, stdout, `{"error": {"code": "oracle_no_definition_found"}}`)

				err = dofindDefinition(params, stdin, stdout, []string{path.Join(rootDir, tc.paths[2])})
				expectJSON(t, err, stdout, fmt.Sprintf(`{"result": {
			"file": %q,
			"row": 5,
			"col": 1
		}}`, path.Join(rootDir, "test.rego")))
			})
		})
	}

}

func expectJSON(t *testing.T, err error, buffer *bytes.Buffer, exp string) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	var x any
	if err := util.UnmarshalJSON(buffer.Bytes(), &x); err != nil {
		t.Fatal(err)
	}
	var y any
	if err := util.UnmarshalJSON([]byte(exp), &y); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(x, y) {
		t.Fatalf("expected %v but got %v", y, x)
	}
	buffer.Reset()
}

func TestOracleParseFilenameOffset(t *testing.T) {

	tests := []struct {
		input    string
		wantFile string
		wantPos  int
	}{
		{
			input:    "x.rego:10",
			wantFile: "x.rego",
			wantPos:  10,
		},
		{
			input:    "/x.rego:10",
			wantFile: "/x.rego",
			wantPos:  10,
		},
		{
			input:    "x.rego:0x10",
			wantFile: "x.rego",
			wantPos:  16,
		},
		{
			input:    "file://x.rego:10",
			wantFile: "x.rego",
			wantPos:  10,
		},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			filename, pos, err := parseFilenameOffset(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantFile != filename || tc.wantPos != pos {
				t.Fatalf("expected %v:%v but got %v:%v", tc.wantFile, tc.wantPos, filename, pos)
			}
		})
	}

}

func TestOracleParseFilenameOffsetError(t *testing.T) {

	tests := []struct {
		input   string
		wantErr error
	}{
		{
			input:   "x.rego",
			wantErr: errors.New("expected <filename>:<offset> argument"),
		},
		{
			input:   "x.rego:",
			wantErr: errors.New("invalid syntax"),
		},
		{
			input:   "x.rego:3.14",
			wantErr: errors.New("invalid syntax"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			_, _, err := parseFilenameOffset(tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr.Error()) {
				t.Fatalf("expected %v but got %v", tc.wantErr, err)
			}
		})
	}

}
