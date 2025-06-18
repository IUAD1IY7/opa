---
title: Kubernetes Admission Control using Vulnerability Scanning
software:
- kubernetes
- clair
labels:
  layer: orchestration
  category: containers
  datasource: clair
code:
- https://github.com/IUAD1IY7/contrib/tree/master/image_enforcer
tutorials:
- https://github.com/IUAD1IY7/contrib/blob/master/image_enforcer/README.md
docs_features:
  rest-api-integration:
    note: |
      This example project in
      [OPA contrib](https://github.com/IUAD1IY7/contrib/tree/main/image_enforcer)
      uses OPA over the REST API to enforce admission policy based on
      vulnerability scanning results.
  kubernetes:
    note: |
      This example project in
      [OPA contrib](https://github.com/IUAD1IY7/contrib/tree/main/image_enforcer)
      uses OPA to enforce admission policy in Kubernetes.
---
Admission control policies in Kubernetes can be augmented with
vulnerability scanning results to make more informed decisions.
This integration demonstrates how to integrate CoreOS Clair with OPA and
run it as an admission controller.

