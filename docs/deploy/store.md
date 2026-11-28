---
title: The store
summary: Where Orion keeps its state (etcd v3 or Kubernetes), how to connect the components to it, with TLS and client certificates, and what happens when it is down
authors:
  - Sebas Mannem
date: 2026-11-28
---

# The store

All Orion components keep their state in a store, and talk to each other only through it (see
[Architecture](../architecture.md)). Orion supports two stores:

| `--store-backend` | Store | Use it for |
|-------------------|-------|------------|
| `etcdv3` | An etcd cluster (the v3 api) | Virtual machines and bare metal, or Kubernetes with a dedicated etcd |
| `kubernetes` | The Kubernetes api server | Orion on Kubernetes without an etcd of its own, see [Orion on Kubernetes](k8s.md) |

The store backends `etcd`, `etcdv2` and `consul` of stolon are no longer supported: a component refuses to start with
them.

## Options

Every component that uses the store (`orion-keeper`, `orion-sentinel`, `orion-proxy` and `orion-api`) has the same
store options. The cli has none: it only talks to the api.

| Option | Default | |
|--------|---------|---|
| `--cluster-name` | (required) | The name of the cluster |
| `--store-backend` | (required) | `etcdv3` or `kubernetes` |
| `--store-endpoints` | `http://127.0.0.1:2379` | The etcd endpoints, comma-separated (etcdv3 only) |
| `--store-prefix` | `orion/cluster` | The prefix of the keys in etcd (etcdv3 only) |
| `--store-timeout` | `5s` | The timeout of a request to the store |
| `--store-cert-file` | | The client certificate for etcd (etcdv3 only) |
| `--store-key` | | The private key of the client certificate (etcdv3 only) |
| `--store-ca-file` | (the system CAs) | The CA bundle to verify the certificates of etcd (etcdv3 only) |
| `--store-skip-tls-verify` | `false` | Don't verify the certificates of etcd. Insecure: only for tests. |

Every option can also be set with an environment variable: `--store-endpoints` of the keeper is
`ORIONKEEPER_STORE_ENDPOINTS`, of the sentinel `ORIONSENTINEL_STORE_ENDPOINTS`, of the proxy
`ORIONPROXY_STORE_ENDPOINTS` and of the api `ORIONAPI_STORE_ENDPOINTS`. See the
[command reference](../reference/commands/index.md).

All components of a cluster need the same `--cluster-name`, `--store-backend` and store options: a component with
another cluster name or prefix doesn't see the cluster.

## etcd

```bash
orion-sentinel --cluster-name orion1 --store-backend etcdv3 \
  --store-endpoints http://etcd1.example.com:2379,http://etcd2.example.com:2379,http://etcd3.example.com:2379
```

- **Run at least three etcd members.** etcd needs a majority of its members to work: three members survive the loss
  of one, five the loss of two.
- **Run etcd outside the PostgreSQL nodes**, or at least on other disks: the disk I/O of PostgreSQL must not slow
  down etcd.
- **The endpoints are a starting point.** The components connect to the endpoints, then use the client urls that the
  etcd members advertise (`--advertise-client-urls` of etcd). Every component must be able to reach those urls, and
  with TLS, the names in them must be on the certificates of etcd.
- **Enable auto-compaction in etcd.** Orion doesn't compact etcd, see
  [etcd compaction](../architecture.md#etcd-compaction).

### TLS and client certificates

With `https://` endpoints, the components connect to etcd with TLS. All endpoints must have the same scheme.

```bash
orion-keeper --cluster-name orion1 --store-backend etcdv3 \
  --store-endpoints https://etcd1.example.com:2379,https://etcd2.example.com:2379,https://etcd3.example.com:2379 \
  --store-ca-file /etc/orion/etcd/ca.crt \
  --store-cert-file /etc/orion/etcd/client.crt \
  --store-key /etc/orion/etcd/client.key \
  ...
```

- `--store-ca-file` verifies the server certificates of etcd. Without it, the components use the CAs of the system.
- `--store-cert-file` and `--store-key` identify the component to etcd. Set both: with only one of them, no client
  certificate is sent.
- To only let in clients with a client certificate, run etcd with `--client-cert-auth` and `--trusted-ca-file`
  (the CA that signed the client certificates), next to `--cert-file` and `--key-file`. See
  [etcd transport security](https://etcd.io/docs/latest/op-guide/security/).

Orion doesn't create the certificates for etcd yet: bring your own. The e2e tests run all components against an etcd
that only accepts client certificates.

## Several clusters in one store

One store can hold several Orion clusters. Each cluster lives under its own name:

- With etcd, under the key `<store-prefix>/<cluster-name>` (a leading `/` of the prefix is removed). Clusters with
  different names can share the prefix; a different prefix also separates them.
- With Kubernetes, in the ConfigMap and the Lease `orion-cluster-<cluster name>` in the namespace of the pods.

Each cluster has its own keepers, sentinels, proxies and api, all with its `--cluster-name`.

## When the store is down

The store is the only way the components learn about each other, so while it doesn't answer, nothing changes in the
cluster:

| Component | While the store is down |
|-----------|-------------------------|
| keeper | Leaves its PostgreSQL instance as it is, and tries again after its sleep interval. |
| sentinel | Makes no decisions: no failover. |
| proxy | Closes all client connections after `proxyTimeout` (`15s` by default), and with `--stop-listening` (the default) stops listening, so a load balancer sees it as down. |
| api | Answers `503 Service Unavailable` with `Retry-After: 2`, within its `--store-timeout`. `/readyz` answers `503`, `/livez` keeps answering `200`. |

The proxy fences itself because it can't know whether its primary is still the primary. So **while the store is
down, clients can't reach PostgreSQL through the proxies**, even though PostgreSQL itself keeps running. When the
store is back, everything continues by itself: the proxies serve the primary again, and the api answers again. The
details are in [When a component can't reach the store](../architecture.md#when-a-component-cant-reach-the-store).

With the `kubernetes` backend, the same happens when the Kubernetes api server is overloaded or down, for example
during an upgrade of Kubernetes.

If the store is lost for good, don't restore it from a backup: set up a new, empty store, see
[Permanent loss of the store](../architecture.md#permanent-loss-of-the-store).
