// Copyright 2018 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

// Package status implements status reporting.
package status

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"slices"

	lstat "github.com/IUAD1IY7/opa/v1/plugins/logs/status"

	"github.com/IUAD1IY7/opa/v1/logging"
	"github.com/IUAD1IY7/opa/v1/metrics"
	"github.com/IUAD1IY7/opa/v1/plugins"
	"github.com/IUAD1IY7/opa/v1/plugins/bundle"
	"github.com/IUAD1IY7/opa/v1/util"
)

const (
	statusBufferLimit           = int64(1)
	statusBufferDropCounterName = "status_dropped_buffer_limit_exceeded"
)

// Logger defines the interface for status plugins.
type Logger interface {
	plugins.Plugin

	Log(context.Context, *UpdateRequestV1) error
}

// UpdateRequestV1 represents the status update message that OPA sends to
// remote HTTP endpoints.
type UpdateRequestV1 struct {
	Labels       map[string]string          `json:"labels"`
	Bundle       *bundle.Status             `json:"bundle,omitempty"` // Deprecated: Use bulk `bundles` status updates instead
	Bundles      map[string]*bundle.Status  `json:"bundles,omitempty"`
	Discovery    *bundle.Status             `json:"discovery,omitempty"`
	DecisionLogs *lstat.Status              `json:"decision_logs,omitempty"`
	Metrics      map[string]any             `json:"metrics,omitempty"`
	Plugins      map[string]*plugins.Status `json:"plugins,omitempty"`
}

// Plugin implements status reporting. Updates can be triggered by the caller.
type Plugin struct {
	manager                *plugins.Manager
	config                 Config
	bundleCh               chan bundle.Status // Deprecated: Use bulk bundle status updates instead
	lastBundleStatus       *bundle.Status     // Deprecated: Use bulk bundle status updates instead
	bulkBundleCh           chan map[string]*bundle.Status
	lastBundleStatuses     map[string]*bundle.Status
	discoCh                chan bundle.Status
	lastDiscoStatus        *bundle.Status
	pluginStatusCh         chan map[string]*plugins.Status
	decisionLogsCh         chan lstat.Status
	lastDecisionLogsStatus *lstat.Status
	lastPluginStatuses     map[string]*plugins.Status
	queryCh                chan chan *UpdateRequestV1
	stop                   chan chan struct{}
	reconfig               chan reconfigure
	metrics                metrics.Metrics
	logger                 logging.Logger
	trigger                chan trigger
	collectors             *collectors
}

// Config contains configuration for the plugin.
type Config struct {
	Plugin           *string              `json:"plugin"`
	Service          string               `json:"service"`
	PartitionName    string               `json:"partition_name,omitempty"`
	ConsoleLogs      bool                 `json:"console"`
	Prometheus       bool                 `json:"prometheus"`
	PrometheusConfig *PrometheusConfig    `json:"prometheus_config,omitempty"`
	Trigger          *plugins.TriggerMode `json:"trigger,omitempty"` // trigger mode
}

// BundleLoadDurationNanoseconds represents the configuration for the status.prometheus_config.bundle_loading_duration_ns settings
type BundleLoadDurationNanoseconds struct {
	Buckets []float64 `json:"buckets,omitempty"` // the float64 array of buckets representing nanoseconds or multiple of nanoseconds
}

type reconfigure struct {
	config any
	done   chan struct{}
}

type trigger struct {
	ctx  context.Context
	done chan error
}

func (c *Config) validateAndInjectDefaults(services []string, pluginsList []string, trigger *plugins.TriggerMode) error {
	if c.Plugin != nil && !slices.Contains(pluginsList, *c.Plugin) {
		return fmt.Errorf("invalid plugin name %q in status", *c.Plugin)
	} else if c.Service == "" && len(services) != 0 && !(c.ConsoleLogs || c.Prometheus) {
		// For backwards compatibility allow defaulting to the first
		// service listed, but only if console logging is disabled. If enabled
		// we can't tell if the deployer wanted to use only console logs or
		// both console logs and the default service option.
		c.Service = services[0]
	} else if c.Service != "" && !slices.Contains(services, c.Service) {
		return fmt.Errorf("invalid service name %q in status", c.Service)
	}

	t, err := plugins.ValidateAndInjectDefaultsForTriggerMode(trigger, c.Trigger)
	if err != nil {
		return fmt.Errorf("invalid status config: %w", err)
	}
	c.Trigger = t

	c.PrometheusConfig = injectDefaultDurationBuckets(c.PrometheusConfig)

	return nil
}

