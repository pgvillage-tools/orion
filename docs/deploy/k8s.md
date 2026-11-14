---
title: Orion on Kubernetes
summary: Run the Orion keepers, sentinels, proxies and api on Kubernetes, with Kubernetes itself as the store (the kubernetes store backend)
authors:
  - Sebas Mannem
date: 2026-11-14
---

# Orion on Kubernetes

This page describes how to run Orion on Kubernetes with the `kubernetes` store backend: Kubernetes itself stores the
cluster data and does the sentinel leader election, so you don't need etcd or consul.

The steps and manifests on this page are the ones the end-to-end tests run on [kind](https://kind.sigs.k8s.io/) for
every change (`tests/k8s`): the manifests below are included from `tests/k8s/manifests`. They are a minimal setup to
start from, not a production setup: add resource requests and limits, PodDisruptionBudgets and anti-affinity, and use
your own StorageClass.

## Prerequisites

- A Kubernetes cluster with a default StorageClass (for the keeper volumes), and `kubectl` access to a namespace in
  which you can create Roles and RoleBindings. This page uses the namespace `orion` and the cluster name `mycluster`.
- `orion-cli` on your machine, to create the certificates and to talk to the api.
- Images of the components. The release workflow publishes them as:

    | Component | Image |
    |-----------|-------|
    | keeper | `ghcr.io/pgvillage-tools/orion/keeper:<pg version>` (e.g. `18`), or `<pg version>-<release>` |
    | sentinel, proxy, api, cli | `ghcr.io/pgvillage-tools/orion/<component>:<release>`, or `:latest` |

    The manifests use `:latest` and `:18`: pin them to a release. To build your own images from the repository:

    ```bash
    make build-images CONTAINER_TOOL=docker   # keeper-18, sentinel, proxy, api and cli
    ```

To try it all on your machine, `make k8s-e2e-test` creates a kind cluster, loads the images you built, and runs the
steps of this page (with podman: add `CONTAINER_TOOL=podman`). `make kind-e2e-clean` removes the cluster.

## How the kubernetes backend works

| What | Where |
|------|-------|
| The cluster data (spec and status) | Annotation `orion-clusterdata` on the ConfigMap `orion-cluster-<cluster name>`. The api creates it on `orion-cli init`. |
| The state of each keeper, sentinel and proxy | Annotation `orion-status` on its own pod. Each component finds the others by the pod labels `component` (`orion-keeper`, `orion-sentinel` or `orion-proxy`) and `orion-cluster` (the cluster name). |
| The sentinel leader | The Lease `orion-cluster-<cluster name>` (`coordination.k8s.io/v1`). The sentinels compete for it; its holder is the uid of the leading sentinel. The api reads it. |

All objects live in the namespace of the pods. Every component (keeper, sentinel, proxy and api) needs:

| Option | Environment variable | Value |
|--------|----------------------|-------|
| `--cluster-name` | `ORION<COMPONENT>_CLUSTER_NAME` | The cluster name. It must match the `orion-cluster` label of the pods. |
| `--store-backend` | `ORION<COMPONENT>_STORE_BACKEND` | `kubernetes` |
| `--kube-resource-kind` | `ORION<COMPONENT>_KUBE_RESOURCE_KIND` | `configmap`, the only supported (and required) value. |

The keepers, sentinels and proxies also need `POD_NAME`, the name of their own pod (from the downward api): they
publish their state on it. `<COMPONENT>` is `KEEPER`, `SENTINEL`, `PROXY` or `API`; every option can be set with its
environment variable (`--some-flag` is `ORION<COMPONENT>_SOME_FLAG`). The components use the in-cluster configuration
of their ServiceAccount, and the namespace of their pod.

## Namespace

```bash
kubectl create namespace orion
```

## Certificates for the api

The api only accepts mTLS connections, see [Certificates for the api and the cli](../api/certificates.md). Create
the certificates on your machine, with the names of the api Service on the api certificate:

```bash
orion-cli pki init --api-san orion-api --api-san orion-api.orion.svc
```

The api certificate is always valid for `localhost` and `127.0.0.1` too, so `kubectl port-forward` works without extra
names. Store the api files in a Secret:

```bash
kubectl -n orion create secret generic orion-api-pki --from-file="$HOME/.orion/api/"
```

The api image runs as uid 1000 and reads its certificates from `/etc/orion/api`. Secret files are owned by root, with
the pod's `fsGroup` as group: the api Deployment below mounts the Secret with `defaultMode: 0440` and sets
`fsGroup: 1000`. Keep `~/.orion/cli` (the CA keys and the client certificate of the cli) on your machine: it doesn't
belong in the cluster.

## Passwords for PostgreSQL

The keepers need a superuser password and a replication password, the same for all keepers:

```bash
kubectl -n orion create secret generic orion-pg \
  --from-literal=su-password="$(openssl rand -base64 24)" \
  --from-literal=repl-password="$(openssl rand -base64 24)"
```

## RBAC

What each component does with the Kubernetes api:

| Component | pods | configmaps | leases (`coordination.k8s.io`) | events |
|-----------|------|------------|--------------------------------|--------|
| keeper | get, patch (its own pod) | get | | |
| proxy | get, patch (its own pod) | get | | |
| sentinel | get, patch (its own pod), list | get, create, update | get, create, update | create, patch |
| api | list | get, create, update | get | |

The sentinel records events about the leader election. One Role for all components is the simplest start; split it
per component (with a ServiceAccount each) to give each only what it needs.

```yaml title="rbac.yaml"
--8<-- "tests/k8s/manifests/rbac.yaml"
```

## Sentinels

```yaml title="sentinel.yaml"
--8<-- "tests/k8s/manifests/sentinel.yaml"
```

## Keepers

A StatefulSet, with a volume per keeper. The keeper image runs as `postgres` (uid 999), with `/pgdata` as data
directory and `/pgwal/pgwal` as WAL directory: both need a persistent volume, or a restarted keeper loses its data.
The keeper generates its uid on the first start, and keeps it in its data directory.

```yaml title="keeper.yaml"
--8<-- "tests/k8s/manifests/keeper.yaml"
```

- The superuser is `postgres` (the user that runs the keeper) unless you set `ORIONKEEPER_PG_SU_USERNAME`. The
  replication user is `repluser`.
- The keeper advertises its pod IP to the other components. The IP changes when the pod restarts; the keeper then
  publishes the new one.

## Proxies

The proxy image listens on port 25432. Clients connect through the Service, which only reaches the primary.

```yaml title="proxy.yaml"
--8<-- "tests/k8s/manifests/proxy.yaml"
```

## Api

The api doesn't need the `orion-cluster` label: the components don't look for it. For the probes, see
[Probe port](../api/certificates.md#probe-port).

```yaml title="api.yaml"
--8<-- "tests/k8s/manifests/api.yaml"
```

## Deploying

Apply the manifests, and wait until everything runs:

```bash
kubectl -n orion apply -f rbac.yaml -f sentinel.yaml -f keeper.yaml -f proxy.yaml -f api.yaml
for w in deployment/orion-sentinel deployment/orion-proxy deployment/orion-api statefulset/orion-keeper; do
  kubectl -n orion rollout status "$w" --timeout=5m
done
```

With the files in one directory, a `kustomization.yaml` can set the namespace and the images for all of them; the
e2e tests do that in `tests/k8s/manifests/kustomization.yaml` (`kubectl apply -k`).

## Creating the cluster

Forward the api port to your machine, and check the connection:

```bash
kubectl -n orion port-forward svc/orion-api 8443:8443 &
orion-cli connect
# Connected to the api as orion-cli (client-certificate)
```

The cli talks to the api only (by default on `127.0.0.1:8443`). `connect` saves the api host and port in
`~/.orion/config.yml`.

Create a new cluster with the default settings, and wait until it is available:

```bash
orion-cli init --wait
```

To start with your own settings, pass a cluster spec: `orion-cli init --file spec.json` (or `-` for stdin). The
sentinel leader initializes the database on one of the keepers, and the other keeper becomes its standby.

## Checking the cluster

```bash
orion-cli status
```

It shows the sentinels (and which one leads), the proxies, the keepers and the primary. On the Kubernetes side:

```bash
kubectl -n orion get lease orion-cluster-mycluster      # the holder is the uid of the leading sentinel
kubectl -n orion get pods -l orion-cluster=mycluster
```

Connect to PostgreSQL through the proxy Service, from a pod in the cluster (`psql -h orion-proxy.orion.svc -U
postgres`), or from your machine:

```bash
kubectl -n orion port-forward svc/orion-proxy 5432:5432 &
psql -h 127.0.0.1 -U postgres     # the su-password of the Secret orion-pg
```

## Failing over

Fail the primary keeper; the sentinel promotes the standby once it caught up:

```bash
orion-cli failkeeper <primary keeper uid> --wait
orion-cli status
```

When the pod of the leading sentinel goes away, the other sentinel takes over the Lease, and with it the lead.

## Limitations

- The components need `--kube-resource-kind=configmap`, although the election uses a Lease. This option is a leftover
  of stolon and may go away.
- The kubernetes backend keeps all cluster data in one annotation, so it is limited by the maximum size of the
  annotations of an object (256 KiB).
- Keepers, sentinels and proxies find each other by the labels of their pods, in their own namespace: a cluster name
  must be unique within the namespace, and the `orion-cluster` label must match `--cluster-name`.
- The api must run with certificates; starting it without certificates (bootstrap mode) is planned.
