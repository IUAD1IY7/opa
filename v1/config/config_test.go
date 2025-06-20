// Copyright 2019 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/IUAD1IY7/opa/v1/util"
	"github.com/IUAD1IY7/opa/v1/version"
)

func TestConfigPluginNames(t *testing.T) {
	tests := []struct {
		name     string
		conf     Config
		expected []string
	}{
		{
			name:     "empty config",
			conf:     Config{},
			expected: nil,
		},
		{
			name: "bundle",
			conf: Config{
				Bundle: []byte(`{"bundle": {"name": "test-bundle"}}`),
			},
			expected: []string{"bundles"},
		},
		{
			name: "bundles",
			conf: Config{
				Bundles: []byte(`{"bundles": {"test-bundle": {}}`),
			},
			expected: []string{"bundles"},
		},
		{
			name: "decision_logs",
			conf: Config{
				DecisionLogs: []byte(`{decision_logs: {}}`),
			},
			expected: []string{"decision_logs"},
		},
		{
			name: "status",
			conf: Config{
				Status: []byte(`{status: {}}`),
			},
			expected: []string{"status"},
		},
		{
			name: "plugins",
			conf: Config{
				Plugins: map[string]json.RawMessage{
					"some-plugin": {},
				},
			},
			expected: []string{"some-plugin"},
		},
		{
			name: "sorted",
			conf: Config{
				DecisionLogs: []byte(`{decision_logs: {}}`),
				Status:       []byte(`{status: {}}`),
			},
			expected: []string{"decision_logs", "status"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := test.conf.PluginNames()
			if !slices.Equal(actual, test.expected) {
				t.Errorf("Expected %v but got %v", test.expected, actual)
			}
		})
	}
}

func TestConfigPluginsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		conf     Config
		expected bool
	}{
		{
			name:     "empty config",
			conf:     Config{},
			expected: false,
		},
		{
			name: "bundle",
			conf: Config{
				Bundle: []byte(`{"bundle": {"name": "test-bundle"}}`),
			},
			expected: true,
		},
		{
			name: "bundles",
			conf: Config{
				Bundles: []byte(`{"bundles": {"test-bundle": {}}`),
			},
			expected: true,
		},
		{
			name: "decision_logs",
			conf: Config{
				DecisionLogs: []byte(`{decision_logs: {}}`),
			},
			expected: true,
		},
		{
			name: "status",
			conf: Config{
				Status: []byte(`{status: {}}`),
			},
			expected: true,
		},
		{
			name: "plugins",
			conf: Config{
				Plugins: map[string]json.RawMessage{
					"some-plugin": {},
				},
			},
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := test.conf.PluginsEnabled()
			if actual != test.expected {
				t.Errorf("Expected %t but got %t", test.expected, actual)
			}
		})
	}
}

func TestPersistDirectory(t *testing.T) {
	pwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}

	c := Config{}
	persistDir, err := c.GetPersistenceDirectory()
	if err != nil {
		t.Fatalf("%v", err)
	}

	if persistDir != filepath.Join(pwd, ".opa") {
		t.Errorf("expected persistDir to be %v, got %v", filepath.Join(pwd, ".opa"), persistDir)
	}

	dir := "/var/opa"
	c.PersistenceDirectory = &dir
	persistDir, err = c.GetPersistenceDirectory()
	if err != nil {
		t.Fatalf("%v", err)
	}

	if persistDir != dir {
		t.Errorf("expected peristDir %v and dir %v to be equal", persistDir, dir)
	}
}

