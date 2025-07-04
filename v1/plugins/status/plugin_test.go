// Copyright 2018 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package status

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	inmem "github.com/IUAD1IY7/opa/v1/storage/inmem/test"
	"github.com/IUAD1IY7/opa/v1/metrics"
	"github.com/IUAD1IY7/opa/v1/plugins"
	"github.com/IUAD1IY7/opa/v1/plugins/bundle"
	lstat "github.com/IUAD1IY7/opa/v1/plugins/logs/status"
	"github.com/IUAD1IY7/opa/v1/util"
	"github.com/IUAD1IY7/opa/v1/util/test"
	"github.com/IUAD1IY7/opa/v1/version"
)

func TestMain(m *testing.M) {
	if version.Version == "" {
		version.Version = "unit-test"
	}
	os.Exit(m.Run())
}

func TestStatusUpdateBuffer(t *testing.T) {

	tests := []struct {
		name                  string
		numberOfStatusUpdates int
		expectedStatusUpdates int
		expectedNameDropped   string
	}{
		{
			name:                  "add multiple events dropping the oldest",
			numberOfStatusUpdates: 11,
			expectedStatusUpdates: 1,
			expectedNameDropped:   "0",
		},
		{
			name:                  "don't drop anything",
			numberOfStatusUpdates: 5,
			expectedStatusUpdates: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newTestFixture(t, nil)
			ctx := context.Background()

			err := fixture.plugin.Start(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer fixture.plugin.Stop(ctx)

			for i := range tc.numberOfStatusUpdates {
				s := bundle.Status{
					Name: strconv.Itoa(i),
				}
				fixture.plugin.BulkUpdateBundleStatus(map[string]*bundle.Status{
					"test": &s,
				})
			}

			if len(fixture.plugin.bulkBundleCh) != tc.expectedStatusUpdates {
				t.Fatalf("expected %d updates, got %d", tc.expectedStatusUpdates, len(fixture.plugin.bulkBundleCh))
			}
			for _, v := range <-fixture.plugin.bulkBundleCh {
				if v.Name == tc.expectedNameDropped {
					t.Fatalf("expected %s dropped", tc.expectedNameDropped)
				}
			}
		})
	}

}

func TestConfigValueParse(t *testing.T) {
	tests := []struct {
		note             string
		input            string
		expectedNoConfig bool
		expectedValue    []float64
	}{
		{
			note:             "empty config",
			input:            `{}`,
			expectedNoConfig: true,
		},
		{
			note:             "empty prometheus config",
			input:            `{"prometheus": false}`,
			expectedNoConfig: true,
		},
		{
			note:          "no specific prometheus config, expected default config buckets",
			input:         `{"prometheus": true}`,
			expectedValue: defaultBundleLoadStageBuckets,
		},
		{
			note:          "no specific prometheus config, expected default config buckets",
			input:         `{"prometheus": true, "prometheus_config": {}}`,
			expectedValue: defaultBundleLoadStageBuckets,
		},
		{
			note:          "no specific collectors config, expected default config buckets",
			input:         `{"prometheus": true, "prometheus_config": {"collectors": {}}}`,
			expectedValue: defaultBundleLoadStageBuckets,
		},
		{
			note:          "specified prometheus config, expected value same as config",
			input:         `{"prometheus": true, "prometheus_config": {"collectors": {"bundle_loading_duration_ns": {}}}}`,
			expectedValue: defaultBundleLoadStageBuckets,
		},
		{
			note:          "specified prometheus config, expected value same as config",
			input:         `{"prometheus": true, "prometheus_config": {"collectors": {"bundle_loading_duration_ns": {"buckets": []}}}}`,
			expectedValue: []float64{},
		},
		{
			note:          "specified prometheus config, expected value same as config",
			input:         `{"prometheus": true, "prometheus_config": {"collectors": {"bundle_loading_duration_ns": {"buckets":[1, 1000, 1000_000, 1e8]}}}}`,
			expectedValue: []float64{1, 1000, 1000_000, 1e8},
		},
	}

	for _, tc := range tests {
		t.Run(tc.note, func(t *testing.T) {
			config, err := ParseConfig([]byte(tc.input), []string{}, []string{"status"})
			if err != nil {
				t.Errorf("expected no error: %v", err)
			}
			if tc.expectedNoConfig && config != nil {
				t.Errorf("expected parsed config is nil, got %v", config)
			}
			if !tc.expectedNoConfig && !slices.Equal(config.PrometheusConfig.Collectors.BundleLoadDurationNanoseconds.Buckets, tc.expectedValue) {
				t.Errorf("expected %v, got %v", tc.expectedValue, config.PrometheusConfig.Collectors.BundleLoadDurationNanoseconds.Buckets)
			}
		})
	}
}

