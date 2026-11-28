---
title: FAQ
summary: Frequently asked questions about Orion, with links to the pages that answer them in full
authors:
  - Sebas Mannem
date: 2026-11-28
---

# FAQ

## Why should clients connect through the proxy?

Orion chooses consistency over availability. A client must only write to the current primary: when the sentinel
elects a new primary (for example during a network partition), a client that still writes to the old one writes data
that is lost when the old primary comes back as a standby.

The proxy prevents that. It only forwards connections to the primary the sentinel elected, and closes the connections
to an old primary. Before the sentinel points the proxies to a new primary, it waits until every proxy has stopped
forwarding to the old one. See [Proxies after a new primary](operations/sentinel-decisions.md#proxies-after-a-new-primary).

A proxy that can't reach the store fences itself: after `proxyTimeout` it closes all client connections, because it
can no longer know which instance is the primary. See
[When a component can't reach the store](architecture.md#when-a-component-cant-reach-the-store).

## Why not an existing proxy, like HAProxy?

The proxy has to close the connections to an old primary right away, follow keepers whose addresses change, and
report to the sentinel that it stopped forwarding, all through the store. A general purpose proxy doesn't do that by
itself. You can still put a load balancer (or a virtual IP) in front of the Orion proxies: with `--stop-listening`
(the default) a fenced proxy stops listening, so the load balancer sees it as down.

## Does the proxy send read-only queries to the standbys?

No. The proxy forwards all connections to the primary. To read from a standby, connect to it directly.

## Why doesn't Orion need shared storage or fencing?

Every keeper has its own data directory, and the standbys replicate with PostgreSQL streaming replication, so there
is no shared storage to protect. Instead of fencing a node (powering it off, or taking away its storage), Orion
relies on:

- the store, to decide which components are healthy and which instance is the primary;
- the proxies, which only send clients to the elected primary, and fence themselves when they lose the store;
- the keepers, which step down an old primary when it comes back (see
  [The old primary](operations/failover-switchover.md#the-old-primary)).

## How does Orion differ from Kubernetes alone, or from Pacemaker?

Kubernetes alone restarts a PostgreSQL pod with its persistent volume, possibly on another node. That depends on how
fast Kubernetes detects a failed node and moves the pod and its volume, and a volume is often tied to one
availability zone. Pacemaker with the PostgreSQL resource agent gives high availability with shared storage or
streaming replication, but needs node fencing.

Orion uses streaming replication between keepers that each have their own data, so:

- the sentinel fails over within seconds (`failInterval`, 20s by default, see
  [Failover](operations/failover-switchover.md#failover));
- a standby can run in another availability zone, or on local disks;
- no node fencing is needed (see above);
- with [synchronous replication](operations/sync-replication.md), a commit that returned is not lost on a failover.

On top of that, Orion manages the cluster through the [cluster spec](cluster/spec.md):
[PostgreSQL parameters](operations/postgres-parameters.md), [pg_hba entries](operations/pg-hba.md),
[point in time recovery](operations/pitr.md) and [standby clusters](operations/standby-cluster.md). It runs on virtual
machines and on [Kubernetes](deploy/k8s.md).

## What happens when the store is partitioned?

A component that can't read from or write to the store doesn't act on old data. The keeper leaves PostgreSQL as it is,
the sentinel makes no decisions, the api answers `503`, and the proxy closes its client connections after
`proxyTimeout`. With etcd, the side of a partition without a majority of the etcd members can't write, so its proxies
fence themselves. See [When a component can't reach the store](architecture.md#when-a-component-cant-reach-the-store).

Run at least three etcd members, so the store survives the loss of one.

## How are backups handled?

Orion doesn't take backups itself. You archive the WAL with PostgreSQL's `archive_command` and take base backups with
the tool of your choice, for example wal-g. To restore, you create a new cluster with init mode `pitr`. See
[Point in time recovery](operations/pitr.md).

## Which standby does the sentinel promote?

The sentinel elects a standby that runs on a healthy keeper, has a healthy and converged database, and is on the
timeline of the primary; not one of a keeper started with `--can-be-master=false`.

- Without synchronous replication, the standby must be at most `maxStandbyLag` behind the last position the primary
  reported. Of those, the sentinel elects the one that is furthest ahead, so the least data is lost. The primary is
  down, so the sentinel can't know its real last position: a failover can lose the last transactions.
- With synchronous replication, the sentinel only elects a synchronous standby. When there is none, it doesn't fail
  over.

See [Failover](operations/failover-switchover.md#failover) and [Sentinel decisions](operations/sentinel-decisions.md).

PostgreSQL commits waiting transactions when a primary restarts, also when no synchronous standby has them. The keeper
closes that gap: see [A primary that restarts](operations/sync-replication.md#a-primary-that-restarts).

## Does Orion use the quorum methods of synchronous replication (FIRST or ANY)?

No. The sentinel picks the synchronous standbys itself, between `minSynchronousStandbys` and `maxSynchronousStandbys`
of the cluster spec, and the keeper sets `synchronous_standby_names` so that **all** of them must confirm a commit
(`2 (a,b)` for two standbys). With `FIRST 2 (a,b,c)` or `ANY 2 (a,b,c)`, the sentinel couldn't know which standbys
have a commit when the primary fails, and could elect one that misses it.

The downside: one failed synchronous standby blocks commits until the sentinel replaces it. See
[Synchronous standbys](operations/sentinel-decisions.md#synchronous-standbys) and
[Availability and minSynchronousStandbys](operations/sync-replication.md#availability-and-minsynchronousstandbys).

## Do several Orion clusters need a store each?

No. With etcd, every cluster lives under `<store-prefix>/<cluster-name>`, so several clusters can share one etcd
cluster: give each a unique `--cluster-name`. Keep the store close to the clusters (the same region or data center).
On Kubernetes, a cluster name must be unique within its namespace. See [The store](architecture.md#the-store).

## Why does Orion have an api?

The api lets you manage clusters with `orion-cli` (or any HTTP client) without access to the store: the cli talks
REST to the api, secured with TLS and client certificates, and only the api needs the store options. See
[Certificates](api/certificates.md).

## Does the cluster need the api to keep running?

No. The api only reads and changes the cluster data in the store. The keepers, sentinels and proxies run the cluster,
and fail over, without it. Without the api, you can't change the cluster with `orion-cli`. See
[Architecture](architecture.md).

## What happens when a certificate of the api or the cli expires?

The cli can't connect to the api anymore. Renew the certificate with `orion-cli pki renew`: the CA stays the same, so
nothing else changes. Don't run `pki init --force` or a new bootstrap: that creates a new CA. `orion-cli pki list`
shows when each certificate expires, and the api exports the expiry of its certificate as a metric. See
[Renewing the certificates](api/certificates.md#renewing-the-certificates).
