package test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/IUAD1IY7/opa/v1/logging"
	test_sdk "github.com/IUAD1IY7/opa/v1/sdk/test"
	"github.com/IUAD1IY7/opa/v1/test/e2e"
	"github.com/IUAD1IY7/opa/v1/util/test"
)

func TestEnablePrintStatementsForFilesystemPolicies(t *testing.T) {

	files := map[string]string{
		"/test.rego": `
			package test
			import rego.v1

			p if {
				print("hello world")
			}
		`,
	}

	test.WithTempFS(files, func(dir string) {

		params := e2e.NewAPIServerTestParams()
		params.Paths = []string{dir}

		buf := bytes.NewBuffer(nil)

		logger := logging.New()
		logger.SetOutput(buf)
		params.Logger = logger

		e2e.WithRuntime(t, e2e.TestRuntimeOpts{}, params, func(rt *e2e.TestRuntime) {

			var dr struct {
				Result bool `json:"result"`
			}

			if err := rt.GetDataWithInputTyped("test/p", nil, &dr); err != nil {
				t.Fatal(err)
			} else if !dr.Result {
				t.Fatal("expected true")
			}

			expContains := "hello world"

			if !strings.Contains(buf.String(), expContains) {
				t.Fatalf("expected logs to contain %q but got: %v", expContains, buf.String())
			}
		})
	})

}

func TestEnablePrintStatementsForHTTPAPIPushedPolicies(t *testing.T) {
	policy := `
		package test
		import rego.v1

		p if {
			print("hello world")
		}
	`

	params := e2e.NewAPIServerTestParams()

	buf := bytes.NewBuffer(nil)

	logger := logging.New()
	logger.SetOutput(buf)
	params.Logger = logger

	e2e.WithRuntime(t, e2e.TestRuntimeOpts{}, params, func(rt *e2e.TestRuntime) {

		if err := rt.UploadPolicy("test.rego", bytes.NewBufferString(policy)); err != nil {
			t.Fatal(err)
		}

		var dr struct {
			Result bool `json:"result"`
		}

		if err := rt.GetDataWithInputTyped("test/p", nil, &dr); err != nil {
			t.Fatal(err)
		} else if !dr.Result {
			t.Fatal("expected true")
		}

		expContains := "hello world"

		if !strings.Contains(buf.String(), expContains) {
			t.Fatalf("expected logs to contain %q but got: %v", expContains, buf.String())
		}
	})

}

func TestEnablePrintStatementsForBundles(t *testing.T) {

	server := test_sdk.MustNewServer(
		test_sdk.RawBundles(true),
		test_sdk.MockBundle("/bundles/bundle.tar.gz", map[string]string{
			"test.rego": `
			package test
			import rego.v1

			p if {
				print("hello world")
			}
		`,
		}))

	params := e2e.NewAPIServerTestParams()

	buf := bytes.NewBuffer(nil)

	logger := logging.New()
	logger.SetOutput(buf)
	params.Logger = logger

	params.ConfigOverrides = []string{
		"services.test.url=" + server.URL(),
		"bundles.test.resource=/bundles/bundle.tar.gz",
	}

	e2e.WithRuntime(t, e2e.TestRuntimeOpts{WaitForBundles: true}, params, func(rt *e2e.TestRuntime) {

		var dr struct {
			Result bool `json:"result"`
		}

		if err := rt.GetDataWithInputTyped("test/p", nil, &dr); err != nil {
			t.Fatal(err)
		} else if !dr.Result {
			t.Fatal("expected true")
		}

		expContains := "hello world"

		if !strings.Contains(buf.String(), expContains) {
			t.Fatalf("expected logs to contain %q but got: %v", expContains, buf.String())
		}
	})
}