func TestPluginPrometheus(t *testing.T) {
	fixture := newTestFixture(t, nil, func(c *Config) {
		c.Prometheus = true
	})
	fixture.server.ch = make(chan UpdateRequestV1)
	defer fixture.server.stop()

	ctx := context.Background()

	err := fixture.plugin.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.plugin.Stop(ctx)
	<-fixture.server.ch

	status := testStatus()

	fixture.plugin.BulkUpdateBundleStatus(map[string]*bundle.Status{"bundle": status})
	<-fixture.server.ch

	registerMock := fixture.manager.PrometheusRegister().(*prometheusRegisterMock)

	assertOpInformationGauge(t, registerMock)

	if registerMock.Collectors[fixture.plugin.collectors.pluginStatus] != true {
		t.Fatalf("Plugin status metric was not registered on prometheus")
	}
	if registerMock.Collectors[fixture.plugin.collectors.loaded] != true {
		t.Fatalf("Loaded metric was not registered on prometheus")
	}
	if registerMock.Collectors[fixture.plugin.collectors.failLoad] != true {
		t.Fatalf("FailLoad metric was not registered on prometheus")
	}
	if registerMock.Collectors[fixture.plugin.collectors.lastRequest] != true {
		t.Fatalf("Last request metric was not registered on prometheus")
	}
	if registerMock.Collectors[fixture.plugin.collectors.lastSuccessfulActivation] != true {
		t.Fatalf("Last Successful Activation metric was not registered on prometheus")
	}
	if registerMock.Collectors[fixture.plugin.collectors.lastSuccessfulDownload] != true {
		t.Fatalf("Last Successful Download metric was not registered on prometheus")
	}
	if registerMock.Collectors[fixture.plugin.collectors.lastSuccessfulRequest] != true {
		t.Fatalf("Last Successful Request metric was not registered on prometheus")
	}
	if registerMock.Collectors[fixture.plugin.collectors.bundleLoadDuration] != true {
		t.Fatalf("Bundle Load Duration metric was not registered on prometheus")
	}
	if len(registerMock.Collectors) != 9 {
		t.Fatalf("Number of collectors expected (%v), got %v", 9, len(registerMock.Collectors))
	}

	lastRequestMetricResult := time.UnixMilli(int64(testutil.ToFloat64(fixture.plugin.collectors.lastRequest) / 1e6))
	if !lastRequestMetricResult.Equal(status.LastRequest) {
		t.Fatalf("Last request expected (%v), got %v", status.LastRequest.UTC(), lastRequestMetricResult.UTC())
	}

	lastSuccessfulRequestMetricResult := time.UnixMilli(int64(testutil.ToFloat64(fixture.plugin.collectors.lastSuccessfulRequest) / 1e6))
	if !lastSuccessfulRequestMetricResult.Equal(status.LastSuccessfulRequest) {
		t.Fatalf("Last request expected (%v), got %v", status.LastSuccessfulRequest.UTC(), lastSuccessfulRequestMetricResult.UTC())
	}

	lastSuccessfulDownloadMetricResult := time.UnixMilli(int64(testutil.ToFloat64(fixture.plugin.collectors.lastSuccessfulDownload) / 1e6))
	if !lastSuccessfulDownloadMetricResult.Equal(status.LastSuccessfulDownload) {
		t.Fatalf("Last request expected (%v), got %v", status.LastSuccessfulDownload.UTC(), lastSuccessfulDownloadMetricResult.UTC())
	}

	lastSuccessfulActivationMetricResult := time.UnixMilli(int64(testutil.ToFloat64(fixture.plugin.collectors.lastSuccessfulActivation) / 1e6))
	if !lastSuccessfulActivationMetricResult.Equal(status.LastSuccessfulActivation) {
		t.Fatalf("Last request expected (%v), got %v", status.LastSuccessfulActivation.UTC(), lastSuccessfulActivationMetricResult.UTC())
	}

	bundlesLoaded := testutil.CollectAndCount(fixture.plugin.collectors.loaded)
	if bundlesLoaded != 1 {
		t.Fatalf("Unexpected number of bundle loads (%v), got %v", 1, bundlesLoaded)
	}

	bundlesFailedToLoad := testutil.CollectAndCount(fixture.plugin.collectors.failLoad)
	if bundlesFailedToLoad != 0 {
		t.Fatalf("Unexpected number of bundle fails load (%v), got %v", 0, bundlesFailedToLoad)
	}

	pluginsStatus := testutil.CollectAndCount(fixture.plugin.collectors.pluginStatus)
	if pluginsStatus != 1 {
		t.Fatalf("Unexpected number of plugins (%v), got %v", 1, pluginsStatus)
	}

	// Assert that metrics are purged when prometheus is disabled
	prometheusDisabledConfig := newConfig(fixture.manager, func(c *Config) {
		c.Prometheus = false
	})
	fixture.plugin.Reconfigure(ctx, prometheusDisabledConfig)
	eventually(t, func() bool { return fixture.plugin.config.Prometheus == false })

	if len(registerMock.Collectors) != 0 {
		t.Fatalf("Number of collectors expected (%v), got %v", 0, len(registerMock.Collectors))
	}

	// Assert that metrics are re-registered when prometheus is re-enabled
	prometheusReenabledConfig := newConfig(fixture.manager, func(c *Config) {
		c.Prometheus = true
	})
	fixture.plugin.Reconfigure(ctx, prometheusReenabledConfig)
	eventually(t, func() bool { return fixture.plugin.config.Prometheus == true })

	if len(registerMock.Collectors) != 9 {
		t.Fatalf("Number of collectors expected (%v), got %v", 9, len(registerMock.Collectors))
	}
}

func eventually(t *testing.T, predicate func() bool) {
	t.Helper()
	if !test.Eventually(t, 1*time.Second, predicate) {
		t.Fatal("check took too long")
	}
}

