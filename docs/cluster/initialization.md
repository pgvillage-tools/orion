---
title: Initialization
summary: How an Orion cluster is created, with orion-cli init, the api, or the --initial-cluster-spec of the sentinel, and its init modes new, existing and pitr
authors:
  - Sebas Mannem
date: 2026-11-24
---

# Initialization

An Orion cluster exists once its cluster data, with a [cluster spec](spec.md), is in the store. You create it in one
of two ways:

- with `orion-cli init` (or `POST /v1/cluster` on the api);
- declaratively, with `--initial-cluster-spec` on the sentinel.

Then the sentinel initializes the cluster in its `initMode`. It picks the keeper for the first db, the primary, and
waits until that db has converged and is healthy. After that the other keepers become standbys of it.

## With orion-cli

Without a spec, `orion-cli init` creates a new cluster with the default settings (`initMode: new`):

```sh
orion-cli init --wait
```

With `--wait` it waits until the cluster is available and every healthy keeper has converged. `--wait-timeout` sets
how long it waits at most (default `5m`).

With a spec, as an argument or from a file (`--file`, `-` for stdin):

```sh
orion-cli init '{"initMode": "new", "pgParameters": {"work_mem": "8MB"}}'
orion-cli init --file spec.json --wait
```

A spec without an `initMode` gets `new`.

`orion-cli init` only creates a cluster that doesn't exist yet. When there already is one, it fails with
`cannot initialize an already existing cluster` and changes nothing. To change a running cluster, use
`orion-cli update` (see [Cluster spec](spec.md#changing-the-spec)).

### With the api

`POST /v1/cluster` takes the spec as its body:

- **An empty body** creates a new cluster with the default spec.
- **On success** the api answers 201 (Created), with the cluster data.
- **A body that isn't a valid spec**, or `null`, gets 400, and nothing is created.
- **When a cluster with a spec already exists**, the api answers 409 (Conflict). That also holds when several clients
  create the cluster at the same time: exactly one gets 201, the others get 409.

## With --initial-cluster-spec

For a declarative setup, for example with Ansible or on Kubernetes, give the sentinel the spec in a file:

```sh
orion-sentinel --initial-cluster-spec /etc/orion/cluster-spec.json ...
```

or set `ORIONSENTINEL_INITIAL_CLUSTER_SPEC`.

- **The sentinel checks the spec at its start**, and doesn't start when the file can't be read or the spec isn't
  valid.
- **It only uses the spec when there is no cluster data in the store yet.** When there is no cluster, it creates the
  cluster with that spec. Once the cluster exists, the file is ignored, also when it changes later. Change the spec with
  `orion-cli update` from then on.
- **The api has nothing left to create.** `orion-cli init` and `POST /v1/cluster` get a conflict.

## Init modes

| `initMode` | The first db | Options |
|------------|--------------|---------|
| `new` | A new, empty database (initdb) on a keeper the sentinel picks. | `newConfig` |
| `existing` | The database a keeper already has, on the keeper you name. | `existingConfig` (required) |
| `pitr` | A point in time recovery on a keeper the sentinel picks. | `pitrConfig` (required) |

The `initMode` is fixed once the cluster exists: the api refuses to change it (see
[Cluster spec](spec.md#what-the-api-checks)).

### new

The sentinel picks one of the registered keepers, which runs initdb and starts PostgreSQL as the primary. It may take
`initTimeout` (default `5m`). When it doesn't succeed within that time, the sentinel drops that db and picks a keeper
again.

`newConfig` holds the options for initdb:

| Field | initdb option |
|-------|---------------|
| `locale` | `--locale` |
| `encoding` | `--encoding` |
| `dataChecksums` | `--data-checksums` |

```json
{"initMode": "new", "newConfig": {"encoding": "UTF8", "dataChecksums": true}}
```

`new` is only for a primary cluster (`role: master`). A [standby cluster](../operations/standby-cluster.md) starts
with `pitr`.

### existing

The keeper you name in `existingConfig.keeperUID` (its `--uid`) keeps the database it already has, and becomes the
primary:

```json
{"initMode": "existing", "existingConfig": {"keeperUID": "keeper0"}}
```

That keeper must be running and registered in the store. The sentinel waits for it as long as it takes, and doesn't
pick another keeper: you chose this one.

### pitr

The sentinel picks a keeper that restores a base backup with `pitrConfig.dataRestoreCommand`, and recovers the WAL
with `pitrConfig.archiveRecoverySettings.restoreCommand`, up to the end of the archive or to the target in
`pitrConfig.recoveryTargetSettings`. See [Point in time recovery](../operations/pitr.md) for the spec and a complete
example.

## Parameters after the initialization

With `mergePgParameters: true` (the default), the sentinel merges the parameters of the new instance into the
`pgParameters` of the spec, once the first db is initialized: the parameters the instance reports, with the ones of the
spec on top. For `new` these are the parameters initdb generated; for `existing` and `pitr`, the ones the database
already had. With `false`, the cluster uses only the `pgParameters` of its spec.
