---
title: Cluster spec
summary: The cluster spec, the desired state of an Orion cluster, and how to view and change it with orion-cli and the api
authors:
  - Sebas Mannem
date: 2026-11-24
---

# Cluster spec

Orion is declarative: the cluster spec is the desired state of the cluster. It lives in the cluster data in the store.
The sentinel reads it and works out what each keeper should do. The keepers then converge their PostgreSQL instances
to it: the parameters, the replication, the pg_hba.conf entries.

You create the spec with the cluster (see [Initialization](initialization.md)). After that you change it with
`orion-cli update` or the api, and the cluster follows it.

## Viewing the spec

```sh
orion-cli spec              # the fields that are set
orion-cli spec --defaults   # every field, with the defaults filled in
```

`orion-cli spec` prints the spec as JSON on stdout. It also prints the ETag of the spec on stderr; see
[Changing the spec safely](#changing-the-spec-safely).

## Changing the spec

There are two ways to change the spec.

**Patch** the spec with a JSON merge patch ([RFC 7386](https://tools.ietf.org/html/rfc7386)), which changes only what
the patch names:

```sh
orion-cli update --patch '{"pgParameters": {"log_min_duration_statement": "1500"}}'
```

In a merge patch, objects are merged, `null` removes a field, and anything else replaces the field. So this removes
the parameter again, and leaves the rest of the spec as it is:

```sh
orion-cli update --patch '{"pgParameters": {"log_min_duration_statement": null}}'
```

**Replace** the whole spec, which is `orion-cli update` without `--patch`. Every field you leave out falls back to its
default. The replacement must still have the same `initMode`: get the current spec with `orion-cli spec`, change it,
and send all of it back.

Both forms take the spec or the patch as an argument, or from a file with `--file` (`-f`). Use `-f -` to read it from
stdin.

With `--wait`, `orion-cli update` waits until the cluster has applied the change: the cluster is available, and every
healthy keeper has converged to the new spec. `--wait-timeout` sets how long it waits at most (default `5m`).

```sh
orion-cli update --patch --wait '{"sleepInterval": "6s"}'
```

### What the api checks

The api validates every new spec before it stores it:

- **An invalid spec is refused (400).** Examples: a negative duration, `proxyCheckInterval` not less than
  `proxyTimeout`, `maxSynchronousStandbys` lower than `minSynchronousStandbys`, a pg_hba.conf entry with a newline,
  invalid JSON, or `null` as the whole spec.
- **Some changes aren't allowed on a running cluster (409):**
  - changing the `initMode`;
  - changing the `role` from `master` to `standby`. The other direction promotes a standby cluster; see
    [Standby cluster](../operations/standby-cluster.md).
- **A refused change changes nothing.** The spec stays as it was.

### Changing the spec safely

Two clients that read and then update the spec at the same time could overwrite each other's change. To prevent that,
pass the ETag that `orion-cli spec` printed:

```sh
orion-cli spec > spec.json          # prints "ETag: ..." on stderr
orion-cli update --if-match '"<etag>"' -f spec.json
```

The api then only applies the update while the spec still has that ETag. If someone changed it in between, the api
answers 412 (Precondition Failed) and your update changes nothing: read the spec again and redo your change.

## The api

| Method and path | What it does |
|-----------------|--------------|
| `GET /v1/cluster/spec` | Returns the spec, with its `ETag`. 404 when there is no cluster yet. |
| `PATCH /v1/cluster/spec` | Applies a JSON merge patch (`Content-Type: application/merge-patch+json`). |
| `PUT /v1/cluster/spec` | Replaces the spec. |
| `POST /v1/cluster` | Creates the cluster with a spec; see [Initialization](initialization.md). |
| `GET /v1/cluster` / `PUT /v1/cluster` | Reads or replaces all the cluster data, the spec included. |

`PATCH` and `PUT` on the spec take an optional `If-Match` header with the ETag of the spec. The ETag of the spec only
changes when the spec changes. The cluster data as a whole has its own ETag, which changes with every write to it,
also the ones the sentinel does all the time.

## Fields

All fields are optional, except `initMode`. Durations are strings like `"5s"`, `"1m30s"` or `"48h"`.

| Field | Default | What it does |
|-------|---------|--------------|
| `sleepInterval` | `5s` | How long the keepers and the sentinels wait between two checks. |
| `requestTimeout` | `10s` | Timeout of a request to the store or to PostgreSQL. |
| `convergenceTimeout` | `30s` | How long a db may take to converge to its spec when no long operation is expected (e.g. a promotion). |
| `initTimeout` | `5m` | How long the initdb of a new cluster may take. |
| `syncTimeout` | `0` (no limit) | How long a db may take to sync with its primary, including the WAL replay of a point in time recovery. |
| `dbWaitReadyTimeout` | `60s` | How long PostgreSQL may take to become ready after a start. |
| `failInterval` | `20s` | How long after the first failure a keeper or db counts as unhealthy. |
| `deadKeeperRemovalInterval` | `48h` | After how long a dead keeper without a db is removed from the cluster data. |
| `proxyCheckInterval` | `5s` | How long the proxies wait between two checks. |
| `proxyTimeout` | `15s` | Within this time a proxy must complete a check, or it closes its connections to the primary. |
| `maxStandbys` | `20` | Maximum number of standbys. |
| `maxStandbysPerSender` | `3` | Maximum number of standbys of one sender (the primary). |
| `maxStandbyLag` | `1048576` (1 MiB) | Maximum lag in bytes of an asynchronous standby that can still replace a failed primary. |
| `synchronousReplication` | `false` | Use synchronous replication between the primary and its standbys. |
| `minSynchronousStandbys` | `1` | Minimum number of synchronous standbys. |
| `maxSynchronousStandbys` | `1` | Maximum number of synchronous standbys. |
| `additionalWalSenders` | `5` | WAL senders on top of the ones Orion needs itself. |
| `additionalMasterReplicationSlots` | none | Extra physical replication slots on the primary. |
| `usePgrewind` | `false` | Try pg_rewind before pg_basebackup when a standby resyncs; see [Standby resync](../operations/resync.md). |
| `resyncFastCheckpoint` | `false` | Ask for an immediate checkpoint when a standby resyncs with pg_basebackup; see [Standby resync](../operations/resync.md). |
| `initMode` | (required) | How the cluster is initialized: `new`, `existing` or `pitr`; see [Initialization](initialization.md). |
| `mergePgParameters` | `true` | After the initialization, merge the parameters of the new instance into `pgParameters`. |
| `role` | `master` | `master` for a primary cluster, `standby` for a [standby cluster](../operations/standby-cluster.md). |
| `newConfig` | none | Options for `initMode: new`. |
| `existingConfig` | none | Options for `initMode: existing`. |
| `pitrConfig` | none | Options for `initMode: pitr`; see [Point in time recovery](../operations/pitr.md). |
| `standbyConfig` | none | Required for `role: standby`; see [Standby cluster](../operations/standby-cluster.md). |
| `defaultSUReplAccessMode` | `all` | Where the superuser and replication users may connect from: `all` hosts, or `strict` (only the other keepers, to the primary). |
| `pgParameters` | none | PostgreSQL parameters for all instances. |
| `pgHBA` | none | Extra pg_hba.conf entries. |
| `automaticPgRestart` | `false` | Restart PostgreSQL by itself when a changed parameter needs a restart. |

## What a change does

The sentinel and the keepers read the spec on each check (`sleepInterval`), so a change takes effect within a few
seconds, without a restart of the Orion components:

- **Timing fields** (`sleepInterval`, `requestTimeout`, the timeouts and intervals): from the next check on.
- **`pgParameters`**: the keepers write them to the `postgresql.conf` of each instance and reload PostgreSQL. The e2e
  tests check this: `work_mem` and `log_min_duration_statement` change on the running instance. A parameter that only
  takes effect after a restart waits for a restart of PostgreSQL, or is applied with one when `automaticPgRestart` is
  true.
- **`pgHBA`** and `defaultSUReplAccessMode`: the keepers write a new pg_hba.conf and reload PostgreSQL.
- **Replication fields** (`synchronousReplication`, the number of standbys): the sentinel changes the cluster view,
  and the keepers follow it.
- **`initMode`** can't change, and `role` can only go from `standby` to `master` (see
  [What the api checks](#what-the-api-checks)).