func assertOpInformationGauge(t *testing.T, registerMock *prometheusRegisterMock) {
	gauges := filterGauges(registerMock)
	if len(gauges) != 1 {
		t.Fatalf("Expected one registered gauge on prometheus but got %v", len(gauges))
	}

	gauge := gauges[0]

	fqName := getName(gauge)
	if fqName != "opa_info" {
		t.Fatalf("Expected gauge to have name opa_info but was %s", fqName)
	}

	labels := getConstLabels(gauge)
	versionAct := labels["version"]
	if versionAct != version.Version {
		t.Fatalf("Expected gauge to have version label with value %s but was %s", version.Version, versionAct)
	}
}

func getName(gauge prometheus.Gauge) string {
	desc := reflect.Indirect(reflect.ValueOf(gauge.Desc()))
	fqName := desc.FieldByName("fqName").String()
	return fqName
}

func getConstLabels(gauge prometheus.Gauge) prometheus.Labels {
	desc := reflect.Indirect(reflect.ValueOf(gauge.Desc()))
	constLabelPairs := desc.FieldByName("constLabelPairs")

	// put all label pairs into a map for easier comparison.
	labels := make(prometheus.Labels, constLabelPairs.Len())
	for i := range constLabelPairs.Len() {
		name := constLabelPairs.Index(i).Elem().FieldByName("Name").Elem().String()
		value := constLabelPairs.Index(i).Elem().FieldByName("Value").Elem().String()
		labels[name] = value
	}
	return labels
}

func filterGauges(registerMock *prometheusRegisterMock) []prometheus.Gauge {
	fltd := make([]prometheus.Gauge, 0)

	for m := range registerMock.Collectors {
		switch metric := m.(type) {
		case prometheus.Gauge:
			fltd = append(fltd, metric)
		}
	}
	return fltd
}

func TestMetricsBundleWithoutRevision(t *testing.T) {
	fixture := newTestFixture(t, nil, func(c *Config) {
		c.Prometheus = true
	})
	fixture.server.ch = make(chan UpdateRequestV1)
	defer fixture.server.stop()

	ctx := context.Background()

	err := fixture.plugin.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.plugin.Stop(ctx)
	<-fixture.server.ch

	status := testStatus()
	status.ActiveRevision = ""

	fixture.plugin.BulkUpdateBundleStatus(map[string]*bundle.Status{"bundle": status})
	<-fixture.server.ch

	bundlesLoaded := testutil.CollectAndCount(fixture.plugin.collectors.loaded)
	if bundlesLoaded != 1 {
		t.Fatalf("Unexpected number of bundle loads (%v), got %v", 1, bundlesLoaded)
	}

	bundlesFailedToLoad := testutil.CollectAndCount(fixture.plugin.collectors.failLoad)
	if bundlesFailedToLoad != 0 {
		t.Fatalf("Unexpected number of bundle fails load (%v), got %v", 0, bundlesFailedToLoad)
	}
}

func TestPluginStart(t *testing.T) {

	fixture := newTestFixture(t, nil)
	fixture.server.ch = make(chan UpdateRequestV1)
	defer fixture.server.stop()

	ctx := context.Background()

	err := fixture.plugin.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.plugin.Stop(ctx)

	// Start will trigger a status update when the plugin state switches
	// from "not ready" to "ok".
	result := <-fixture.server.ch

	exp := UpdateRequestV1{
		Labels: map[string]string{
			"id":      "test-instance-id",
			"app":     "example-app",
			"version": version.Version,
		},
		Plugins: map[string]*plugins.Status{
			"status": {State: plugins.StateOK},
		},
	}

	if !result.Equal(exp) {
		t.Fatalf("Expected: %v but got: %v", exp, result)
	}

	status := testStatus()

	fixture.plugin.BulkUpdateBundleStatus(map[string]*bundle.Status{"test": status})
	result = <-fixture.server.ch

	exp.Bundles = map[string]*bundle.Status{"test": status}

	if !result.Equal(exp) {
		t.Fatalf("Expected: %v but got: %v", exp, result)
	}
}

