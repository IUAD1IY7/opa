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

	go_metrics "github.com/rcrowley/go-metrics"
)

var metricSlicePool = sync.Pool{
	New: func() any {
		return make([]metric, 0, 16)
	},
}

var stringSlicePool = sync.Pool{
	New: func() any {
		return make([]string, 0, 16)
	},
}

// Intern common metric names for memory optimization
var (
	metricNameIntern = make(map[string]string)
	metricNameMux    = sync.RWMutex{}
	
	// Pre-intern well-known metric names
	wellKnownMetrics = []string{
		BundleRequest,
		ServerHandler,
		ServerQueryCacheHit,
		SDKDecisionEval,
		RegoQueryCompile,
		RegoQueryEval,
		RegoQueryParse,
		RegoModuleParse,
		RegoDataParse,
		RegoModuleCompile,
		RegoPartialEval,
		RegoInputParse,
		RegoLoadFiles,
		RegoLoadBundles,
		RegoExternalResolve,
	}
)

func init() {
	// Pre-intern well-known metric names
	for _, name := range wellKnownMetrics {
		metricNameIntern[name] = name
	}
}

// internMetricName interns metric names to save memory
func internMetricName(name string) string {
	metricNameMux.RLock()
	if interned, exists := metricNameIntern[name]; exists {
		metricNameMux.RUnlock()
		return interned
	}
	metricNameMux.RUnlock()
	
	metricNameMux.Lock()
	defer metricNameMux.Unlock()
	
	// Double-check after acquiring write lock
	if interned, exists := metricNameIntern[name]; exists {
		return interned
	}
	
	metricNameIntern[name] = name
	return name
}

// Well-known metric names.
const (
	BundleRequest       = "bundle_request"
	ServerHandler       = "server_handler"
	ServerQueryCacheHit = "server_query_cache_hit"
	SDKDecisionEval     = "sdk_decision_eval"
	RegoQueryCompile    = "rego_query_compile"
	RegoQueryEval       = "rego_query_eval"
	RegoQueryParse      = "rego_query_parse"
	RegoModuleParse     = "rego_module_parse"
	RegoDataParse       = "rego_data_parse"
	RegoModuleCompile   = "rego_module_compile"
	RegoPartialEval     = "rego_partial_eval"
	RegoInputParse      = "rego_input_parse"
	RegoLoadFiles       = "rego_load_files"
	RegoLoadBundles     = "rego_load_bundles"
	RegoExternalResolve = "rego_external_resolve"
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

type metrics struct {
	mtx        sync.Mutex
	timers     map[string]Timer
	histograms map[string]Histogram
	counters   map[string]Counter
}

// New returns a new Metrics object.
func New() Metrics {
	return &metrics{
		timers:     make(map[string]Timer, 8),
		histograms: make(map[string]Histogram, 8),
		counters:   make(map[string]Counter, 8),
	}
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
	sorted := metricSlicePool.Get().([]metric)
	sorted = sorted[:0] // Reset length but keep capacity

	for key, value := range all {
		sorted = append(sorted, metric{
			Key:   key,
			Value: value,
		})
	}

	slices.SortFunc(sorted, func(a, b metric) int {
		return strings.Compare(a.Key, b.Key)
	})

	buf := stringSlicePool.Get().([]string)
	buf = buf[:0] // Reset length but keep capacity
	
	if cap(buf) < len(sorted) {
		buf = make([]string, 0, len(sorted))
	}
	
	for i := range sorted {
		buf = append(buf, fmt.Sprintf("%v:%v", sorted[i].Key, sorted[i].Value))
	}

	result := strings.Join(buf, " ")
	
	// Return slices to pool
	metricSlicePool.Put(sorted)
	stringSlicePool.Put(buf)
	
	return result
}

func (m *metrics) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.All())
}

func (m *metrics) Timer(name string) Timer {
	name = internMetricName(name) // Intern the metric name
	m.mtx.Lock()
	defer m.mtx.Unlock()
	t, ok := m.timers[name]
	if !ok {
		t = &timer{}
		m.timers[name] = t
	}
	return t
}

func (m *metrics) Histogram(name string) Histogram {
	name = internMetricName(name) // Intern the metric name
	m.mtx.Lock()
	defer m.mtx.Unlock()
	h, ok := m.histograms[name]
	if !ok {
		h = newHistogram()
		m.histograms[name] = h
	}
	return h
}

func (m *metrics) Counter(name string) Counter {
	name = internMetricName(name) // Intern the metric name
	m.mtx.Lock()
	defer m.mtx.Unlock()
	c, ok := m.counters[name]
	if !ok {
		zero := counter{}
		c = &zero
		m.counters[name] = c
	}
	return c
}

func (m *metrics) All() map[string]any {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	result := make(map[string]any, len(m.timers)+len(m.histograms)+len(m.counters))
	for name, timer := range m.timers {
		result[m.formatKey(name, timer)] = timer.Value()
	}
	for name, hist := range m.histograms {
		result[m.formatKey(name, hist)] = hist.Value()
	}
	for name, cntr := range m.counters {
		result[m.formatKey(name, cntr)] = cntr.Value()
	}
	return result
}

func (m *metrics) Timers() map[string]any {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	ts := make(map[string]any, len(m.timers))
	for n, t := range m.timers {
		ts[m.formatKey(n, t)] = t.Value()
	}
	return ts
}

func (m *metrics) Clear() {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	// Reuse existing maps instead of creating new ones
	for k := range m.timers {
		delete(m.timers, k)
	}
	for k := range m.histograms {
		delete(m.histograms, k)
	}
	for k := range m.counters {
		delete(m.counters, k)
	}
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
	mtx   sync.Mutex
	start time.Time
	value int64
}

func (t *timer) Start() {
	t.mtx.Lock()
	t.start = time.Now()
	t.mtx.Unlock()
}

func (t *timer) Stop() int64 {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	var delta int64
	if !t.start.IsZero() {
		// Add the delta to the accumulated time value so far.
		delta = time.Since(t.start).Nanoseconds()
		t.value += delta
		t.start = time.Time{} // Reset the start time to zero.
	}

	return delta
}

func (t *timer) Value() any {
	return t.Int64()
}

func (t *timer) Int64() int64 {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return t.value
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
	values := make(map[string]any, 12)
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