// ParseConfig validates the config and injects default values.
func ParseConfig(config []byte, services []string, pluginsList []string) (*Config, error) {
	t := plugins.DefaultTriggerMode
	return NewConfigBuilder().WithBytes(config).WithServices(services).WithPlugins(pluginsList).WithTriggerMode(&t).Parse()
}

// ConfigBuilder assists in the construction of the plugin configuration.
type ConfigBuilder struct {
	raw      []byte
	services []string
	plugins  []string
	trigger  *plugins.TriggerMode
}

// NewConfigBuilder returns a new ConfigBuilder to build and parse the plugin config.
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{}
}

// WithBytes sets the raw plugin config.
func (b *ConfigBuilder) WithBytes(config []byte) *ConfigBuilder {
	b.raw = config
	return b
}

// WithServices sets the services that implement control plane APIs.
func (b *ConfigBuilder) WithServices(services []string) *ConfigBuilder {
	b.services = services
	return b
}

// WithPlugins sets the list of named plugins for status updates.
func (b *ConfigBuilder) WithPlugins(plugins []string) *ConfigBuilder {
	b.plugins = plugins
	return b
}

// WithTriggerMode sets the plugin trigger mode.
func (b *ConfigBuilder) WithTriggerMode(trigger *plugins.TriggerMode) *ConfigBuilder {
	b.trigger = trigger
	return b
}

// Parse validates the config and injects default values.
func (b *ConfigBuilder) Parse() (*Config, error) {
	if b.raw == nil {
		return nil, nil
	}

	var parsedConfig Config

	if err := util.Unmarshal(b.raw, &parsedConfig); err != nil {
		return nil, err
	}

	if parsedConfig.Plugin == nil && parsedConfig.Service == "" && len(b.services) == 0 && !parsedConfig.ConsoleLogs && !parsedConfig.Prometheus {
		// Nothing to validate or inject
		return nil, nil
	}

	if err := parsedConfig.validateAndInjectDefaults(b.services, b.plugins, b.trigger); err != nil {
		return nil, err
	}

	return &parsedConfig, nil
}

// New returns a new Plugin with the given config.
func New(parsedConfig *Config, manager *plugins.Manager) *Plugin {
	p := &Plugin{
		manager:        manager,
		config:         *parsedConfig,
		bundleCh:       make(chan bundle.Status, statusBufferLimit),
		bulkBundleCh:   make(chan map[string]*bundle.Status, statusBufferLimit),
		discoCh:        make(chan bundle.Status),
		decisionLogsCh: make(chan lstat.Status),
		stop:           make(chan chan struct{}),
		reconfig:       make(chan reconfigure),
		// we use a buffered channel here to avoid blocking other plugins
		// when updating statuses
		pluginStatusCh: make(chan map[string]*plugins.Status, statusBufferLimit),
		queryCh:        make(chan chan *UpdateRequestV1),
		logger:         manager.Logger().WithFields(map[string]any{"plugin": Name}),
		trigger:        make(chan trigger),
		collectors:     newCollectors(parsedConfig.PrometheusConfig),
	}

	p.manager.UpdatePluginStatus(Name, &plugins.Status{State: plugins.StateNotReady})

	return p
}

// WithMetrics sets the global metrics provider to be used by the plugin.
func (p *Plugin) WithMetrics(m metrics.Metrics) *Plugin {
	p.metrics = m
	return p
}

// Name identifies the plugin on manager.
const Name = "status"

// Lookup returns the status plugin registered with the manager.
func Lookup(manager *plugins.Manager) *Plugin {
	if p := manager.Plugin(Name); p != nil {
		return p.(*Plugin)
	}
	return nil
}