func TestPluginNoLogging(t *testing.T) {
	// Given no custom plugin, no service(s) and no console logging configured,
	// this should not be an error, but neither do we need to initiate the plugin
	cases := []struct {
		note   string
		config []byte
	}{
		{
			note:   "no plugin attributes",
			config: []byte(`{}`),
		},
		{
			note:   "empty plugin configuration",
			config: []byte(`{"status": {}}`),
		},
		{
			note:   "only disabled console logger",
			config: []byte(`{"status": {"console": "false"}}`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.note, func(t *testing.T) {
			config, err := ParseConfig(tc.config, []string{}, nil)
			if err != nil {
				t.Errorf("expected no error: %v", err)
			}
			if config != nil {
				t.Errorf("excected no config for a no-op logging plugin")
			}
		})
	}
}

func TestPluginStartTriggerManualStart(t *testing.T) {

	fixture := newTestFixture(t, nil)
	fixture.server.ch = make(chan UpdateRequestV1)
	defer fixture.server.stop()

	ctx := context.Background()
	tr := plugins.TriggerManual
	fixture.plugin.config.Trigger = &tr

	// Start will trigger a status update when the plugin state switches
	// from "not ready" to "ok". This status update will be sent only after a manual trigger
	err := fixture.plugin.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.plugin.Stop(ctx)

	// trigger the status update
	go func() {
		_ = fixture.plugin.Trigger(ctx)
	}()

	result := <-fixture.server.ch

	exp := UpdateRequestV1{
		Labels: map[string]string{
			"id":      "test-instance-id",
			"app":     "example-app",
			"version": version.Version,
		},
	}

	if !maps.Equal(result.Labels, exp.Labels) {
		t.Fatalf("Expected: %v but got: %v", exp, result)
	}
}

func TestPluginStartTriggerManual(t *testing.T) {
	fixture := newTestFixture(t, nil)
	fixture.server.ch = make(chan UpdateRequestV1)
	defer fixture.server.stop()
	tr := plugins.TriggerManual
	fixture.plugin.config.Trigger = &tr

	status := testStatus()

	fixture.plugin.BulkUpdateBundleStatus(map[string]*bundle.Status{"test": status})

	statuses := <-fixture.plugin.bulkBundleCh
	fixture.plugin.lastBundleStatuses = statuses

	// trigger the status update
	go func() {
		_ = fixture.plugin.Trigger(context.Background())
	}()

	go func() {
		update := <-fixture.plugin.trigger
		err := fixture.plugin.oneShot(update.ctx)
		if err != nil {
			t.Error(err)
			return
		}
	}()

	result := <-fixture.server.ch

	exp := UpdateRequestV1{
		Labels: map[string]string{
			"id":      "test-instance-id",
			"app":     "example-app",
			"version": version.Version,
		},
	}
	exp.Bundles = map[string]*bundle.Status{"test": status}

	if !maps.EqualFunc(result.Bundles, exp.Bundles, (*bundle.Status).Equal) {
		t.Fatalf("Expected: %v but got: %v", exp, result)
	}
}

func TestPluginStartTriggerManualMultiple(t *testing.T) {

	fixture := newTestFixture(t, nil)
	fixture.server.ch = make(chan UpdateRequestV1)
	defer fixture.server.stop()

	ctx := context.Background()
	tr := plugins.TriggerManual
	fixture.plugin.config.Trigger = &tr

	err := fixture.plugin.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.plugin.Stop(ctx)

	status := testStatus()
	fixture.plugin.BulkUpdateBundleStatus(map[string]*bundle.Status{"test": status})
	fixture.plugin.UpdateDiscoveryStatus(*status)

	exp := UpdateRequestV1{
		Labels: map[string]string{
			"id":      "test-instance-id",
			"app":     "example-app",
			"version": version.Version,
		},
		Plugins: map[string]*plugins.Status{
			"status": {State: plugins.StateOK},
		},
	}

	// trigger the status update
	go func() {
		_ = fixture.plugin.Trigger(ctx)
	}()

	result := <-fixture.server.ch

	exp.Bundles = map[string]*bundle.Status{"test": status}
	exp.Discovery = status

	if !result.Equal(exp) {
		t.Fatalf("Expected: %v but got: %v", exp, result)
	}
}

func TestPluginStartTriggerManualWithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(3 * time.Second) // this should cause the context deadline to exceed
	}))

	managerConfig := fmt.Appendf(nil, `{
			"labels": {
				"app": "example-app"
			},
			"services": [
				{
					"name": "example",
					"url": %q
				}
			]}`, s.URL)

	manager, err := plugins.New(managerConfig, "test-instance-id", inmem.New())
	if err != nil {
		t.Fatal(err)
	}

	pluginConfig := []byte(`{
			"service": "example",
		}`)

	config, _ := ParseConfig(pluginConfig, manager.Services(), nil)
	tr := plugins.TriggerManual
	config.Trigger = &tr

	p := New(config, manager)

	// Start will trigger a status update when the plugin state switches
	// from "not ready" to "ok". This status update will be sent only after a manual trigger
	err = p.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop(ctx)

	// trigger the status update
	done := make(chan struct{})
	go func() {
		// this call should block till the context deadline exceeds
		_ = p.Trigger(ctx)
		close(done)
	}()
	<-done

	if ctx.Err() == nil {
		t.Fatal("Expected error but got nil")
	}

	exp := "context deadline exceeded"
	if ctx.Err().Error() != exp {
		t.Fatalf("Expected error %v but got %v", exp, ctx.Err().Error())
	}
}

func TestPluginStartTriggerManualWithError(t *testing.T) {
	ctx := context.Background()

	managerConfig := []byte(`{
			"labels": {
				"app": "example-app"
			},
			"services": [
				{
					"name": "example",
					"url": "http://localhost:12345"
				}
			]}`)

	manager, err := plugins.New(managerConfig, "test-instance-id", inmem.New())
	if err != nil {
		t.Fatal(err)
	}

	pluginConfig := []byte(`{
			"service": "example",
		}`)

	config, _ := ParseConfig(pluginConfig, manager.Services(), nil)
	tr := plugins.TriggerManual
	config.Trigger = &tr

	p := New(config, manager)

	// Start will trigger a status update when the plugin state switches
	// from "not ready" to "ok". This status update will be sent only after a manual trigger
	err = p.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop(ctx)

	// trigger the status update
	// this call should result in an error from the bad service config
	err = p.Trigger(ctx)
	if err == nil {
		t.Fatal("Expected error but got nil")
	}

	exp := "connection refused"
	if !strings.Contains(err.Error(), exp) {
		t.Fatalf("Unexpected error message %v", err.Error())
	}
}

