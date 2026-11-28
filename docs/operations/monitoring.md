---
title: Monitoring
summary: The probes and the Prometheus metrics of the keeper, the sentinel, the proxy and the api, and what to alert on
authors:
  - Sebas Mannem
date: 2026-11-28
---

# Monitoring

Every Orion component serves Prometheus metrics over plain HTTP, without authentication. The api also has probes for
Kubernetes and load balancers. This page lists what each component serves, and what to alert on.

| Component | Where | Default | Endpoints |
|-----------|-------|---------|-----------|
| keeper | `--metrics-listen-address` (`ORIONKEEPER_METRICS_LISTEN_ADDRESS`) | disabled | `/metrics` |
| sentinel | `--metrics-listen-address` (`ORIONSENTINEL_METRICS_LISTEN_ADDRESS`) | disabled | `/metrics` |
| proxy | `--metrics-listen-address` (`ORIONPROXY_METRICS_LISTEN_ADDRESS`) | disabled | `/metrics` |
| api | `--probe-port` (`ORIONAPI_PROBE_PORT`), on `--baseurl` | `8080` | `/livez`, `/readyz`, `/status`, `/metrics` |

- **Keeper, sentinel and proxy:** set `--metrics-listen-address` to an address and port, e.g. `0.0.0.0:8080`. When the
  address can't be used (e.g. the port is taken), the component stops.
