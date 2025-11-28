// Copyright 2017 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

// Package metrics contains helpers for performance metric management inside the policy engine.
package metrics

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unique"

	go_metrics "github.com/rcrowley/go-metrics"
)

type stringBuilderPool struct{ pool sync.Pool }

func (p *stringBuilderPool) Get() *strings.Builder {
	return p.pool.Get().(*strings.Builder)
}

func (p *stringBuilderPool) Put(sb *strings.Builder) {
	sb.Reset()
	p.pool.Put(sb)
}

var sbPool = &stringBuilderPool{
	pool: sync.Pool{
		New: func() any {
			return &strings.Builder{}
		},
	},
}

// Well-known metric names.
const (
	BundleRequest                     = "bundle_request"
	ServerHandler                     = "server_handler"
	ServerQueryCacheHit               = "server_query_cache_hit"
	SDKDecisionEval                   = "sdk_decision_eval"
	RegoQueryCompile                  = "rego_query_compile"
	RegoQueryEval                     = "rego_query_eval"
	RegoQueryParse                    = "rego_query_parse"
	RegoModuleParse                   = "rego_module_parse"
	RegoDataParse                     = "rego_data_parse"
	RegoModuleCompile                 = "rego_module_compile"
	RegoPartialEval                   = "rego_partial_eval"
	RegoInputParse                    = "rego_input_parse"
	RegoLoadFiles                     = "rego_load_files"
	RegoLoadBundles                   = "rego_load_bundles"
	RegoExternalResolve               = "rego_external_resolve"
	CompilePrepPartial                = "compile_prep_partial"
	CompileEvalConstraints            = "compile_eval_constraints"
	CompileTranslateQueries           = "compile_translate_queries"
	CompileExtractAnnotationsUnknowns = "compile_extract_annotations_unknowns"
	CompileExtractAnnotationsMask     = "compile_extract_annotations_mask"
	CompileEvalMaskRule               = "compile_eval_mask_rule"
)

// Info contains attributes describing the underlying metrics provider.
type Info struct {
	Name string `json:"name"` // name is a unique human-readable identifier for the provider.
}

// Metrics defines the interface for a collection of performance metrics in the
// policy engine.
type Metrics interface {
	Info() Info
	Timer(name string) Timer
	Histogram(name string) Histogram
	Counter(name string) Counter
	All() map[string]any
	Clear()
	json.Marshaler
}

type TimerMetrics interface {
	Timers() map[string]any
}

// metricsState holds the actual metrics maps and is replaced atomically
type metricsState struct {
	timers     map[unique.Handle[string]]Timer
	histograms map[unique.Handle[string]]Histogram
	counters   map[unique.Handle[string]]Counter
}

type metrics struct {
	state atomic.Pointer[metricsState]
	mtx   sync.Mutex // Only for writes to ensure consistency
}

// New returns a new Metrics object.
func New() Metrics {
	m := &metrics{}
	initialState := &metricsState{
		timers:     make(map[unique.Handle[string]]Timer),
		histograms: make(map[unique.Handle[string]]Histogram),
		counters:   make(map[unique.Handle[string]]Counter),
	}
	m.state.Store(initialState)
	return m
}

// NoOp returns a Metrics implementation that does nothing and costs nothing.
// Used when metrics are expected, but not of interest.
func NoOp() Metrics {
	return noOpMetricsInstance
}

type metric struct {
	Key   string
	Value any
}

func (*metrics) Info() Info {
	return Info{
		Name: "<built-in>",
	}
}

func (m *metrics) String() string {
	all := m.All()
	sorted := make([]metric, 0, len(all))

	for key, value := range all {
		sorted = append(sorted, metric{
			Key:   key,
			Value: value,
		})
	}

	slices.SortFunc(sorted, func(a, b metric) int {
		return strings.Compare(a.Key, b.Key)
	})

	buf := sbPool.Get()
	defer sbPool.Put(buf)

	totalLen := 0
	for i := range sorted {
		totalLen += len(sorted[i].Key) + 20 // estimate for value and separators
	}
	buf.Grow(totalLen)

	for i := range sorted {
		if i > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(sorted[i].Key)
		buf.WriteByte(':')
		fmt.Fprintf(buf, "%v", sorted[i].Value)
	}

	return buf.String()
}

func (m *metrics) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.All())
}