func TestPluginStartBulkUpdate(t *testing.T) {

	fixture := newTestFixture(t, nil)
	fixture.server.ch = make(chan UpdateRequestV1)
	defer fixture.server.stop()

	ctx := context.Background()

	err := fixture.plugin.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.plugin.Stop(ctx)

	// Start will trigger a status update when the plugin state switches
	// from "not ready" to "ok".
	<-fixture.server.ch // Discard first request.

	exp := UpdateRequestV1{
		Labels: map[string]string{
			"id":      "test-instance-id",
			"app":     "example-app",
			"version": version.Version,
		},
		Plugins: map[string]*plugins.Status{
			"status": {State: plugins.StateOK},
		},
	}

	status := testStatus()

	fixture.plugin.BulkUpdateBundleStatus(map[string]*bundle.Status{status.Name: status})
	result := <-fixture.server.ch

	exp.Bundles = map[string]*bundle.Status{status.Name: status}

	if !result.Equal(exp) {
		t.Fatalf("Expected: %v but got: %v", exp, result)
	}
}

func TestPluginStartBulkUpdateMultiple(t *testing.T) {

	fixture := newTestFixture(t, nil)
	fixture.server.ch = make(chan UpdateRequestV1)
	defer fixture.server.stop()

	ctx := context.Background()

	err := fixture.plugin.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.plugin.Stop(ctx)

	// Ignore the plugin updating its status (tested elsewhere)
	<-fixture.server.ch

	statuses := map[string]*bundle.Status{}
	tDownload, _ := time.Parse(time.RFC3339Nano, "2018-01-01T00:00:00.0000000Z")
	tActivate, _ := time.Parse(time.RFC3339Nano, "2018-01-01T00:00:01.0000000Z")
	for i := range 20 {
		name := fmt.Sprintf("test-bundle-%d", i)
		statuses[name] = &bundle.Status{
			Name:                     name,
			ActiveRevision:           fmt.Sprintf("v%d", i),
			LastSuccessfulDownload:   tDownload,
			LastSuccessfulActivation: tActivate,
		}
	}

	fixture.plugin.BulkUpdateBundleStatus(statuses)
	result := <-fixture.server.ch

	expLabels := map[string]string{
		"id":      "test-instance-id",
		"app":     "example-app",
		"version": version.Version,
	}

	if !maps.Equal(result.Labels, expLabels) {
		t.Fatalf("Unexpected status labels: %+v", result.Labels)
	}

	if len(result.Bundles) != len(statuses) {
		t.Fatalf("Expected %d statuses, got %d", len(statuses), len(result.Bundles))
	}

	for name, s := range statuses {
		actualStatus := result.Bundles[name]
		if actualStatus.Name != s.Name ||
			actualStatus.LastSuccessfulActivation != s.LastSuccessfulActivation ||
			actualStatus.LastSuccessfulDownload != s.LastSuccessfulDownload ||
			actualStatus.ActiveRevision != s.ActiveRevision {
			t.Errorf("Bundle %s has unexpected status:\n\n %v\n\nExpected:\n%v\n\n", name, actualStatus, s)
		}
	}
}

func TestPluginStartDiscovery(t *testing.T) {

	fixture := newTestFixture(t, nil)
	fixture.server.ch = make(chan UpdateRequestV1)
	defer fixture.server.stop()

	ctx := context.Background()

	err := fixture.plugin.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.plugin.Stop(ctx)

	// Ignore the plugin updating its status (tested elsewhere)
	<-fixture.server.ch

	status := testStatus()

	fixture.plugin.UpdateDiscoveryStatus(*status)
	result := <-fixture.server.ch

	exp := UpdateRequestV1{
		Labels: map[string]string{
			"id":      "test-instance-id",
			"app":     "example-app",
			"version": version.Version,
		},
		Discovery: status,
		Plugins: map[string]*plugins.Status{
			"status": {State: plugins.StateOK},
		},
	}

	if !result.Equal(exp) {
		t.Fatalf("Expected: %+v but got: %+v", exp, result)
	}
}

func TestPluginStartDecisionLogs(t *testing.T) {

	fixture := newTestFixture(t, nil)
	fixture.server.ch = make(chan UpdateRequestV1)
	defer fixture.server.stop()

	ctx := context.Background()

	err := fixture.plugin.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.plugin.Stop(ctx)

	// Ignore the plugin updating its status (tested elsewhere)
	<-fixture.server.ch

	status := &lstat.Status{
		Code:     "decision_log_error",
		Message:  "Upload Failed",
		HTTPCode: "400",
	}

	fixture.plugin.UpdateDecisionLogsStatus(*status)
	result := <-fixture.server.ch

	exp := UpdateRequestV1{
		Labels: map[string]string{
			"id":      "test-instance-id",
			"app":     "example-app",
			"version": version.Version,
		},
		DecisionLogs: status,
		Plugins: map[string]*plugins.Status{
			"status": {State: plugins.StateOK},
		},
	}

	if !result.Equal(exp) {
		t.Fatalf("Expected: %+v but got: %+v", exp, result)
	}
}

