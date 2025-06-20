package concurrency

import (
	"flag"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/IUAD1IY7/opa/v1/server/types"
	"github.com/IUAD1IY7/opa/v1/storage/disk"
	"github.com/IUAD1IY7/opa/v1/test/e2e"
)

var testRuntime *e2e.TestRuntime

func TestMain(m *testing.M) {
	flag.Parse()
	testServerParams := e2e.NewAPIServerTestParams()

	dir, err := os.MkdirTemp("", "disk-store")
	if err != nil {
		panic(err)
	}
	defer func() { os.RemoveAll(dir) }()

	for _, opts := range []*disk.Options{
		nil,
		{Dir: dir, Partitions: nil},
	} {
		var err error
		testServerParams.DiskStorage = opts
		testRuntime, err = e2e.NewTestRuntime(testServerParams)
		if err != nil {
			panic(err)
		}
		if ec := testRuntime.RunTests(m); ec != 0 {
			os.Exit(ec)
		}
	}
}

func TestConcurrencyGetV1Data(t *testing.T) {

	policy := `
	package test
	p = true
	`

	err := testRuntime.UploadPolicy(t.Name(), strings.NewReader(policy))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	num := runtime.NumCPU()
	wg.Add(num)

	for range num {
		go func() {
			defer wg.Done()
			for range 1000 {
				dr := struct {
					Result bool `json:"result"`
				}{}
				if err := testRuntime.GetDataWithInputTyped("test/p", nil, &dr); err != nil {
					t.Error(err)
					return
				}
				if !dr.Result {
					t.Errorf("Unexpected response: %+v", dr)
					return
				}
			}
		}()
	}

	wg.Wait()
}

func TestConcurrencyCompile(t *testing.T) {

	policy := `
	package test
	import rego.v1

	f(_)
	p if {
		not q
	}
	q if {
		not f(input.foo)
	}
	`

	err := testRuntime.UploadPolicy(t.Name(), strings.NewReader(policy))
	if err != nil {
		t.Fatal(err)
	}

	req := types.CompileRequestV1{
		Query: "data.test.p",
	}

	var wg sync.WaitGroup
	num := runtime.NumCPU()
	wg.Add(num)

	for range num {
		go func() {
			defer wg.Done()
			for range 1000 {
				if _, err := testRuntime.CompileRequest(req); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}

	wg.Wait()
}