func (m *metrics) Timer(name string) Timer {
	handle := unique.Make(name)
	
	// Fast path: read without lock
	state := m.state.Load()
	if t, ok := state.timers[handle]; ok {
		return t
	}

	// Slow path: need to add new timer
	m.mtx.Lock()
	defer m.mtx.Unlock()

	// Double-check after acquiring lock
	state = m.state.Load()
	if t, ok := state.timers[handle]; ok {
		return t
	}

	// Create new state with added timer
	newState := &metricsState{
		timers:     make(map[unique.Handle[string]]Timer, len(state.timers)+1),
		histograms: state.histograms,
		counters:   state.counters,
	}
	for k, v := range state.timers {
		newState.timers[k] = v
	}
	
	t := &timer{}
	newState.timers[handle] = t
	m.state.Store(newState)
	
	return t
}

func (m *metrics) Histogram(name string) Histogram {
	handle := unique.Make(name)
	
	// Fast path: read without lock
	state := m.state.Load()
	if h, ok := state.histograms[handle]; ok {
		return h
	}

	// Slow path: need to add new histogram
	m.mtx.Lock()
	defer m.mtx.Unlock()

	// Double-check after acquiring lock
	state = m.state.Load()
	if h, ok := state.histograms[handle]; ok {
		return h
	}

	// Create new state with added histogram
	newState := &metricsState{
		timers:     state.timers,
		histograms: make(map[unique.Handle[string]]Histogram, len(state.histograms)+1),
		counters:   state.counters,
	}
	for k, v := range state.histograms {
		newState.histograms[k] = v
	}
	
	h := newHistogram()
	newState.histograms[handle] = h
	m.state.Store(newState)
	
	return h
}

func (m *metrics) Counter(name string) Counter {
	handle := unique.Make(name)
	
	// Fast path: read without lock
	state := m.state.Load()
	if c, ok := state.counters[handle]; ok {
		return c
	}

	// Slow path: need to add new counter
	m.mtx.Lock()
	defer m.mtx.Unlock()

	// Double-check after acquiring lock
	state = m.state.Load()
	if c, ok := state.counters[handle]; ok {
		return c
	}

	// Create new state with added counter
	newState := &metricsState{
		timers:     state.timers,
		histograms: state.histograms,
		counters:   make(map[unique.Handle[string]]Counter, len(state.counters)+1),
	}
	for k, v := range state.counters {
		newState.counters[k] = v
	}
	
	c := &counter{}
	newState.counters[handle] = c
	m.state.Store(newState)
	
	return c
}

func (m *metrics) All() map[string]any {
	state := m.state.Load()
	result := make(map[string]any, len(state.timers)+len(state.histograms)+len(state.counters))
	
	for handle, timer := range state.timers {
		name := handle.Value()
		result[m.formatKey(name, timer)] = timer.Value()
	}
	for handle, hist := range state.histograms {
		name := handle.Value()
		result[m.formatKey(name, hist)] = hist.Value()
	}
	for handle, cntr := range state.counters {
		name := handle.Value()
		result[m.formatKey(name, cntr)] = cntr.Value()
	}
	
	return result
}

func (m *metrics) Timers() map[string]any {
	state := m.state.Load()
	ts := make(map[string]any, len(state.timers))
	
	for handle, t := range state.timers {
		name := handle.Value()
		ts[m.formatKey(name, t)] = t.Value()
	}
	
	return ts
}

func (m *metrics) Clear() {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	
	newState := &metricsState{
		timers:     make(map[unique.Handle[string]]Timer),
		histograms: make(map[unique.Handle[string]]Histogram),
		counters:   make(map[unique.Handle[string]]Counter),
	}
	m.state.Store(newState)
}

func (*metrics) formatKey(name string, metrics any) string {
	switch metrics.(type) {
	case Timer:
		return "timer_" + name + "_ns"
	case Histogram:
		return "histogram_" + name
	case Counter:
		return "counter_" + name
	default:
		return name
	}
}

// Timer defines the interface for a restartable timer that accumulates elapsed
// time.
type Timer interface {
	Value() any
	Int64() int64
	// Start or resume a timer's time tracking.
	Start()
	// Stop a timer, and accumulate the delta (in nanoseconds) since it was last
	// started.
	Stop() int64
}