func TestActiveConfig(t *testing.T) {

	common := `"labels": {
			"region": "west"
		},
		"keys": {
			"global_key": {
				"algorithm": HS256,
				"key": "secret"
			},
			"local_key": {
				"private_key": "some_private_key"
			}
		},
		"decision_logs": {
			"service": "acmecorp",
			"reporting": {
				"min_delay_seconds": 300,
				"max_delay_seconds": 600
			}
		},
		"plugins": {
			"some-plugin": {}
		},
		"server": {
			"decoding": {
				"max_length": 134217728,
				"gzip": {
					"max_length": 268435456
				}
			},
			"encoding": {
				"gzip": {
					"min_length": 1024,
					"compression_level": 1
				}
			},
			"metrics": {
				"prom": {
					"http_request_duration_seconds": {
						"buckets": [0.1, 0.2]
					}
				}
			}
		},
		"discovery": {"name": "config"}`

	serviceObj := `"services": {
			"acmecorp": {
				"url": "https://example.com/control-plane-api/v1",
				"response_header_timeout_seconds": 5,
				"headers": {"foo": "bar"},
				"credentials": {"bearer": {"token": "test"}}
			},
			"opa.example.com": {
				"url": "https://opa.example.com",
				"headers": {"foo": "bar"},
				"credentials": {"gcp_metadata": {"audience": "test"}}
			}
		},`

	servicesList := `"services": [
			{
				"name": "acmecorp",
				"url": "https://example.com/control-plane-api/v1",
				"response_header_timeout_seconds": 5,
				"headers": {"foo": "bar"},
				"credentials": {"bearer": {"token": "test"}}
			},
			{
				"name": "opa.example.com",
				"url": "https://opa.example.com",
				"headers": {"foo": "bar"},
				"credentials": {"gcp_metadata": {"audience": "test"}}
			}
		],`

	expectedCommon := fmt.Sprintf(`"labels": {
			"id": "foo",
			"version": %v,
			"region": "west"
		},
		"keys": {
			"global_key": {
				"algorithm": HS256
			},
			"local_key": {}
		},
		"decision_logs": {
			"service": "acmecorp",
			"reporting": {
				"min_delay_seconds": 300,
				"max_delay_seconds": 600
			}
		},
		"plugins": {
			"some-plugin": {}
		},
		"server": {
			"decoding": {
				"max_length": 134217728,
				"gzip": {
					"max_length": 268435456
				}
			},
			"encoding": {
				"gzip": {
					"min_length": 1024,
					"compression_level": 1
				}
			},
			"metrics": {
				"prom": {
					"http_request_duration_seconds": {
						"buckets": [0.1, 0.2]
					}
				}
			}
		},
		"default_authorization_decision": "/system/authz/allow",
		"default_decision": "/system/main",
		"discovery": {"name": "config"}`, version.Version)

	expectedServiceObj := `"services": {
			"acmecorp": {
				"url": "https://example.com/control-plane-api/v1",
				"response_header_timeout_seconds": 5,
				"headers": {"foo": "bar"}
			},
			"opa.example.com": {
				"url": "https://opa.example.com",
				"headers": {"foo": "bar"}
			}
		},`

	expectedServicesList := `"services": [
			{
				"name": "acmecorp",
				"url": "https://example.com/control-plane-api/v1",
				"response_header_timeout_seconds": 5,
				"headers": {"foo": "bar"}
			},
			{
				"name": "opa.example.com",
				"url": "https://opa.example.com",
				"headers": {"foo": "bar"}
			}
		],`

	badKeysConfig := []byte(`{
		"keys": [
			{
				"algorithm": "HS256"
			}
		]
	}`)

	badServicesConfig := []byte(`{
		"services": {
			"acmecorp": ["foo"]
		}
	}`)

	tests := map[string]struct {
		raw      []byte
		expected []byte
		wantErr  bool
		err      error
	}{
		"valid_config_with_svc_object": {
			fmt.Appendf(nil, `{ %v %v }`, serviceObj, common),
			fmt.Appendf(nil, `{ %v %v }`, expectedServiceObj, expectedCommon),
			false,
			nil,
		},
		"valid_config_with_svc_list": {
			fmt.Appendf(nil, `{ %v %v }`, servicesList, common),
			fmt.Appendf(nil, `{ %v %v }`, expectedServicesList, expectedCommon),
			false,
			nil,
		},
		"invalid_config_with_bad_keys": {
			badKeysConfig,
			nil,
			true,
			errors.New("illegal keys config type: []interface {}"),
		},
		"invalid_config_with_bad_creds": {
			badServicesConfig,
			nil,
			true,
			errors.New("type assertion error"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			conf, err := ParseConfig(tc.raw, "foo")
			if err != nil {
				t.Fatal(err)
			}

			actual, err := conf.ActiveConfig()

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

				var expected map[string]any
				if err := util.Unmarshal(tc.expected, &expected); err != nil {
					t.Fatal(err)
				}

				if !reflect.DeepEqual(actual, expected) {
					t.Fatalf("want %v got %v", expected, actual)
				}
			}
		})
	}

}

func TestExtraConfigFieldsRoundtrip(t *testing.T) {
	raw := []byte(`
decision_logger:
  console: true
foo: baz
bar:
  really: yes!`)
	conf, err := ParseConfig(raw, "id")
	if err != nil {
		t.Fatal(err)
	}

	actual, err := conf.ActiveConfig()
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]any{
		"foo":                            "baz",
		"bar":                            map[string]any{"really": "yes!"},
		"decision_logger":                map[string]any{"console": true},
		"default_authorization_decision": "/system/authz/allow",
		"default_decision":               "/system/main",
		"labels": map[string]any{
			"id":      "id",
			"version": version.Version,
		},
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("want %v got %v", expected, actual)
	}
}
