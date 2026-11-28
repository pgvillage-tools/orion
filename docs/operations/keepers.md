---
title: Managing keepers
summary: Add a keeper to a cluster, remove a keeper that is gone for good, and read the keepers, proxies and sentinels in orion-cli status and the api
authors:
  - Sebas Mannem
date: 2026-11-28
---

# Managing keepers

Every keeper manages one PostgreSQL instance. The sentinel decides which keeper runs the primary and which run a
standby; you add a keeper by starting it, and remove one that is gone for good with `orion-cli removekeeper`.

## Adding a keeper

Start `orion-keeper` with the same cluster name and store as the other components:

```bash
orion-keeper --cluster-name orion-cluster --store-backend etcdv3 --store-endpoints https://etcd-0:2379 \
  --uid keeper2 --data-dir /var/lib/orion/keeper2 \
  --pg-listen-address 10.0.0.12 --pg-repl-username repluser --pg-repl-passwordfile /etc/orion/repl-password \
  --pg-su-passwordfile /etc/orion/su-password
```

- **`--uid`** is the name of the keeper in the cluster: unique, with lower-case letters, numbers and `_` only.
  Without `--uid`, the keeper generates one on its first start. Either way it keeps its uid in
  `<data-dir>/keeperstate`, and refuses to start with another `--uid` than the one it saved.
- **`--data-dir`** holds the state of the keeper and, in `<data-dir>/postgres`, the PostgreSQL data directory. Give
  every keeper its own data directory on a persistent volume: the keeper locks it, so two keepers can't use the same
  one.
- **`--can-be-master=false`** keeps the keeper from ever being elected as the primary, and
  **`--can-be-synchronous-replica=false`** from being chosen as a synchronous standby (see
  [Synchronous replication](sync-replication.md)).

The keeper registers itself in the store, and the sentinel adds it to the cluster data. When the primary has fewer
standbys than `maxStandbysPerSender` (3 by default, see the [cluster spec](../cluster/spec.md)), the sentinel gives the
new keeper a database: a standby of the primary, which the keeper copies from the primary (see
[Standby resync](resync.md)). With enough standbys, the keeper waits without a database, as a spare: the sentinel uses
it when a standby fails.

The options of `orion-keeper` are in the [command reference](../reference/commands/orion-keeper.md); on Kubernetes, the
keepers are a StatefulSet (see [Orion on Kubernetes](../deploy/k8s.md)).

## Keepers, proxies and sentinels in the status

`orion-cli status` shows the components of the cluster, and the keeper of the primary:

```text
=== Active sentinels ===

ID              LEADER
1a2b3c4d        true
5e6f7a8b        false

=== Active proxies ===

ID
9c0d1e2f

=== Keepers ===

UID      HEALTHY  PG LISTENADDRESS  PG HEALTHY  PG WANTEDGENERATION  PG CURRENTGENERATION
keeper0  true     10.0.0.10:5432    true        3                    3
keeper1  true     10.0.0.11:5432    true        2                    2
keeper2  true     (no db assigned)  false       0                    0

=== Cluster Info ===

Cluster role: master
Master Keeper: keeper0

===== Keepers/DB tree =====

keeper0 (master)
└─keeper1
```

| Column | Meaning |
|--------|---------|
| Sentinel `LEADER` | The sentinel that leads: only the leader changes the cluster data. The others take over when it stops. |
| Proxy `ID` | A proxy that runs. A stopped proxy drops out of the list after 2 proxy timeouts (30s by default). |
| `HEALTHY` | The keeper reported to the store within `failInterval` (20s by default). |
| `PG LISTENADDRESS` | The address of its PostgreSQL; `(no db assigned)` for a keeper without a database (a spare), `(unknown)` until its database reported it. |
| `PG HEALTHY` | Its PostgreSQL runs as the sentinel wants it. |
| `PG WANTEDGENERATION`, `PG CURRENTGENERATION` | The sentinel raises the wanted generation of a database each time it changes what the keeper should do with it (e.g. its parameters, or which primary it follows); the keeper sets the current generation when it has done so. They differ while the keeper applies a change. |