type timer struct {
	start atomic.Int64 // nanoseconds since epoch, 0 means not started
	value atomic.Int64 // accumulated nanoseconds
}

func (t *timer) Start() {
	t.start.Store(time.Now().UnixNano())
}

func (t *timer) Stop() int64 {
	startNs := t.start.Swap(0) // Reset to 0 atomically
	if startNs == 0 {
		return 0
	}

	delta := time.Now().UnixNano() - startNs
	if delta < 0 {
		delta = 0 // Clock skew protection
	}
	
	t.value.Add(delta)
	return delta
}

func (t *timer) Value() any {
	return t.Int64()
}

func (t *timer) Int64() int64 {
	return t.value.Load()
}

// Histogram defines the interface for a histogram with hardcoded percentiles.
type Histogram interface {
	Value() any
	Update(int64)
}

type histogram struct {
	hist go_metrics.Histogram // is thread-safe because of the underlying ExpDecaySample
}

func newHistogram() Histogram {
	// NOTE(tsandall): the reservoir size and alpha factor are taken from
	// https://github.com/rcrowley/go-metrics. They may need to be tweaked in
	// the future.
	sample := go_metrics.NewExpDecaySample(1028, 0.015)
	hist := go_metrics.NewHistogram(sample)
	return &histogram{hist}
}

func (h *histogram) Update(v int64) {
	h.hist.Update(v)
}

func (h *histogram) Value() any {
	snap := h.hist.Snapshot()
	percentiles := snap.Percentiles([]float64{
		0.5,
		0.75,
		0.9,
		0.95,
		0.99,
		0.999,
		0.9999,
	})
	
	// Preallocate map with exact size
	values := make(map[string]any, 12)
	values["count"] = snap.Count()
	values["min"] = snap.Min()
	values["max"] = snap.Max()
	values["mean"] = snap.Mean()
	values["stddev"] = snap.StdDev()
	values["median"] = percentiles[0]
	values["75%"] = percentiles[1]
	values["90%"] = percentiles[2]
	values["95%"] = percentiles[3]
	values["99%"] = percentiles[4]
	values["99.9%"] = percentiles[5]
	values["99.99%"] = percentiles[6]
	
	return values
}

// Counter defines the interface for a monotonic increasing counter.
type Counter interface {
	Value() any
	Incr()
	Add(n uint64)
}

type counter struct {
	c uint64
}

func (c *counter) Incr() {
	atomic.AddUint64(&c.c, 1)
}

func (c *counter) Add(n uint64) {
	atomic.AddUint64(&c.c, n)
}

func (c *counter) Value() any {
	return atomic.LoadUint64(&c.c)
}

func Statistics(num ...int64) any {
	t := newHistogram()
	for _, n := range num {
		t.Update(n)
	}
	return t.Value()
}

type noOpMetrics struct{}
type noOpTimer struct{}
type noOpHistogram struct{}
type noOpCounter struct{}

var (
	noOpMetricsInstance   = &noOpMetrics{}
	noOpTimerInstance     = &noOpTimer{}
	noOpHistogramInstance = &noOpHistogram{}
	noOpCounterInstance   = &noOpCounter{}
)

func (*noOpMetrics) Info() Info                      { return Info{Name: "<built-in no-op>"} }
func (*noOpMetrics) Timer(name string) Timer         { return noOpTimerInstance }
func (*noOpMetrics) Histogram(name string) Histogram { return noOpHistogramInstance }
func (*noOpMetrics) Counter(name string) Counter     { return noOpCounterInstance }
func (*noOpMetrics) All() map[string]any             { return nil }
func (*noOpMetrics) Clear()                          {}
func (*noOpMetrics) MarshalJSON() ([]byte, error) {
	return []byte(`{"name": "<built-in no-op>"}`), nil
}

func (*noOpTimer) Start()       {}
func (*noOpTimer) Stop() int64  { return 0 }
func (*noOpTimer) Value() any   { return 0 }
func (*noOpTimer) Int64() int64 { return 0 }

func (*noOpHistogram) Update(v int64) {}
func (*noOpHistogram) Value() any     { return nil }

func (*noOpCounter) Incr()        {}
func (*noOpCounter) Add(_ uint64) {}
func (*noOpCounter) Value() any   { return 0 }
func (*noOpCounter) Int64() int64 { return 0 }