// Start starts the plugin.
func (p *Plugin) Start(ctx context.Context) error {
	p.logger.Info("Starting status reporter.")

	go p.loop(ctx)

	// Setup a listener for plugin statuses, but only after starting the loop
	// to prevent blocking threads pushing the plugin updates.
	p.manager.RegisterPluginStatusListener(Name, p.UpdatePluginStatus)

	if p.config.Prometheus {
		p.collectors.RegisterAll(p.manager.PrometheusRegister(), p.logger)
	}

	// Set the status plugin's status to OK now that everything is registered and
	// the loop is running. This will trigger an update on the listener with the
	// current status of all the other plugins too.
	p.manager.UpdatePluginStatus(Name, &plugins.Status{State: plugins.StateOK})
	return nil
}

// Stop stops the plugin.
func (p *Plugin) Stop(ctx context.Context) {
	p.logger.Info("Stopping status reporter.")

	done := make(chan struct{})

	// stop the status plugin loop and flush any pending status updates
	go func() {
		p.manager.UnregisterPluginStatusListener(Name)
		d := make(chan struct{})
		p.stop <- d
		<-d
		p.flush(ctx)
		done <- struct{}{}
	}()

	// wait for status plugin to shut down gracefully or timeout
	select {
	case <-done:
		p.manager.UpdatePluginStatus(Name, &plugins.Status{State: plugins.StateNotReady})
	case <-ctx.Done():
		switch ctx.Err() {
		case context.DeadlineExceeded, context.Canceled:
			p.logger.Error("Status Plugin stopped with statuses possibly not sent.")
		}
	}
}

func (p *Plugin) flush(ctx context.Context) {
	if !p.readBundleStatus() {
		return
	}

	err := p.oneShot(ctx)
	if err != nil {
		p.logger.Error("%v.", err)
	} else {
		p.logger.Info("Final status update sent successfully.")
	}
}

// UpdateBundleStatus notifies the plugin that the policy bundle was updated.
// Deprecated: Use BulkUpdateBundleStatus instead.
func (p *Plugin) UpdateBundleStatus(status bundle.Status) {
	util.PushFIFO(p.bundleCh, status, p.metrics, statusBufferDropCounterName)
}

// BulkUpdateBundleStatus notifies the plugin that the policy bundle was updated.
func (p *Plugin) BulkUpdateBundleStatus(status map[string]*bundle.Status) {
	util.PushFIFO(p.bulkBundleCh, status, p.metrics, statusBufferDropCounterName)
}

// UpdateDiscoveryStatus notifies the plugin that the discovery bundle was updated.
func (p *Plugin) UpdateDiscoveryStatus(status bundle.Status) {
	p.discoCh <- status
}

// UpdateDecisionLogsStatus notifies the plugin that status of a decision log upload event.
func (p *Plugin) UpdateDecisionLogsStatus(status lstat.Status) {
	p.decisionLogsCh <- status
}

// UpdatePluginStatus notifies the plugin that a plugin status was updated.
func (p *Plugin) UpdatePluginStatus(status map[string]*plugins.Status) {
	p.pluginStatusCh <- status
}

// Reconfigure notifies the plugin with a new configuration.
func (p *Plugin) Reconfigure(_ context.Context, config any) {
	done := make(chan struct{})
	p.reconfig <- reconfigure{config: config, done: done}
	<-done
}

// Snapshot returns the current status.
func (p *Plugin) Snapshot() *UpdateRequestV1 {
	ch := make(chan *UpdateRequestV1)
	p.queryCh <- ch
	s := <-ch
	return s
}