func TestPluginBadAuth(t *testing.T) {
	fixture := newTestFixture(t, nil)
	ctx := context.Background()
	fixture.server.expCode = 401
	defer fixture.server.stop()
	fixture.plugin.lastBundleStatuses = map[string]*bundle.Status{}
	err := fixture.plugin.oneShot(ctx)
	if err == nil {
		t.Fatal("Expected error")
	}
	if err.Error() != "status update failed, server replied with HTTP 401 Unauthorized" {
		t.Fatalf("Unexpected error contents: %v", err)
	}
}

func TestPluginBadPath(t *testing.T) {
	fixture := newTestFixture(t, nil)
	ctx := context.Background()
	fixture.server.expCode = 404
	defer fixture.server.stop()
	fixture.plugin.lastBundleStatuses = map[string]*bundle.Status{}
	err := fixture.plugin.oneShot(ctx)
	if err == nil {
		t.Fatal("Expected error")
	}
	if err.Error() != "status update failed, server replied with HTTP 404 Not Found" {
		t.Fatalf("Unexpected error contents: %v", err)
	}
}

func TestPluginBadStatus(t *testing.T) {
	fixture := newTestFixture(t, nil)
	ctx := context.Background()
	fixture.server.expCode = 500
	defer fixture.server.stop()
	fixture.plugin.lastBundleStatuses = map[string]*bundle.Status{}
	err := fixture.plugin.oneShot(ctx)
	if err == nil {
		t.Fatal("Expected error")
	}
	if err.Error() != "status update failed, server replied with HTTP 500 Internal Server Error" {
		t.Fatalf("Unexpected error contents: %v", err)
	}
}

func TestPluginNonstandardStatus(t *testing.T) {
	fixture := newTestFixture(t, nil)
	ctx := context.Background()
	fixture.server.expCode = 599
	defer fixture.server.stop()
	fixture.plugin.lastBundleStatuses = map[string]*bundle.Status{}
	err := fixture.plugin.oneShot(ctx)
	if err == nil {
		t.Fatal("Expected error")
	}
	if err.Error() != "status update failed, server replied with HTTP 599 " {
		t.Fatalf("Unexpected error contents: %v", err)
	}
}

func TestPlugin2xxStatus(t *testing.T) {
	fixture := newTestFixture(t, nil)
	ctx := context.Background()
	fixture.server.expCode = 204
	defer fixture.server.stop()
	fixture.plugin.lastBundleStatuses = map[string]*bundle.Status{}
	err := fixture.plugin.oneShot(ctx)
	if err != nil {
		t.Fatal("Expected no error")
	}
}

func TestPluginReconfigure(t *testing.T) {
	ctx := context.Background()
	fixture := newTestFixture(t, nil, func(c *Config) {
		c.Prometheus = true
	})
	defer fixture.server.stop()

	if err := fixture.plugin.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// expect initial prometheus config is filled with default BundleLoadDurationNanoseconds bucket
	initialPrometheusConfig := &PrometheusConfig{
		Collectors: &Collectors{
			BundleLoadDurationNanoseconds: &BundleLoadDurationNanoseconds{
				Buckets: defaultBundleLoadStageBuckets,
			},
		},
	}
	if !reflect.DeepEqual(fixture.plugin.config.PrometheusConfig, initialPrometheusConfig) {
		t.Fatalf("Expected initial prometheus config %+v: test but got %+v", initialPrometheusConfig, fixture.plugin.config.PrometheusConfig)
	}

	pluginConfig := []byte(`{
			"service": "example",
			"partition_name": "test",
			"prometheus": true,
			"prometheus_config": {
				"collectors": {
					"bundle_loading_duration_ns": {
						"buckets":[1, 1000, 1000_000, 1e8]
					}
				}
			}
		}`)

	config, _ := ParseConfig(pluginConfig, fixture.manager.Services(), nil)

	fixture.plugin.Reconfigure(ctx, config)
	fixture.plugin.Stop(ctx)

	// after reconfigure, expect partition name and prometheus config are updated
	if fixture.plugin.config.PartitionName != "test" {
		t.Fatalf("Expected partition name: test but got %v", fixture.plugin.config.PartitionName)
	}

	expectedPromConfig := &PrometheusConfig{
		Collectors: &Collectors{
			BundleLoadDurationNanoseconds: &BundleLoadDurationNanoseconds{
				Buckets: []float64{1, 1000, 1000_000, 1e8},
			},
		},
	}
	if !reflect.DeepEqual(fixture.plugin.config.PrometheusConfig, expectedPromConfig) {
		t.Fatalf("Expected prometheus config %+v: test but got %+v", expectedPromConfig, fixture.plugin.config.PrometheusConfig)
	}
}

func TestMetrics(t *testing.T) {
	fixture := newTestFixture(t, metrics.New())
	fixture.server.ch = make(chan UpdateRequestV1)
	defer fixture.server.stop()

	ctx := context.Background()

	err := fixture.plugin.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.plugin.Stop(ctx)

	// Ignore the plugin updating its status (tested elsewhere)
	<-fixture.server.ch

	status := testStatus()

	fixture.plugin.BulkUpdateBundleStatus(map[string]*bundle.Status{"bundle": status})
	result := <-fixture.server.ch

	exp := map[string]any{"<built-in>": map[string]any{}}

	if !reflect.DeepEqual(result.Metrics, exp) {
		t.Fatalf("Expected %v but got %v", exp, result.Metrics)
	}
}

