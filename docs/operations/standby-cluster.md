---
title: Standby cluster
summary: Run an Orion cluster as a standby of the primary of another cluster, and promote it into a primary cluster of its own
authors:
  - Sebas Mannem
date: 2026-11-21
---

# Standby cluster

A standby cluster is an Orion cluster with the role `standby`: it doesn't have a primary of its own, but replicates
from the primary of another cluster (the source cluster). It is read only. When you change its role to `master`, it
stops following the source cluster and becomes a primary cluster of its own.

You create a standby cluster with the init mode `pitr`: Orion runs your restore command on one keeper, which copies
the database from the source primary (for example with `pg_basebackup`), and then starts PostgreSQL as a standby of
the source primary.

The steps on this page are the ones the end-to-end tests run for every change (`tests/system/standby_cluster_test.go`),
with a source cluster and a standby cluster on the same network.

## Before you start

- The keepers of the standby cluster must reach the primary of the source cluster on its PostgreSQL port.
- The source primary must accept replication connections of the replication user from the keepers of the standby
  cluster. When the source cluster is an Orion cluster with `defaultSUReplAccessMode` `all` (the default), it accepts
  them from every host.
- The standby cluster starts from a copy of the source database, with its roles. Give the keepers of the standby
  cluster the same superuser and replication users and passwords (`--pg-su-username`, `--pg-su-passwordfile`,
  `--pg-repl-username`, `--pg-repl-passwordfile`) as the source cluster.

## Create the standby cluster

Start the keepers, sentinels, proxies and api of the standby cluster (with their own `--cluster-name`), and initialize
it with this spec, where `<source primary>` is the address of the primary of the source cluster:

```json
{
  "role": "standby",
  "initMode": "pitr",
  "pitrConfig": {
    "dataRestoreCommand": "PGPASSWORD=<replication password> pg_basebackup -h <source primary> -U <replication user> -D %d -X stream"
  },
  "standbyConfig": {
    "standbySettings": {
      "primaryConninfo": "host=<source primary> port=5432 user=<replication user> password=<replication password> sslmode=disable"
    }
  }
}
```

```bash
orion-cli init --file standby.json --wait
```

- `role: standby` makes this a standby cluster. Without it, the cluster gets the default role `master`.
- `initMode: pitr` is the only init mode for a standby cluster: the data must come from the source cluster. `new`
  would create an empty database, and the api refuses `role: standby` with `initMode: new`.
- `pitrConfig.dataRestoreCommand` copies the database from the source primary. `%d` is the data directory of the
  keeper. `-X stream` puts the WAL of the backup in the copy, so the standby needs no WAL archive to start.
- `standbyConfig.standbySettings.primaryConninfo` becomes PostgreSQL's `primary_conninfo`: the connection the standby
  uses to stream the WAL from the source primary. The api refuses a standby cluster without `standbyConfig`.

The example puts the replication password in the spec, which every user of the api can read. Prefer a
[password file](https://www.postgresql.org/docs/current/libpq-pgpass.html) on the keepers (`PGPASSFILE=...` in the
`dataRestoreCommand`, `passfile=...` in `primaryConninfo`).

Once converged, the standby cluster replicates the changes of the source primary:

```sql
-- on the source primary
CREATE TABLE standby (id int);
INSERT INTO standby VALUES (1), (2);

-- on the standby cluster, a moment later
SELECT count(*) FROM standby;  -- 2
```

It is read only: writes fail with `cannot execute INSERT in a read-only transaction`.

## Promote the standby cluster

To make the standby cluster a primary cluster of its own, for example when the source cluster is lost, or to finish
a migration, change its role to `master`:

```bash
orion-cli update --patch '{"role": "master"}'
```

The sentinel then tells its keeper to promote PostgreSQL. Once it did, the cluster accepts writes, and it no longer
follows the source cluster: changes on the source cluster don't reach it anymore, and changes on it don't reach the
source cluster.

A promoted cluster can't become a standby cluster again: the api refuses to change the role from `master` to
`standby`. To follow the source cluster again, create a new standby cluster.