- **The api** always serves its probe port; it doesn't use `--metrics-listen-address`. See
  [Probe port](../api/certificates.md#probe-port) for the probes, and [Metrics](../api/certificates.md#metrics) for
  its metrics.

All flags are in the [command reference](../reference/commands/index.md).

## Probes

Only the api has probes:

| Endpoint | Returns |
|----------|---------|
| `/livez` | 200 as long as the api process serves requests. It doesn't check the store. |
| `/readyz` | 200 when the api serves, isn't shutting down and its store answers within a second; 503 otherwise. |
| `/status` | The mode of the api: `{"mode":"normal"}`, or `{"mode":"bootstrap"}`. It doesn't depend on the store. |

A store outage makes the api not ready, but keeps it live: restarting the api wouldn't fix the store. On SIGTERM,
`/readyz` returns 503 for `--shutdown-delay` (5 seconds by default) before the api stops, so load balancers stop
sending requests first. Details and the Kubernetes probes: [Probe port](../api/certificates.md#probe-port).

The keeper, the sentinel and the proxy have no HTTP probes. In Kubernetes, the
[example manifests](../deploy/k8s.md) check the proxy with a `tcpSocket` readiness probe on its PostgreSQL port.

## Metrics of all components

Every component serves:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `orion_cluster_identifier` | gauge | `cluster_name`, `component` | Always 1. `component` is `keeper`, `sentinel`, `proxy` or `api`. Use it to join the other metrics to a cluster (see below). |
| `go_*`, `process_*` | | | The standard Go and process metrics, e.g. `process_start_time_seconds`. |

### Keeper

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `orion_keeper_local_role` | gauge | `role` | 1 for the current role of the instance, 0 for the others. `role` is `master`, `standby` or `undefined`. |
| `orion_keeper_target_role` | gauge | `role` | 1 for the role the sentinel asked for last, 0 for the others. |
| `orion_keeper_last_sync_success_seconds` | gauge | | When the keeper last converged its instance to the cluster data without errors, in seconds since the unix epoch. |
| `orion_keeper_clusterdata_last_valid_update_seconds` | gauge | | When the keeper last read valid cluster data from the store, in seconds since the unix epoch. |
| `orion_keeper_needs_reload` | gauge | | 1 when PostgreSQL needs a reload and the reload failed. |
| `orion_keeper_needs_restart` | gauge | | 1 when PostgreSQL needs a restart for a changed parameter, and it didn't restart (yet). With `automaticPgRestart` off, it stays 1 until you restart PostgreSQL. |
| `orion_keeper_sleep_interval` | gauge | | The `sleepInterval` of the cluster spec, in seconds: the time between two checks. |
| `orion_keeper_shutdown_seconds` | gauge | | When the keeper got a termination signal, in seconds since the unix epoch. |

Until the first successful check, both role metrics are 0 for every role, and the timestamps are 0. See
[PostgreSQL parameters](postgres-parameters.md) for when the keeper reloads or restarts PostgreSQL.

### Sentinel

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `orion_sentinel_is_leader` | gauge | | 1 when this sentinel is the leader. |
| `orion_sentinel_leader_count` | gauge | | How often this sentinel was elected leader since it started. |
| `orion_sentinel_last_cluster_check_success_seconds` | gauge | | When this sentinel last finished a check of the cluster without errors, in seconds since the unix epoch. |

Only the leader checks the cluster, so only the leader updates `orion_sentinel_last_cluster_check_success_seconds`.
On the other sentinels it stays at the time they were leader last, or 0.

### Proxy

The proxy has no metrics of its own: only `orion_cluster_identifier` and the Go and process metrics. Use `up` to see
whether it runs.

## Scraping with Prometheus

Scrape each component on its metrics address, and the api on its probe port:

```yaml
scrape_configs:
  - job_name: orion
    static_configs:
      - targets:
          - db1.example.com:8080   # keeper
          - db2.example.com:8080   # keeper
          - mon1.example.com:8081  # sentinel
          - mon1.example.com:8082  # proxy
          - orion-api.example.com:8080  # api (probe port)
```

On Kubernetes, the [example manifests](../deploy/k8s.md) set `METRICS_LISTEN_ADDRESS` to `0.0.0.0:8080` for the keeper,
the sentinel and the proxy, and name that container port `metrics`; the api serves its metrics on its probe port
`8080`. Scrape those ports with your usual pod discovery.

When one Prometheus scrapes several clusters, join a metric to its cluster with `orion_cluster_identifier`, which has
the same `instance` label:

```promql
orion_sentinel_is_leader * on (instance) group_left (cluster_name) orion_cluster_identifier
```

## What to alert on

These are examples; tune the times to your `sleepInterval` (5 seconds by default) and your environment. The
expressions assume one cluster; with several, add the join above and `by (cluster_name)`.

```yaml
groups:
  - name: orion
    rules:
      - alert: OrionComponentDown
        expr: up{job="orion"} == 0
        for: 2m
      - alert: OrionNoSentinelLeader
        # no leader, or two that both think they are
        expr: sum(orion_sentinel_is_leader) != 1
        for: 1m
      - alert: OrionSentinelNotChecking
        # the leader hasn't finished a check of the cluster for a minute
        expr: time() - max(orion_sentinel_last_cluster_check_success_seconds) > 60
        for: 1m
      - alert: OrionNoSinglePrimary
        expr: sum(orion_keeper_local_role{role="master"}) != 1
        for: 1m
      - alert: OrionKeeperNotSyncing
        # also fires for a keeper that never synced (0)
        expr: time() - orion_keeper_last_sync_success_seconds > 60
        for: 1m
      - alert: OrionKeeperNoClusterData
        # the keeper can't read valid cluster data from the store
        expr: time() - orion_keeper_clusterdata_last_valid_update_seconds > 60
        for: 1m
      - alert: OrionKeeperRoleMismatch
        # the instance doesn't get the role the sentinel asked for
        expr: orion_keeper_local_role != orion_keeper_target_role
        for: 5m
      - alert: OrionPostgresNeedsRestart
        expr: orion_keeper_needs_restart == 1
        for: 10m
      - alert: OrionAPINotReady
        # needs a blackbox exporter, or a load balancer health check, on /readyz of the probe port
        expr: probe_success{job="orion-api-readyz"} == 0
        for: 2m
```

- **No (single) leader or primary:** during a failover, there is briefly no primary; `for: 1m` rides that out. See
  [Failover and switchover](failover-switchover.md) and [Sentinel decisions](sentinel-decisions.md).
- **Stale timestamps on all keepers and sentinels at once** usually mean the store is down or unreachable: the api is
  then not ready either.
- **Certificates of the api:** alert on `orion_api_certificate_expiry_timestamp_seconds`, see
  [Metrics](../api/certificates.md#metrics), and renew them with
  [orion-cli pki renew](../api/certificates.md#renewing-the-certificates).

## Tests

- `tests/system/probe_livez_test.go`, `probe_readyz_test.go` and `probe_status_test.go` check the probes of the api,
  also while the store doesn't answer and on SIGTERM.
- `tests/system/probe_metrics_test.go` checks the request and certificate metrics of the api, and that it serves the
  Go, process and `orion_cluster_identifier` metrics.
- The metrics of the keeper, the sentinel and the proxy have no end-to-end test yet.