func TestParseConfigUseDefaultServiceNoConsole(t *testing.T) {
	services := []string{
		"s0",
		"s1",
		"s3",
	}

	loggerConfig := []byte(`{
		"console": false
	}`)

	config, err := ParseConfig(loggerConfig, services, nil)

	if err != nil {
		t.Errorf("Unexpected error: %s", err)
	}

	if config.Service != services[0] {
		t.Errorf("Expected %s service in config, actual = '%s'", services[0], config.Service)
	}
}

func TestParseConfigDefaultServiceWithConsole(t *testing.T) {
	services := []string{
		"s0",
		"s1",
		"s3",
	}

	loggerConfig := []byte(`{
		"console": true
	}`)

	config, err := ParseConfig(loggerConfig, services, nil)

	if err != nil {
		t.Errorf("Unexpected error: %s", err)
	}

	if config.Service != "" {
		t.Errorf("Expected no service in config, actual = '%s'", config.Service)
	}
}

func TestParseConfigTriggerMode(t *testing.T) {
	cases := []struct {
		note     string
		config   []byte
		expected plugins.TriggerMode
		wantErr  bool
		err      error
	}{
		{
			note:     "default trigger mode",
			config:   []byte(`{}`),
			expected: plugins.DefaultTriggerMode,
		},
		{
			note:     "manual trigger mode",
			config:   []byte(`{"trigger": "manual"}`),
			expected: plugins.TriggerManual,
		},
		{
			note:     "trigger mode mismatch",
			config:   []byte(`{"trigger": "manual"}`),
			expected: plugins.TriggerPeriodic,
			wantErr:  true,
			err:      errors.New("invalid status config: trigger mode mismatch: periodic and manual (hint: check discovery configuration)"),
		},
		{
			note:     "bad trigger mode",
			config:   []byte(`{"trigger": "foo"}`),
			expected: "foo",
			wantErr:  true,
			err:      errors.New("invalid status config: invalid trigger mode \"foo\" (want \"periodic\" or \"manual\")"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.note, func(t *testing.T) {
			c, err := NewConfigBuilder().WithBytes(tc.config).WithServices([]string{"s0"}).WithTriggerMode(&tc.expected).Parse()

			if tc.wantErr {
				if err == nil {
					t.Fatal("Expected error but got nil")
				}

				if tc.err != nil && tc.err.Error() != err.Error() {
					t.Fatalf("Expected error message %v but got %v", tc.err.Error(), err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error %v", err)
				}

				if *c.Trigger != tc.expected {
					t.Fatalf("Expected trigger mode %v but got %v", tc.expected, *c.Trigger)
				}
			}
		})
	}
}

type testFixture struct {
	manager *plugins.Manager
	plugin  *Plugin
	server  *testServer
}

type testPluginCustomizer func(c *Config)

func newPlugin(t *testing.T, url string, m metrics.Metrics, options ...testPluginCustomizer) (*plugins.Manager, *Plugin) {

	managerConfig := fmt.Appendf(nil, `{
			"labels": {
				"app": "example-app"
			},
			"services": [
				{
					"name": "example",
					"url": %q,
					"credentials": {
						"bearer": {
							"scheme": "Bearer",
							"token": "secret"
						}
					}
				}
			]}`, url)

	registerMock := &prometheusRegisterMock{
		Collectors: map[prometheus.Collector]bool{},
	}
	manager, err := plugins.New(managerConfig, "test-instance-id", inmem.New(), plugins.WithPrometheusRegister(registerMock))
	if err != nil {
		t.Fatal(err)
	}

	config := newConfig(manager, options...)

	p := New(config, manager).WithMetrics(m)

	return manager, p
}

func newTestFixture(t *testing.T, m metrics.Metrics, options ...testPluginCustomizer) testFixture {

	ts := testServer{
		t:       t,
		expCode: 200,
	}

	ts.start()

	manager, p := newPlugin(t, ts.server.URL, m, options...)

	return testFixture{
		manager: manager,
		plugin:  p,
		server:  &ts,
	}

}

func newConfig(manager *plugins.Manager, options ...testPluginCustomizer) *Config {
	pluginConfig := []byte(`{
			"service": "example",
		}`)

	config, _ := ParseConfig(pluginConfig, manager.Services(), nil)
	for _, option := range options {
		option(config)
	}

	return config
}

type testServer struct {
	t       *testing.T
	expCode int
	server  *httptest.Server
	ch      chan UpdateRequestV1
}

func (t *testServer) handle(w http.ResponseWriter, r *http.Request) {

	status := UpdateRequestV1{}

	if err := util.NewJSONDecoder(r.Body).Decode(&status); err != nil {
		t.t.Fatal(err)
	}

	if t.ch != nil {
		t.ch <- status
	}

	w.WriteHeader(t.expCode)
}

func (t *testServer) start() {
	t.server = httptest.NewServer(http.HandlerFunc(t.handle))
}

func (t *testServer) stop() {
	t.server.Close()
}

func testStatus() *bundle.Status {
	tDownload, _ := time.Parse(time.RFC3339Nano, "2018-01-01T00:00:00.0000000Z")
	tActivate, _ := time.Parse(time.RFC3339Nano, "2018-01-01T00:00:01.0000000Z")
	tSuccessfulRequest, _ := time.Parse(time.RFC3339Nano, "2018-01-01T00:00:02.0000000Z")
	tRequest, _ := time.Parse(time.RFC3339Nano, "2018-01-01T00:00:03.0000000Z")

	status := bundle.Status{
		Name:                     "example/authz",
		ActiveRevision:           "quickbrawnfaux",
		LastSuccessfulDownload:   tDownload,
		LastSuccessfulActivation: tActivate,
		LastRequest:              tRequest,
		LastSuccessfulRequest:    tSuccessfulRequest,
	}

	return &status
}

type testPlugin struct {
	reqs []UpdateRequestV1
}

func (*testPlugin) Start(context.Context) error {
	return nil
}

func (*testPlugin) Stop(context.Context) {
}

func (*testPlugin) Reconfigure(context.Context, any) {
}

func (p *testPlugin) Log(_ context.Context, req *UpdateRequestV1) error {
	p.reqs = append(p.reqs, *req)
	return nil
}

func TestPluginCustomBackend(t *testing.T) {
	ctx := context.Background()
	manager, _ := plugins.New(nil, "test-instance-id", inmem.New())

	backend := &testPlugin{}
	manager.Register("test_plugin", backend)

	config, err := ParseConfig([]byte(`{"plugin": "test_plugin"}`), nil, []string{"test_plugin"})
	if err != nil {
		t.Fatal(err)
	}

	plugin := New(config, manager)
	err = plugin.oneShot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = plugin.oneShot(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(backend.reqs) != 2 {
		t.Fatalf("Unexpected number of reqs: expected 2, got %d: %v", len(backend.reqs), backend.reqs)
	}
}

type prometheusRegisterMock struct {
	Collectors map[prometheus.Collector]bool
}

func (p prometheusRegisterMock) Register(collector prometheus.Collector) error {
	p.Collectors[collector] = true
	return nil
}

func (p prometheusRegisterMock) MustRegister(collector ...prometheus.Collector) {
	for _, c := range collector {
		p.Collectors[c] = true
	}
}

func (p prometheusRegisterMock) Unregister(collector prometheus.Collector) bool {
	delete(p.Collectors, collector)
	return true
}

func TestPluginTerminatesAfterGracefulShutdownPeriodWithoutStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	fixture := newTestFixture(t, nil)
	defer fixture.server.stop()

	if err := fixture.plugin.Start(ctx); err != nil {
		t.Fatal(err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	fixture.plugin.Stop(timeoutCtx)
	if timeoutCtx.Err() != nil {
		t.Fatal("Stop did not exit before context expiration")
	}
}

func TestPluginTerminatesAfterGracefulShutdownPeriodWithStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	fixture := newTestFixture(t, nil)
	fixture.server.ch = make(chan UpdateRequestV1, 1)
	defer fixture.server.stop()

	// simplified status loop just to return done
	// but doesn't read from the channels
	go func() {
		for done := range fixture.plugin.stop {
			done <- struct{}{}
			return
		}
	}()

	status := testStatus()
	fixture.plugin.BulkUpdateBundleStatus(map[string]*bundle.Status{status.Name: status})

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	fixture.plugin.Stop(timeoutCtx)
	if timeoutCtx.Err() != nil {
		t.Fatal("Stop did not exit before context expiration")
	}

	result := <-fixture.server.ch

	exp := UpdateRequestV1{
		Labels: map[string]string{
			"id":      "test-instance-id",
			"app":     "example-app",
			"version": version.Version,
		},
		Bundles: map[string]*bundle.Status{status.Name: status},
	}

	if !result.Equal(exp) {
		t.Fatalf("Expected: %v but got: %v", exp, result)
	}
}

func TestSlowServer(t *testing.T) {
	t.Parallel()

	received := make(chan struct{})
	wait := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// notify test the server got the request
		received <- struct{}{}

		// block until the test is ready to move on, so that multiple status updates can be sent
		<-wait
	}))
	defer server.Close()

	_, plugin := newPlugin(t, server.URL, nil)

	// just start the loop, calling Start will also send a plugin status update that isn't needed for this test
	go plugin.loop(context.Background())

	status := bundle.Status{
		Name: "test",
	}
	plugin.BulkUpdateBundleStatus(map[string]*bundle.Status{status.Name: &status})

	// wait for server to get stuck
	<-received

	if len(plugin.bulkBundleCh) != 0 {
		t.Fatalf("Unexpected bulk bundle status: %v", plugin.bulkBundleCh)
	}

	status = bundle.Status{
		Name: "I will be dropped",
	}
	plugin.BulkUpdateBundleStatus(map[string]*bundle.Status{status.Name: &status})
	expectedStatusName := "I won't be dropped"
	status = bundle.Status{
		Name: expectedStatusName,
	}
	plugin.BulkUpdateBundleStatus(map[string]*bundle.Status{status.Name: &status})

	currentStatus := <-plugin.bulkBundleCh
	if len(currentStatus) > 1 {
		t.Fatalf("Only one status expected but got: %v", currentStatus)
	}
	if _, ok := currentStatus[expectedStatusName]; !ok {
		t.Fatalf("Expected status name not found: %v", currentStatus)
	}

	// stop blocking server
	wait <- struct{}{}
}