// Trigger can be used to control when the plugin attempts to upload
// status in manual triggering mode.
func (p *Plugin) Trigger(ctx context.Context) error {
	done := make(chan error)
	p.trigger <- trigger{ctx: ctx, done: done}

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Plugin) loop(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)

	for {
		select {
		case statuses := <-p.pluginStatusCh:
			p.lastPluginStatuses = statuses
			if *p.config.Trigger == plugins.TriggerPeriodic {
				err := p.oneShot(ctx)
				if err != nil {
					p.logger.Error("%v.", err)
				} else {
					p.logger.Info("Status update sent successfully in response to plugin update.")
				}
			}

		case statuses := <-p.bulkBundleCh:
			p.lastBundleStatuses = statuses
			if *p.config.Trigger == plugins.TriggerPeriodic {
				err := p.oneShot(ctx)
				if err != nil {
					p.logger.Error("%v.", err)
				} else {
					p.logger.Info("Status update sent successfully in response to bundle update.")
				}
			}

		case status := <-p.bundleCh:
			p.lastBundleStatus = &status
			err := p.oneShot(ctx)
			if err != nil {
				p.logger.Error("%v.", err)
			} else {
				p.logger.Info("Status update sent successfully in response to bundle update.")
			}
		case status := <-p.discoCh:
			p.lastDiscoStatus = &status
			if *p.config.Trigger == plugins.TriggerPeriodic {
				err := p.oneShot(ctx)
				if err != nil {
					p.logger.Error("%v.", err)
				} else {
					p.logger.Info("Status update sent successfully in response to discovery update.")
				}
			}
		case status := <-p.decisionLogsCh:
			p.lastDecisionLogsStatus = &status
			if *p.config.Trigger == plugins.TriggerPeriodic {
				err := p.oneShot(ctx)
				if err != nil {
					p.logger.Error("%v.", err)
				} else {
					p.logger.Info("Status update sent successfully in response to decision log update.")
				}
			}
		case update := <-p.reconfig:
			p.reconfigure(update.config)
			update.done <- struct{}{}
		case respCh := <-p.queryCh:
			p.readBundleStatus()
			respCh <- p.snapshot()
		case update := <-p.trigger:
			// make sure the more recent status is registered
			p.readBundleStatus()
			err := p.oneShot(update.ctx)
			if err != nil {
				p.logger.Error("%v.", err)
				if update.ctx.Err() == nil {
					update.done <- err
				}
			} else {
				p.logger.Info("Status update sent successfully in response to manual trigger.")
			}
			close(update.done)
		case done := <-p.stop:
			cancel()
			done <- struct{}{}
			return
		}
	}
}

// readBundleStatus is a non-blocking read to make sure the latest status is received
func (p *Plugin) readBundleStatus() bool {
	var changed bool

	select {
	case status := <-p.pluginStatusCh:
		p.lastPluginStatuses = status
		changed = true
	default:
	}

	select {
	case status := <-p.bulkBundleCh:
		p.lastBundleStatuses = status
		changed = true
	default:
	}

	select {
	case status := <-p.bundleCh:
		p.lastBundleStatus = &status
		changed = true
	default:
	}

	select {
	case status := <-p.discoCh:
		p.lastDiscoStatus = &status
		changed = true
	default:
	}

	select {
	case status := <-p.decisionLogsCh:
		p.lastDecisionLogsStatus = &status
		changed = true
	default:
	}

	return changed
}

func (p *Plugin) oneShot(ctx context.Context) error {
	req := p.snapshot()

	if p.config.ConsoleLogs {
		err := p.logUpdate(req)
		if err != nil {
			p.logger.Error("Failed to log to console: %v.", err)
		}
	}

	if p.config.Prometheus {
		p.updatePrometheusMetrics(req)
	}

	if p.config.Plugin != nil {
		proxy, ok := p.manager.Plugin(*p.config.Plugin).(Logger)
		if !ok {
			return errors.New("plugin does not implement Logger interface")
		}
		return proxy.Log(ctx, req)
	}

	if p.config.Service != "" {
		resp, err := p.manager.Client(p.config.Service).
			WithJSON(req).
			Do(ctx, "POST", fmt.Sprintf("/status/%v", p.config.PartitionName))
		if err != nil {
			return fmt.Errorf("status update failed: %w", err)
		}

		defer util.Close(resp)

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("status update failed, server replied with HTTP %v %v", resp.StatusCode, http.StatusText(resp.StatusCode))
		}
	}
	return nil
}

