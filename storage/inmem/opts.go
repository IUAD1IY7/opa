package inmem

import v1 "github.com/IUAD1IY7/opa/v1/storage/inmem"

// An Opt modifies store at instantiation.
type Opt = v1.Opt

// OptRoundTripOnWrite sets whether incoming objects written to store are
// round-tripped through JSON to ensure they are serializable to JSON.
//
// Callers should disable this if they can guarantee all objects passed to
// Write() are serializable to JSON. Failing to do so may result in undefined
// behavior, including panics.
//
// Usually, when only storing objects in the inmem store that have been read
// via encoding/json, this is safe to disable, and comes with an improvement
// in performance and memory use.
//
// If setting to false, callers should deep-copy any objects passed to Write()
// unless they can guarantee the objects will not be mutated after being written,
// and that mutations happening to the objects after they have been passed into
// Write() don't affect their logic.
func OptRoundTripOnWrite(enabled bool) Opt {
	return v1.OptRoundTripOnWrite(enabled)
}

// OptReturnASTValuesOnRead sets whether data values added to the store should be
// eagerly converted to AST values, which are then returned on read.
//
// When enabled, this feature does not sanity check data before converting it to AST values,
// which may result in panics if the data is not valid. Callers should ensure that passed data
// can be serialized to AST values; otherwise, it's recommended to also enable OptRoundTripOnWrite.
func OptReturnASTValuesOnRead(enabled bool) Opt {
	return v1.OptReturnASTValuesOnRead(enabled)
}
