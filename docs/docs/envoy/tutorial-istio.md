---
title: "Tutorial: Istio"
sidebar_position: 3
---

[Istio](https://istio.io/latest/) is an open source service mesh for managing the different microservices that make
up a cloud-native application. Istio provides a mechanism to use a service as an external authorizer with the
[AuthorizationPolicy API](https://istio.io/latest/docs/tasks/security/authorization/authz-custom/).

This tutorial shows how Istio's AuthorizationPolicy can be configured to delegate authorization decisions to OPA.

## Prerequisites

This tutorial requires Kubernetes 1.20 or later. To run the tutorial locally ensure you start a cluster with Kubernetes
version 1.20+, we recommend using [minikube](https://kubernetes.io/docs/getting-started-guides/minikube) or
[KIND](https://kind.sigs.k8s.io/).

The tutorial also requires Istio v1.19.0 or later. It assumes you have Istio deployed on top of Kubernetes.
See Istio's [Helm Install](https://istio.io/latest/docs/setup/install/helm/) page to get started.

If you are using an earlier version of Istio (1.9+), you will have to customize the `AuthorizationPolicy` in the
`quick_start.yaml` file to use the `security.istio.io/v1beta1` API version instead of `security.istio.io/v1`.

## Steps

### 1. Install OPA-Envoy

```bash
kubectl apply -f https://raw.githubusercontent.com/open-policy-agent/opa-envoy-plugin/main/examples/istio/quick_start.yaml
```

The `quick_start.yaml` manifest defines the following resources:

- AuthorizationPolicy to direct authorization checks to the OPA-Envoy sidecar. See `kubectl -n {$NAMESPACE} get authorizationpolicy ext-authz` for details.

- ServiceEntry to allow Istio to find the OPA-Envoy sidecars. See `kubectl -n {$NAMESPACE} get serviceentry opa-ext-authz-grpc-local` for details.

- Kubernetes namespace (`opa-istio`) for OPA-Envoy control plane components.

- Kubernetes admission controller in the `opa-istio` namespace that automatically injects the OPA-Envoy sidecar into pods in namespaces labelled with `opa-istio-injection=enabled`.

- OPA configuration file and an OPA policy into ConfigMaps in the namespace where the app will be deployed, e.g., `default`.
  The following is the example OPA policy:

  - alice is granted a **guest** role and can perform a `GET` request to `/productpage`.
  - bob is granted an **admin** role and can perform a `GET` to `/productpage` and `/api/v1/products`.

  ```rego title="authz.rego"
  package istio.authz

  default allow := false

  allow if {
  	input.parsed_path[0] == "health"
  	input.attributes.request.method == "GET"
  }

  allow if {
  	some user_role in _user_roles[_user_name]
  	some permission in _role_permissions[user_role]

  	permission.method == input.attributes.request.http.method
  	permission.path == input.attributes.request.http.path
  }

  # Underscore prefix used only to signal that rules and functions are
  # intended to be referenced only within the same policy, i.e. "private".
  # It has no special meaning to OPA.

  _user_name := parsed if {
  	[_, encoded] := split(input.attributes.request.http.headers.authorization, " ")
  	[parsed, _] := split(base64url.decode(encoded), ":")
  }

  _user_roles := {
  	"alice": ["guest"],
  	"bob": ["admin"],
  }

  _role_permissions := {
  	"guest": [{"method": "GET", "path": "/productpage"}],
  	"admin": [
  		{"method": "GET", "path": "/productpage"},
  		{"method": "GET", "path": "/api/v1/products"},
  	],
  }
  ```

  <RunSnippet id="authz.rego"/>

  OPA is configured to query for the `data.istio.authz.allow`
  decision. If the response is `true` the operation is allowed, otherwise the
  operation is denied. Sample input received by OPA is shown below:

  ```json title="input.json"
  {
    "attributes": {
      "request": {
        "http": {
          "method": "GET",
          "path": "/productpage",
          "headers": {
            "authorization": "Basic YWxpY2U6cGFzc3dvcmQ="
          }
        }
      }
    }
  }
  ```

  <RunSnippet id="input.json"/>

  ```rego
  package example

  result := data.istio.authz.allow
  ```

  <RunSnippet files="#input.json #authz.rego" command="data.example.result" />

  An example of the complete input received by OPA can be seen [here](https://github.com/IUAD1IY7/opa-envoy-plugin/tree/main/examples/istio#example-input).

  > In typical deployments the policy would either be built into the OPA container
  > image or it would be fetched dynamically via the [Bundle API](../management-bundles/). ConfigMaps are
  > used in this tutorial for test purposes.

### 2. Configure the mesh to define the external authorizer

Edit the mesh configmap with `kubectl edit configmap -n istio-system istio` and define the external provider:

```yaml
data:
  mesh: |-
    # Add the following lines to define the ServiceEntry previously created as an external authorizer:
    extensionProviders:
    - name: opa-ext-authz-grpc
      envoyExtAuthzGrpc:
        service: opa-ext-authz-grpc.local
        port: 9191
```

See [the Istio Docs for AuthorizationPolicy](https://istio.io/latest/docs/tasks/security/authorization/authz-custom/#define-the-external-authorizer) for
more details.

The format of the service value is `[<Namespace>/]<Hostname>`. The specification
of `<Namespace>` is required only when it is insufficient to unambiguously resolve
a service in the service registry. See also the [configuration documentation](https://istio.io/latest/docs/reference/config/istio.mesh.v1alpha1/#MeshConfig-ExtensionProvider-EnvoyExternalAuthorizationGrpcProvider).
Example: `opa-ext-authz-grpc.foo.svc.cluster.local` or
`bar/opa-ext-authz-grpc.local`.

### 3. Enable automatic injection of the Istio Proxy and OPA-Envoy sidecars in the namespace where the app will be deployed, e.g., `default`

```bash
kubectl label namespace default opa-istio-injection="enabled"
kubectl label namespace default istio-injection="enabled"
```

### 4. Deploy the BookInfo application and make it accessible outside the cluster

```bash
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/bookinfo/platform/kube/bookinfo.yaml
```

```bash
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/bookinfo/networking/bookinfo-gateway.yaml
```

### 5. Set the `SERVICE_HOST` environment variable in your shell to the public IP/port of the Istio Ingress gateway

Run this command in a new terminal window to start a Minikube tunnel that sends traffic to your Istio Ingress Gateway:

```
minikube tunnel
```

Check that the Service shows an `EXTERNAL-IP`:

```bash
kubectl -n istio-system get service istio-ingressgateway

NAME                   TYPE           CLUSTER-IP     EXTERNAL-IP   PORT(S)                                                                      AGE
istio-ingressgateway   LoadBalancer   10.98.42.178   127.0.0.1     15021:32290/TCP,80:30283/TCP,443:32497/TCP,31400:30216/TCP,15443:30690/TCP   5s
```

**minikube:**

```bash
export SERVICE_HOST=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
```

For other platforms see the [Istio documentation on determining ingress IP and ports.](https://istio.io/docs/tasks/traffic-management/ingress/#determining-the-ingress-ip-and-ports)

### 6. Exercise the OPA policy

Check that **alice** can access `/productpage` **BUT NOT** `/api/v1/products`.

```bash
curl --user alice:password -i http://$SERVICE_HOST/productpage
curl --user alice:password -i http://$SERVICE_HOST/api/v1/products
```

Check that **bob** can access `/productpage` **AND** `/api/v1/products`.

```bash
curl --user bob:password -i http://$SERVICE_HOST/productpage
curl --user bob:password -i http://$SERVICE_HOST/api/v1/products
```

## Wrap Up

Congratulations for finishing the tutorial !

This tutorial showed how Istio's [AuthorizationPolicy API](https://istio.io/latest/docs/tasks/security/authorization/authz-custom/)
can be configured to use OPA as an External authorization service.

This tutorial also showed a sample OPA policy that returns a `boolean` decision
to indicate whether a request should be allowed or not.

More details about the tutorial can be seen
[here](https://github.com/IUAD1IY7/opa-envoy-plugin/tree/main/examples/istio).
