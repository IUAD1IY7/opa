---
title: Debugging Tips
---

If you run into problems getting OPA to enforce admission control policies in
Kubernetes there are a few things you can check to make sure everything is
configured correctly. If none of these tips work, feel free to join
[our slack](https://slack.openpolicyagent.org) and ask for help.

The tips below cover the OPA-Kubernetes integration that uses kube-mgmt.
The [OPA Gatekeeper version](https://open-policy-agent.github.io/gatekeeper)
has its own docs.

### Check for the `openpolicyagent.org/kube-mgmt-status` annotation on ConfigMaps containing policies

If you are loading policies into OPA via
[kube-mgmt](https://github.com/IUAD1IY7/kube-mgmt) you can check the
`openpolicyagent.org/kube-mgmt-status` annotation on ConfigMaps that contain your
policies. The annotation should be set to `{"status":"ok"}` if the policy was loaded
successfully. If errors occurred during loading (e.g., because the policy
contained a syntax error) the cause will be reported here.

If the annotation is
missing entirely, check the `kube-mgmt` container logs for connection errors
between the container and the Kubernetes API server.

### Check the `kube-mgmt` container logs for error messages

When `kube-mgmt` is healthy, the container logs will be quiet/empty. If you are
trying to enforce policies based on Kubernetes context (e.g., to check for
ingress conflicts) then you need to make sure that `kube-mgmt` can replicate
Kubernetes objects into OPA. If `kube-mgmt` is unable to list/watch resources in
the Kubernetes API server, they will not be replicated into OPA and the policy
will not get enforced.

### Check the `opa` container logs for TLS errors

Communication between the Kubernetes API server and OPA is secured with TLS. If
the CA bundle specified in the webhook configuration is out-of-sync with the
server certificate that OPA is configured with, OPA will log errors indicating a
TLS issue. Verify that the CA bundle specified in the validating or mutating
webhook configurations matches the server certificate you configured OPA to use.

### Check for POST requests in the `opa` container logs

When the Kubernetes API server queries OPA for admission control decisions, it
sends HTTP `POST` requests. If there are no `POST` requests contained in the
`opa` container logs, it indicates that the webhook configuration is wrong or
there is a network connectivity problem between the Kubernetes API server and
OPA.

- If you have access to the Kubernetes API server logs, review them to see if
  they indicate the cause.
- If you are running on AWS EKS make sure your security group settings allow
  traffic from Kubernetes "master" nodes to the node(s) where OPA is running.

### Ensure the webhook is configured for the proper namespaces

When you create the webhook according to the installation instructions,
it includes a namespaceSelector so that you
can decide which namespaces to ignore.

```yaml
namespaceSelector:
  matchExpressions:
  - key: openpolicyagent.org/webhook
    operator: NotIn
    values:
    - ignore
```

If OPA seems to not be making the decisions you expect, check if the namespace
is using the label `openpolicyagent.org/webhook: ignore`.

If OPA is making decision on namespaces (like `kube-system`) that you would
prefer OPA would ignore, assign the namespace the label
`openpolicyagent.org/webhook: ignore`.

### Ensure mutating policies construct JSON Patches correctly

If you are using OPA to enforce mutating admission policies you must ensure the
JSON Patch objects you generate escape "/" characters in the JSON Pointer. For
example, if you are generating a JSON Patch that sets annotations like
`acmecorp.com/myannotation` you need to escape the "/" character in the
annotation name using `~1` (per [RFC 6901](https://tools.ietf.org/html/rfc6901#section-3)).

<SideBySideContainer>
<SideBySideColumn>

```json title="Correct"
{
  "op": "add",
  "path": "/metadata/annotations/acmecorp.com~1myannotation",
  "value": "somevalue"
}
```

</SideBySideColumn>

<SideBySideColumn>
```json title="Incorrect"
{
  "op": "add",
  "path": "/metadata/annotations/acmecorp.com/myannotation",
  "value": "somevalue"
}
```
</SideBySideColumn>
</SideBySideContainer>

In addition, when your policy generates the response for the Kubernetes API
server, you must use the `base64.encode` built-in function to encode the JSON
Patch objects. DO NOT use the `base64url.encode` function because the Kubernetes
API server will not process it:

<SideBySideContainer>
<SideBySideColumn>
```rego title="Correct"
package system

main := {
"apiVersion": "admission.k8s.io/v1",
"kind": "AdmissionReview",
"response": response,
}

response := {
"allowed": true,
"patchType": "JSONPatch",
"patch": base64.encode(json.marshal(patches)) # <-- GOOD: uses base64.encode
}

patches := [
{
"op": "add",
"path": "/metadata/annotations/acmecorp.com~1myannotation",
"value": "somevalue"
}
]

````
</SideBySideColumn>
<SideBySideColumn>
```rego title="Incorrect"
package system

main := {
	"apiVersion": "admission.k8s.io/v1",
	"kind": "AdmissionReview",
	"response": response,
}

response := {
  "allowed": true,
  "patchType": "JSONPatch",
  "patch": base64url.encode(json.marshal(patches))   # <-- BAD: uses base64url.encode
}

patches := [
  {
    "op": "add",
    "path": "/metadata/annotations/acmecorp.com~1myannotation",
    "value": "somevalue"
  }
]
````

</SideBySideColumn>
</SideBySideContainer>

Also, for more examples of how to construct mutating policies and integrating
them with validating policies, see [these examples](https://github.com/IUAD1IY7/library/tree/master/kubernetes/mutating-admission)
in https://github.com/IUAD1IY7/library.
