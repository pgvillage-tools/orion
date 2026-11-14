# Orion on Kubernetes

A minimal orion cluster with the `kubernetes` store backend: two sentinels, two keepers (a StatefulSet), two proxies
and the api, with their RBAC.

```bash
kubectl create namespace orion
# the api certificates and the postgres passwords: see the docs
kubectl -n orion apply -k examples/kubernetes
```

The steps, from the certificates to a failover, are in [Deploy on Kubernetes](../../docs/deploy/k8s.md).

These manifests are tested: the e2e tests (`tests/k8s`, `make kind-e2e-setup k8s-e2e-test`) deploy them on
[kind](https://kind.sigs.k8s.io/) for every change, through `tests/k8s/manifests/kustomization.yaml`, which only adds
the namespace and the locally built images.

They are a start, not a production setup: add resource requests and limits, PodDisruptionBudgets and anti-affinity,
and use your own StorageClass.
