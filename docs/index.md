---
title: Orion
summary: Orion is a cloud native cluster manager for PostgreSQL, a fork of stolon, with a REST api and a cli
authors:
  - Sebas Mannem
date: 2026-11-08
---

# Orion

Orion is a cluster manager for PostgreSQL. It runs PostgreSQL clusters with a primary and one or more standbys, fails
over automatically when the primary fails, and makes sure clients only connect to the current primary. You can run
Orion on virtual machines and on Kubernetes.

Orion is a fork of [stolon](https://github.com/sorintlab/stolon). It keeps stolon's keeper, sentinel and proxy, and adds
a REST api with a cli on top of it, secured with TLS and client certificates.

## Components

| Component | Binary | What it does |
|-----------|--------|--------------|
| keeper | `orion-keeper` | Manages one PostgreSQL instance and converges it to the cluster view of the leader sentinel. |
| sentinel | `orion-sentinel` | Discovers and monitors the keepers and proxies, and computes the optimal cluster view. |
| proxy | `orion-proxy` | The access point for clients. It only forwards connections to the current primary, and closes connections to old primaries. |
| api | `orion-api` | Manages Orion clusters with a REST api, with TLS and client certificates (mTLS). |
| cli | `orion-cli` | Consumes the api. |

All components keep their state in a highly available store (etcd or the Kubernetes api server), so they can
run anywhere, come and go, and change their addresses.

## Documentation

- [API certificates](api/certificates.md): the certificates the api and the cli use, and how to create them.
- [Bootstrap design](api/bootstrap-design.md): the design of TLS and the bootstrap of the api.
- [Cluster spec](cluster/spec.md): the desired state of a cluster, and how to view and change it.
- [Initialization](cluster/initialization.md): create a cluster with `orion-cli init` or `--initial-cluster-spec`, in
  init mode `new`, `existing` or `pitr`.
- [Orion on Kubernetes](deploy/k8s.md): run Orion on Kubernetes, with Kubernetes as the store.
- [Failover and switchover](operations/failover-switchover.md): when the sentinel fails over, what happens to the old
  primary, and how to switch over with `orion-cli failkeeper`.
- [Point in time recovery](operations/pitr.md): archive the WAL, take base backups, and restore into a new cluster.
- [PostgreSQL parameters](operations/postgres-parameters.md): set the parameters of all instances in the cluster
  spec, and when the keeper reloads or restarts PostgreSQL.
- [Sentinel decisions](operations/sentinel-decisions.md): how the sentinel picks the synchronous standbys, switches
  the proxies, and replaces a standby that can't catch up.
- [Rejoining with pg_rewind](operations/pg-rewind.md): let an old primary rejoin with pg_rewind instead of a full copy,
  with `usePgrewind`.
- [Standby cluster](operations/standby-cluster.md): replicate from the primary of another cluster, and promote into a
  primary cluster of its own.
- [Standby resync](operations/resync.md): how a keeper gets a new standby, and `resyncFastCheckpoint`.
- [Synchronous replication](operations/sync-replication.md): commits that wait for a synchronous standby, failover,
  and a restarted primary.
- [Unresponsive standbys](operations/unresponsive-standbys.md): set `tcp_user_timeout`, so a standby that stops
  reading can't block the primary.

!!! note
    The rest of the documentation still comes from stolon and describes stolon's behaviour. Until it is reviewed and
    updated for Orion, it lives in
    [docs/stolon](https://github.com/pgvillage-tools/orion/tree/main/docs/stolon) in the repository.
