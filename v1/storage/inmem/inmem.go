package inmem

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/IUAD1IY7/opa/v1/ast"
	"github.com/IUAD1IY7/opa/v1/storage"
	"github.com/IUAD1IY7/opa/v1/util"
)

// New returns an empty in-memory store.
func New() storage.Store {
	return NewWithOpts()
}

// NewWithOpts returns an empty in-memory store, with extra options passed.
func NewWithOpts(opts ...Opt) storage.Store {
	s := &store{
		triggers:              nil,                  // Lazy initialization
		policies:              &map[string][]byte{}, // Initialize policies to a non-nil empty map
		roundTripOnWrite:      false,
		returnASTValuesOnRead: false,
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.returnASTValuesOnRead {
		s.data = ast.NewObject()
	} else {
		s.data = map[string]any{}
	}

	return s
}

// NewFromObject returns a new in-memory store from the supplied data object.
func NewFromObject(data map[string]any) storage.Store {
	return NewFromObjectWithOpts(data)
}

// NewFromObjectWithOpts returns a new in-memory store from the supplied data object, with the
// options passed.
func NewFromObjectWithOpts(data map[string]any, opts ...Opt) storage.Store {
	db := NewWithOpts(opts...)
	ctx := context.Background()
	txn, err := db.NewTransaction(ctx, storage.WriteParams)
	if err != nil {
		panic(err)
	}
	if err := db.Write(ctx, txn, storage.AddOp, storage.Path{}, data); err != nil {
		panic(err)
	}
	if err := db.Commit(ctx, txn); err != nil {
		panic(err)
	}
	return db
}

// NewFromReader returns a new in-memory store from a reader that produces a
// JSON serialized object. This function is for test purposes.
func NewFromReader(r io.Reader) storage.Store {
	return NewFromReaderWithOpts(r)
}

// NewFromReader returns a new in-memory store from a reader that produces a
// JSON serialized object, with extra options. This function is for test purposes.
func NewFromReaderWithOpts(r io.Reader, opts ...Opt) storage.Store {
	d := util.NewJSONDecoder(r)
	var data map[string]any
	if err := d.Decode(&data); err != nil {
		panic(err)
	}
	return NewFromObjectWithOpts(data, opts...)
}

type store struct {
	rmu      sync.RWMutex                       // reader-writer lock
	wmu      sync.Mutex                         // writer lock
	xid      uint64                             // last generated transaction id
	data     any                                // raw or AST data
	policies *map[string][]byte                 // Lazy pointer to policies
	triggers *map[*handle]storage.TriggerConfig // Lazy pointer to triggers

	roundTripOnWrite      bool
	returnASTValuesOnRead bool
}

type handle struct {
	db *store
}

func (db *store) NewTransaction(_ context.Context, params ...storage.TransactionParams) (storage.Transaction, error) {
	var write bool
	var ctx *storage.Context
	if len(params) > 0 {
		write = params[0].Write
		ctx = params[0].Context
	}
	xid := atomic.AddUint64(&db.xid, uint64(1))
	if write {
		db.wmu.Lock()
	} else {
		db.rmu.RLock()
	}
	return newTransaction(xid, write, ctx, db), nil
}

// Truncate implements the storage.Store interface. This method must be called within a transaction.
func (db *store) Truncate(ctx context.Context, txn storage.Transaction, params storage.TransactionParams, it storage.Iterator) error {
	// Use minimal initial capacity
	mergedData := make(map[string]any)

	var update *storage.Update
	var err error

	underlying, err := db.underlying(txn)
	if err != nil {
		return err
	}

	// Count updates for optimization
	updateCount := 0
	for {
		update, err = it.Next()
		if err != nil {
			break
		}
		updateCount++

		// Expand map only when necessary
		if len(mergedData) == 0 && updateCount == 1 {
			mergedData = make(map[string]any, max(len(params.BasePaths), 4))
		}

		if update.IsPolicy {
			err = underlying.UpsertPolicy(strings.TrimLeft(update.Path.String(), "/"), update.Value)
			if err != nil {
				return err
			}
		} else {
			var value any
			err = util.Unmarshal(update.Value, &value)
			if err != nil {
				return err
			}

			dirpath := strings.TrimLeft(update.Path.String(), "/")
			var key []string
			if len(dirpath) > 0 {
				key = strings.Split(dirpath, "/")
			}

			if value != nil {
				obj, err := mktree(key, value)
				if err != nil {
					return err
				}

				merged, ok := InterfaceMaps(mergedData, obj)
				if !ok {
					return fmt.Errorf("failed to insert data file from path %s", filepath.Join(key...))
				}
				mergedData = merged
			}
		}
	}

	// err is known not to be nil at this point, as it getting assigned
	// a non-nil value is the only way the loop above can exit.
	if err != io.EOF {
		return err
	}

	// For backwards compatibility, check if `RootOverwrite` was configured.
	if params.RootOverwrite {
		newPath, ok := storage.ParsePathEscaped("/")
		if !ok {
			return fmt.Errorf("storage path invalid: %v", newPath)
		}
		return underlying.Write(storage.AddOp, newPath, mergedData)
	}

	for _, root := range params.BasePaths {
		newPath, ok := storage.ParsePathEscaped("/" + root)
		if !ok {
			return fmt.Errorf("storage path invalid: %v", newPath)
		}

		if value, ok := lookup(newPath, mergedData); ok {
			if len(newPath) > 0 {
				if err := storage.MakeDir(ctx, db, txn, newPath[:len(newPath)-1]); err != nil {
					return err
				}
			}
			if err := underlying.Write(storage.AddOp, newPath, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func (db *store) Commit(ctx context.Context, txn storage.Transaction) error {
	underlying, err := db.underlying(txn)
	if err != nil {
		return err
	}
	if underlying.write {
		db.rmu.Lock()
		event := underlying.Commit()
		db.runOnCommitTriggers(ctx, txn, event)
		// Mark the transaction stale after executing triggers, so they can
		// perform store operations if needed.
		underlying.stale = true
		db.rmu.Unlock()
		db.wmu.Unlock()
	} else {
		db.rmu.RUnlock()
	}
	return nil
}

func (db *store) Abort(_ context.Context, txn storage.Transaction) {
	underlying, err := db.underlying(txn)
	if err != nil {
		panic(err)
	}
	underlying.stale = true
	if underlying.write {
		db.wmu.Unlock()
	} else {
		db.rmu.RUnlock()
	}
}

func (db *store) ListPolicies(_ context.Context, txn storage.Transaction) ([]string, error) {
	underlying, err := db.underlying(txn)
	if err != nil {
		return nil, err
	}
	return underlying.ListPolicies(), nil
}

func (db *store) GetPolicy(_ context.Context, txn storage.Transaction, id string) ([]byte, error) {
	underlying, err := db.underlying(txn)
	if err != nil {
		return nil, err
	}
	return underlying.GetPolicy(id)
}

func (db *store) UpsertPolicy(_ context.Context, txn storage.Transaction, id string, bs []byte) error {
	underlying, err := db.underlying(txn)
	if err != nil {
		return err
	}
	return underlying.UpsertPolicy(id, bs)
}

func (db *store) DeletePolicy(_ context.Context, txn storage.Transaction, id string) error {
	underlying, err := db.underlying(txn)
	if err != nil {
		return err
	}
	if _, err := underlying.GetPolicy(id); err != nil {
		return err
	}
	return underlying.DeletePolicy(id)
}

func (db *store) Register(_ context.Context, txn storage.Transaction, config storage.TriggerConfig) (storage.TriggerHandle, error) {
	underlying, err := db.underlying(txn)
	if err != nil {
		return nil, err
	}
	if !underlying.write {
		return nil, &storage.Error{
			Code:    storage.InvalidTransactionErr,
			Message: "triggers must be registered with a write transaction",
		}
	}

	if db.triggers == nil {
		triggers := make(map[*handle]storage.TriggerConfig)
		db.triggers = &triggers
	}
	h := &handle{db}
	(*db.triggers)[h] = config
	return h, nil
}

func (db *store) Read(_ context.Context, txn storage.Transaction, path storage.Path) (any, error) {
	underlying, err := db.underlying(txn)
	if err != nil {
		return nil, err
	}

	v, err := underlying.Read(path)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func (db *store) Write(_ context.Context, txn storage.Transaction, op storage.PatchOp, path storage.Path, value any) error {
	underlying, err := db.underlying(txn)
	if err != nil {
		return err
	}
	val := util.Reference(value)
	if db.roundTripOnWrite {
		if err := util.RoundTrip(val); err != nil {
			return err
		}
	}
	return underlying.Write(op, path, *val)
}

func (h *handle) Unregister(_ context.Context, txn storage.Transaction) {
	underlying, err := h.db.underlying(txn)
	if err != nil {
		panic(err)
	}
	if !underlying.write {
		panic(&storage.Error{
			Code:    storage.InvalidTransactionErr,
			Message: "triggers must be unregistered with a write transaction",
		})
	}

	if h.db.triggers != nil {
		delete(*h.db.triggers, h)
	}
}

func (db *store) runOnCommitTriggers(ctx context.Context, txn storage.Transaction, event storage.TriggerEvent) {
	// Check for triggers without initialization
	if db.triggers == nil || len(*db.triggers) == 0 {
		return
	}

	if db.returnASTValuesOnRead {
		// Lazy initialization of dataEvents
		var dataEvents []storage.DataEvent
		if len(event.Data) > 0 {
			dataEvents = make([]storage.DataEvent, 0, len(event.Data))

			for _, dataEvent := range event.Data {
				if astData, ok := dataEvent.Data.(ast.Value); ok {
					jsn, err := ast.ValueToInterface(astData, illegalResolver{})
					if err != nil {
						panic(err)
					}
					dataEvents = append(dataEvents, storage.DataEvent{
						Path:    dataEvent.Path,
						Data:    jsn,
						Removed: dataEvent.Removed,
					})
				} else {
					dataEvents = append(dataEvents, dataEvent)
				}
			}

			event = storage.TriggerEvent{
				Policy:  event.Policy,
				Data:    dataEvents,
				Context: event.Context,
			}
		}
	}

	for _, t := range *db.triggers {
		t.OnCommit(ctx, txn, event)
	}
}

type illegalResolver struct{}

func (illegalResolver) Resolve(ref ast.Ref) (any, error) {
	return nil, fmt.Errorf("illegal value: %v", ref)
}

func (db *store) underlying(txn storage.Transaction) (*transaction, error) {
	underlying, ok := txn.(*transaction)
	if !ok {
		return nil, &storage.Error{
			Code:    storage.InvalidTransactionErr,
			Message: fmt.Sprintf("unexpected transaction type %T", txn),
		}
	}
	if underlying.db != db {
		return nil, &storage.Error{
			Code:    storage.InvalidTransactionErr,
			Message: "unknown transaction",
		}
	}
	if underlying.stale {
		return nil, &storage.Error{
			Code:    storage.InvalidTransactionErr,
			Message: "stale transaction",
		}
	}
	return underlying, nil
}

const rootMustBeObjectMsg = "root must be object"
const rootCannotBeRemovedMsg = "root cannot be removed"

func invalidPatchError(f string, a ...any) *storage.Error {
	return &storage.Error{
		Code:    storage.InvalidPatchErr,
		Message: fmt.Sprintf(f, a...),
	}
}

// Optimized mktree function with minimal allocations
func mktree(path []string, value any) (map[string]any, error) {
	if len(path) == 0 {
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, invalidPatchError(rootMustBeObjectMsg)
		}
		return obj, nil
	}

	// Use a single object for the entire chain
	result := make(map[string]any, 1)
	current := result

	for i := range len(path) - 1 {
		next := make(map[string]any, 1)
		current[path[i]] = next
		current = next
	}
	current[path[len(path)-1]] = value

	return result, nil
}

func lookup(path storage.Path, data map[string]any) (any, bool) {
	if len(path) == 0 {
		return data, true
	}
	// Loop from 0 to len(path)-1 instead of iterating over an integer.
	for i := range len(path) - 1 {
		value, ok := data[path[i]]
		if !ok {
			return nil, false
		}
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		data = obj
	}
	value, ok := data[path[len(path)-1]]
	return value, ok
}
