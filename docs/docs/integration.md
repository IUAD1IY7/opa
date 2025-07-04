---
title: Integrating OPA
sidebar_position: 7
---

OPA exposes domain-agnostic APIs that your service can call to manage and
enforce policies. Read this page if you want to integrate an application,
service, or tool with OPA.

When integrating with OPA there are two interfaces to consider:

- **Evaluation**: OPA's interface for asking for policy decisions. Integrating OPA is primarily focused on integrating an application, service, or tool with OPA's policy evaluation interface. This integration results in policy decisions being decoupled from that application, service, or tool.
- **Management**: OPA's interface for deploying policies, understanding status, uploading logs, and so on. This integration is typically the same across all OPA instances, regardless what software the evaluation interface is integrated with. Distributing policy, retrieving status, and storing logs in the same way across all OPAs provides a unified management plane for policy across many different software systems.

This page focuses predominantly on different ways to integrate with OPA's policy evaluation interface and how they compare. For more information about the management interface:

- See the [Bundle API](./management-bundles) for distributing policy and data to OPA.
- See the [Status API](./management-status) for collecting status reports on bundle activation and agent health.
- See the [Decision Log API](./management-decision-logs) for collecting a log of policy decisions made by agents.
- See the [Health API](./rest-api#health-api) for checking agent deployment readiness and health.
- See the [Prometheus API endpoint](./monitoring/#prometheus) to obtain insight into performance and errors.

## Evaluating Policies

OPA supports different ways to evaluate policies.

- The [REST API](./rest-api) returns decisions as JSON over HTTP.
  - Also see the [Language SDKs](/ecosystem/#languages) for working with the REST API in different languages.
- The [Go API (GoDoc)](https://pkg.go.dev/github.com/IUAD1IY7/opa/v1/rego) returns
  decisions as simple Go types (`bool`, `string`, `map[string]interface{}`,
  etc.)
- [WebAssembly](./wasm) compiles Rego policies into Wasm instructions so they can be embedded and evaluated by any WebAssembly runtime
- Custom compilers and evaluators may be written to parse evaluation plans in the low-level
  [Intermediate Representation](./ir) format, which can be emitted by the `opa build` command
- The [SDK](https://pkg.go.dev/github.com/IUAD1IY7/opa/v1/sdk) provides high-level APIs for obtaining the output
  of query evaluation as simple Go types (`bool`, `string`, `map[string]interface{}`, etc.)

### Integrating with the REST API

To integrate with OPA outside of Go, we recommend you deploy OPA as a host-level
daemon or sidecar container. Running OPA locally on the same host as your
application or service helps ensure policy decisions are fast and highly-available.

When your application or service needs to make policy decisions it can query OPA
locally via HTTP. While it's possible to call OPA's [REST API](./rest-api) directly,
you can also find a number of [native language REST SDKs](/ecosystem/#languages)
which make the integration easier.

#### Named Policy Decisions

Use the [Data API](./rest-api#data-api) to query OPA for _named_ policy decisions:

```http
POST /v1/data/<path>
Content-Type: application/json
```

```json
{
    "input": <the input document>
}
```

The `<path>` in the HTTP request identifies the policy decision to ask for. In
OPA, every rule generates a policy decision. In the example below there are two
decisions: `example/authz/allow` and `example/authz/is_admin`.

```rego
package example.authz

default allow := false

allow if {
	input.method == "GET"
	input.path == ["salary", input.subject.user]
}

allow if is_admin

is_admin if "admin" in input.subject.groups
```

You can request specific decisions by querying for `<package path>/<rule name>`.
For example to request the `allow` decision execute the following HTTP request:

```http
POST /v1/data/example/authz/allow
Content-Type: application/json
```

```json
{
    "input": <the input document>
}
```

The body of the request specifies the value of the `input` document to use
during policy evaluation. For example:

```http
POST /v1/data/example/authz/allow
Content-Type: application/json
```

```json
{
  "input": {
    "method": "GET",
    "path": ["salary", "bob"],
    "subject": {
      "user": "bob"
    }
  }
}
```

OPA returns an HTTP 200 response code if the policy was evaluated successfully.
Non-HTTP 200 response codes indicate configuration or runtime errors. The policy
decision is contained in the `"result"` key of the response message body. For
example, the above request returns the following response:

```http
200 OK
Content-Type: application/json
```

```json
{
  "result": true
}
```

If the requested policy decision is _undefined_ OPA returns an HTTP 200 response
without the `"result"` key. For example, the following request for `is_admin` is
undefined because there is no default value for `is_admin` and the input does
not satisfy the `is_admin` rule body:

```http
POST /v1/data/example/authz/is_admin
Content-Type: application/json
```

```json
{
  "input": {
    "subject": {
      "user": "bob",
      "groups": ["sales", "marketing"]
    }
  }
}
```

The response:

```http
200 OK
Content-Type: application/json
```

```json
{}
```

For another example of how to integrate with OPA via HTTP see the [HTTP API Authorization](./http-api-authorization) tutorial.
The [reference documentation](./rest-api) is also a good place to start.

#### Ecosystem Projects

The REST API is a common way to build integrations with OPA in distributed systems.
Browse the OPA Ecosystem for <EcosystemFeatureLink feature="rest-api-integration">
examples on REST API integrations</EcosystemFeatureLink> for inspiration.

### Integrating with the Go SDK

:::info
This section documents the v1 SDK package.
Please see [v0 Backwards Compatibility](./v0-compatibility) for notes on using
the v0 SDK package.
:::

The [SDK](https://pkg.go.dev/github.com/IUAD1IY7/opa/sdk) package contains high-level APIs for embedding OPA
inside of Go programs and obtaining the output of query evaluation. To get started, import the `sdk` package:

```go
import "github.com/IUAD1IY7/opa/v1/sdk"
```

A typical workflow when using the `sdk` package would involve first creating a new `sdk.OPA` object by calling
`sdk.New` and then invoking its `Decision` method to fetch the policy decision. The `sdk.New` call takes the
`sdk.Options` object as an input which allows specifying the OPA configuration, console logger, plugins, etc.

Here is an example that shows this process:

```go
package main

import (
	"bytes"
	"context"
	"fmt"

	"github.com/IUAD1IY7/opa/v1/sdk"
	sdktest "github.com/IUAD1IY7/opa/v1/sdk/test"
)

func main() {
	ctx := context.Background()

	// create a mock HTTP bundle server
	server, err := sdktest.NewServer(sdktest.MockBundle("/bundles/bundle.tar.gz", map[string]string{
		"example.rego": `
				package authz

				default allow := false

				allow if input.open == "sesame"
			`,
	}))
	if err != nil {
		// handle error.
	}

	defer server.Stop()

	// provide the OPA configuration which specifies
	// fetching policy bundles from the mock server
	// and logging decisions locally to the console
	config := []byte(fmt.Sprintf(`{
		"services": {
			"test": {
				"url": %q
			}
		},
		"bundles": {
			"test": {
				"resource": "/bundles/bundle.tar.gz"
			}
		},
		"decision_logs": {
			"console": true
		}
	}`, server.URL()))

	// create an instance of the OPA object
	opa, err := sdk.New(ctx, sdk.Options{
		ID:     "opa-test-1",
		Config: bytes.NewReader(config),
	})
	if err != nil {
		// handle error.
	}

	defer opa.Stop(ctx)

	// get the named policy decision for the specified input
	if result, err := opa.Decision(ctx, sdk.DecisionOptions{Path: "/authz/allow", Input: map[string]interface{}{"open": "sesame"}}); err != nil {
		// handle error.
	} else if decision, ok := result.Result.(bool); !ok || !decision {
		// handle error.
	}
}
```

If you executed this code, the output (i.e. [Decision Log](https://www.openpolicyagent.org/docs/latest/management-decision-logs/) event)
would be logged to the console by default.

Setting an `ID` in `sdk.Options` is optional, but recommended. If you do not set an `ID`, a random one will be created
for the system. While this is fine for testing, it makes it difficult to monitor the system over time, as a new ID will
be created each time the SDK is initialized, such as when the process is restarted.

#### Manually Triggering Bundle Reloads

Users of the SDK can
[manually trigger](./configuration/#bundles)
the SDK's Bundle plugin to load new bundles immediately based on external
events. When doing so, it's recommended to set `bundles[_].trigger` to `manual`
if you want to disable periodic bundle polling.

In this short example, the `bundle` plugin is loaded from the SDK instance and
triggered to check for new bundles. Do this sparingly, it is not intended to be
used as a replacement for periodic bundle polling. For best performance, only
trigger the bundle plugin when you know that new bundles are available.

```go
options := sdk.Options{
    Config: bytes.NewReader(config),
    Logger: logger,
    Ready:  make(chan struct{}), // <-- needed or else sdk.New will block
}

opa, err := sdk.New(ctx, options)
if err != nil {
    log.Fatal(err)
}
defer opa.Stop(ctx)

bundle, ok := opa.Plugin("bundle").(*bundle.Plugin)
if !ok {
    log.Fatal("bundle plugin not found")
}

err = bundle.Trigger(ctx)
if err != nil {
    log.Fatal(err)
}
```

### Integrating with the Go API

Use the low-level
[github.com/IUAD1IY7/opa/v1/rego](https://pkg.go.dev/github.com/IUAD1IY7/opa/v1/rego)
package to embed OPA as a library inside services written in Go, when only policy **evaluation** — and
no other capabilities of OPA, like the management features — are desired. If you're unsure which one to
use, the SDK is probably the better option.

To get started import the `rego` package:

```go
import "github.com/IUAD1IY7/opa/v1/rego"
```

The `rego` package exposes different options for customizing how policies are
evaluated. Through the `rego` package you can supply policies and data, enable
metrics and tracing, toggle optimizations, etc. In most cases you will:

1. Use the `rego` package to construct a prepared query.
2. Execute the prepared query to produce policy decisions.
3. Interpret and enforce the policy decisions.

Preparing queries in advance avoids parsing and compiling the policies on each
query and improves performance considerably. Prepared queries are safe to share
across multiple Go routines.

To prepare a query create a new `rego.Rego` object by calling `rego.New(...)`
and then invoke `rego.Rego#PrepareForEval`. The `rego.New(...)` call can be
parameterized with different options like the query, policy module(s), data
store, etc.

```go
module := `
package example.authz

default allow := false

allow if {
    input.method == "GET"
    input.path == ["salary", input.subject.user]
}

allow if is_admin

is_admin if "admin" in input.subject.groups
`

ctx := context.TODO()

query, err := rego.New(
    rego.Query("x = data.example.authz.allow"),
    rego.Module("example.rego", module),
    ).PrepareForEval(ctx)

if err != nil {
    // Handle error.
}
```

Using the `query` returned by `rego.Rego#PrepareForEval` call the `Eval`
function to evaluate the policy:

```go
input := map[string]interface{}{
    "method": "GET",
    "path": []interface{}{"salary", "bob"},
    "subject": map[string]interface{}{
        "user": "bob",
        "groups": []interface{}{"sales", "marketing"},
    },
}

results, err := query.Eval(ctx, rego.EvalInput(input))
```

The `rego.PreparedEvalQuery#Eval` function returns a _result set_ that contains
the query results. If the result set is empty it indicates the query could not
be satisfied. Each element in the result set contains a set of _variable
bindings_ and a set of expression values. The query from above includes a single
variable `x` so we can lookup the value and interpret it to enforce the policy
decision.

```go
if err != nil {
    // Handle evaluation error.
} else if len(results) == 0 {
    // Handle undefined result.
} else if result, ok := results[0].Bindings["x"].(bool); !ok {
    // Handle unexpected result type.
} else {
    // Handle result/decision.
    // fmt.Printf("%+v", results) => [{Expressions:[true] Bindings:map[x:true]}]
}
```

For the common case of policies evaluating to a single boolean value, there's
a helper method: With `results.Allowed()`, the previous snippet can be shortened
to

```go
results, err := query.Eval(ctx, rego.EvalInput(input))
if err != nil {
    // handle error
}
if !results.Allowed() {
    // handle result
}
```

For more examples of embedding OPA as a library see the
[`rego`](https://pkg.go.dev/github.com/IUAD1IY7/opa/v1/rego#pkg-examples)
package in the Go documentation.

:::info
This section documents the v1 Rego package.
Please see [v0 Backwards Compatibility](./v0-compatibility) for notes on using
the v0 Rego package.
:::

#### Ecosystem Projects

The Go API is made available to allow other projects to build policy functionality into their
applications. Browse <EcosystemFeatureLink feature="go-integration">Go
integrations</EcosystemFeatureLink> in the OPA Ecosystem for inspiration.

### WebAssembly (Wasm)

Policies can be evaluated as compiled Wasm binaries. See [OPA Wasm docs](./wasm) for more details.

There are a number of projects already built on OPA's Wasm support. Take a look
in <EcosystemFeatureLink feature="wasm-integration">Wasm
integrations</EcosystemFeatureLink> in the OPA Ecosystem for more details.

### Intermediate Representation (IR)

Policies may be compiled into evaluation plans using an intermediate representation format, suitable for custom
compilers and evaluators.

See [OPA IR docs](./ir) for more details.

## Comparison

A comparison of the different integration choices are summarized below.

| Dimension  | REST API        | Go Lib                     | Wasm                       |
| ---------- | --------------- | -------------------------- | -------------------------- |
| Evaluation | Fast            | Faster                     | Fastest                    |
| Language   | Any             | Only Go                    | Any with Wasm              |
| Operations | Update just OPA | Update entire service      | Update service rarely      |
| Security   | Must secure API | Enable only what is needed | Enable only what is needed |

Integrating OPA via the REST API is the most common, at the time of writing. OPA
is most often deployed either as a sidecar or less commonly as an external
service. Operationally this makes it easy to upgrade OPA and to configure it to
use its management services (bundles, status, decision logs, etc.). Because it
is a separate process it requires monitoring and logging (though this happens
automatically for any sidecar-aware environment like Kubernetes). OPA's
configuration and APIs must be secured according to the [security guide](./security).

Integrating OPA via the Go API only works for Go software. Updates to OPA
require re-vendoring and re-deploying the software. Evaluation has less overhead
than the REST API because all the communication happens in the same
operating-system process. All of the management functionality (bundles, decision
logs, etc.) must be either enabled or implemented. Security concerns are limited
to those management features that are enabled or implemented.

Wasm policies are embeddable in any programming language that has a Wasm
runtime. Evaluation has less overhead than the REST API (because it is evaluated
in the same operating-system process) and should outperform the Go API (because
the policies have been compiled to a lower-level instruction set). Each
programming language will need its own SDKs that implement the management
functionality and the evaluation interface. Typically new OPA language features
will not require updating the service since neither the Wasm runtime nor the
SDKs will be impacted. Updating the SDKs will require re-deploying the service.
Security is analogous to the Go API integration: it is mainly the management
functionality that presents security risks.
