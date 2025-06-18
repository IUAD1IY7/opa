package topdown

import (
	"context"
	"testing"
	"time"

	inmem "github.com/IUAD1IY7/opa/v1/storage/inmem/test"
	"github.com/IUAD1IY7/opa/v1/ast"
	"github.com/IUAD1IY7/opa/v1/storage"
)

func TestNetCIDRExpandCancellation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	compiler := compileModules([]string{
		`
		package test

		p if { net.cidr_expand("1.0.0.0/1") }  # generating 2**31 hosts will take a while...
		`,
	})

	store := inmem.New()
	txn := storage.NewTransactionOrDie(ctx, store)
	cancel := NewCancel()

	query := NewQuery(ast.MustParseBody("data.test.p")).
		WithCompiler(compiler).
		WithStore(store).
		WithTransaction(txn).
		WithCancel(cancel)

	go func() {
		time.Sleep(time.Millisecond * 50)
		cancel.Cancel()
	}()

	qrs, err := query.Run(ctx)

	if err == nil || err.(*Error).Code != CancelErr {
		t.Fatalf("Expected cancel error but got: %v (err: %v)", qrs, err)
	}
}