Sentinels also drop out of the list when they stop (after 2 sleep intervals, 10s by default). Without a primary, the
cli prints `No cluster available` instead of the cluster info. `orion-cli status --format json` prints the status as
the api returns it.

Over the api, with the client certificate of the cli (see [API certificates](../api/certificates.md)):

| Request | Answer |
|---------|--------|
| `GET /v1/cluster/status` | The keepers, proxies and sentinels, and `status`: `available`, `role`, and the `primaryDB` and `primaryKeeper` when the cluster has a primary. |
| `GET /v1/cluster/keepers` | The keepers, sorted on uid: `uid`, `listenAddress`, `healthy`, `pgHealthy`, `pgWantedGeneration`, `pgCurrentGeneration`. |
| `GET /v1/cluster/proxies` | The proxies that run, sorted on uid: `uid`, and `generation`, the generation of the proxy configuration it applied (the sentinel raises it when it points the proxies to another primary). |
| `GET /v1/cluster/sentinels` | The sentinels that run, sorted on uid: `uid` and `leader`. |
| `GET /v1/cluster/keepers/{uid}`, `GET /v1/cluster/proxies/{uid}`, `GET /v1/cluster/sentinels/{uid}` | One of them, or `404 Not Found`. |

Without a cluster, the lists are empty. While the store doesn't respond, the api answers `503 Service Unavailable`.

## Removing a keeper

A keeper stays in the cluster data when it stops: it may come back. Remove a keeper that is gone for good (e.g. its
node is lost) with:

```bash
orion-cli removekeeper keeper1 --wait
```

or over the api: `DELETE /v1/cluster/keepers/{uid}` (`202 Accepted`). It removes the keeper, and its database, from
the cluster data; with `--wait`, the cli waits until the cluster is available and every healthy keeper applied the
change (at most `--wait-timeout`, 5 minutes by default).

- **The keeper of the primary can't be removed** (`409 Conflict`): fail it first with `orion-cli failkeeper` (see
  [Failover and switchover](failover-switchover.md#switchover)), and remove it once another keeper runs the primary.
- **A keeper that still runs registers itself again**, and the sentinel adds it back. Stop the keeper first.
- An unknown keeper, or a store without a cluster, gives `404 Not Found`.
- When the cluster data changes while the keeper is removed, the api answers `409 Conflict`: try again.

The sentinel also removes a keeper by itself: one without a database that has been dead for longer than
`deadKeeperRemovalInterval` (48h by default). A keeper that had a standby is not removed by the sentinel, but once
the primary has `maxStandbysPerSender` good standbys again (e.g. on a spare keeper), the sentinel drops the database
of the dead keeper.

## When a keeper loses its primary

What happens when the keeper of the primary crashes, freezes, or loses its data directory, is in
[Failover and switchover](failover-switchover.md#failover): the sentinel fails over to a standby, and the old keeper
comes back as a standby. A standby that stops reading its replication connection is in
[Unresponsive standbys](unresponsive-standbys.md).

## Tested

| Test | What it checks |
|------|----------------|
| `tests/system/v1_delete_keeper_test.go` | Removing the keeper of the primary is refused (409); a stopped keeper is removed with its database; a keeper that runs registers itself again; 404 for an unknown keeper or without a cluster; 503 while the store doesn't respond. |
| `tests/system/v1_members_test.go` | The keepers in the same order as in the status; every keeper, proxy and sentinel by uid; 404 for an unknown one. |
| `tests/system/v1_proxies_test.go` | A stopped proxy drops out of the list. |
| `tests/system/v1_sentinels_test.go` | A second sentinel is listed; when the leader stops, it drops out and the other one leads; no sentinels when none runs. |
| `tests/system/v1_fail_keeper_test.go` | Failing a standby keeps the primary; failing the primary fails over. |