func (p *Plugin) reconfigure(config any) {
	newConfig := config.(*Config)

	if reflect.DeepEqual(p.config, *newConfig) {
		p.logger.Debug("Status reporter configuration unchanged.")
		return
	}

	p.logger.Info("Status reporter configuration changed.")

	if newConfig.Prometheus && !p.config.Prometheus {
		p.collectors.RegisterAll(p.manager.PrometheusRegister(), p.logger)
	} else if !newConfig.Prometheus && p.config.Prometheus {
		p.collectors.UnregisterAll(p.manager.PrometheusRegister())
	} else if newConfig.Prometheus && p.config.Prometheus {
		if !reflect.DeepEqual(newConfig.PrometheusConfig, p.config.PrometheusConfig) {
			p.collectors.ReregisterBundleLoadDuration(p.manager.PrometheusRegister(), newConfig.PrometheusConfig, p.logger)
		}
	}

	p.config = *newConfig
}

func (p *Plugin) snapshot() *UpdateRequestV1 {
	s := &UpdateRequestV1{
		Labels:       p.manager.Labels(),
		Discovery:    p.lastDiscoStatus,
		DecisionLogs: p.lastDecisionLogsStatus,
		Bundle:       p.lastBundleStatus,
		Bundles:      p.lastBundleStatuses,
		Plugins:      p.lastPluginStatuses,
	}

	if p.metrics != nil {
		s.Metrics = map[string]any{p.metrics.Info().Name: p.metrics.All()}
	}

	return s
}

func (p *Plugin) logUpdate(update *UpdateRequestV1) error {
	eventBuf, err := json.Marshal(&update)
	if err != nil {
		return err
	}
	fields := map[string]any{}
	err = util.UnmarshalJSON(eventBuf, &fields)
	if err != nil {
		return err
	}
	p.manager.ConsoleLogger().WithFields(fields).WithFields(map[string]any{
		"type": "openpolicyagent.org/status",
	}).Info("Status Log")
	return nil
}

func (p *Plugin) updatePrometheusMetrics(u *UpdateRequestV1) {
	p.collectors.pluginStatus.Reset()
	for name, plugin := range u.Plugins {
		p.collectors.pluginStatus.WithLabelValues(name, string(plugin.State)).Set(1)
	}
	p.collectors.lastSuccessfulActivation.Reset()
	for _, bundle := range u.Bundles {
		if bundle.Code == "" && !bundle.LastSuccessfulActivation.IsZero() {
			p.collectors.loaded.WithLabelValues(bundle.Name).Inc()
		} else {
			p.collectors.failLoad.WithLabelValues(bundle.Name, bundle.Code, bundle.Message).Inc()
		}
		p.collectors.lastSuccessfulActivation.WithLabelValues(bundle.Name, bundle.ActiveRevision).Set(float64(bundle.LastSuccessfulActivation.UnixNano()))
		p.collectors.lastSuccessfulDownload.WithLabelValues(bundle.Name).Set(float64(bundle.LastSuccessfulDownload.UnixNano()))
		p.collectors.lastSuccessfulRequest.WithLabelValues(bundle.Name).Set(float64(bundle.LastSuccessfulRequest.UnixNano()))
		p.collectors.lastRequest.WithLabelValues(bundle.Name).Set(float64(bundle.LastRequest.UnixNano()))

		if bundle.Metrics != nil {
			for stage, metric := range bundle.Metrics.All() {
				switch stage {
				case "timer_bundle_request_ns", "timer_rego_data_parse_ns", "timer_rego_module_parse_ns", "timer_rego_module_compile_ns", "timer_rego_load_bundles_ns":
					p.collectors.bundleLoadDuration.WithLabelValues(bundle.Name, stage).Observe(float64(metric.(int64)))
				}
			}
		}
	}
}

func (u UpdateRequestV1) Equal(other UpdateRequestV1) bool {
	return maps.Equal(u.Labels, other.Labels) &&
		maps.EqualFunc(u.Bundles, other.Bundles, (*bundle.Status).Equal) &&
		maps.EqualFunc(u.Plugins, other.Plugins, (*plugins.Status).Equal) &&
		u.Bundle.Equal(other.Bundle) &&
		u.Discovery.Equal(other.Discovery) &&
		u.DecisionLogs.Equal(other.DecisionLogs) &&
		nullSafeDeepEqual(u.Metrics, other.Metrics)
}

func nullSafeDeepEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	return a != nil && b != nil && reflect.DeepEqual(a, b)
}
