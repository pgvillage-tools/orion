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

The source doesn't have to be an Orion cluster: any PostgreSQL primary that accepts streaming replication will do. Use
a standby cluster for:

- disaster recovery: a copy of a cluster on another site, that you promote when the first site is lost;
- a migration to Orion with (near) zero downtime: replicate from the old primary, and promote the standby cluster when
  the clients switch to it.

You create a standby cluster with the init mode `pitr`: Orion runs your restore command on one keeper, which copies
the database from the source primary (for example with `pg_basebackup`), and then starts PostgreSQL as a standby of
the source primary.

The steps on this page are the ones the end-to-end tests run for every change (`tests/system/standby_cluster_test.go`),
with a source cluster and a standby cluster on the same network.

## Before you start

- The keepers of the standby cluster must reach the primary of the source cluster on its PostgreSQL port.
- The source primary must accept replication connections of the replication user from the keepers of the standby
  cluster. When the source cluster is an Orion cluster with `defaultSUReplAccessMode` `all` (the default), it accepts
  them from every host. Otherwise add a line to its `pg_hba.conf` (for an Orion cluster: to the `pgHBA` of its spec),
  for example `host replication repluser 10.0.2.0/24 scram-sha-256`.
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
[password file](https://www.postgresql.org/docs/current/libpq-pgpass.html) on every keeper, readable only by the user
that runs PostgreSQL (`chmod 600`):

```text
<source primary>:5432:*:<replication user>:<replication password>
```

and point to it with `PGPASSFILE=<file>` in the `dataRestoreCommand` and `passfile=<file>` in `primaryConninfo`,
instead of the passwords.

`primaryConninfo` names one host. Orion sets `recovery_target_timeline` to `latest`, so the standby cluster follows a
new primary of the source cluster after a failover, but only when it can still reach it on that address. Use an
address that always reaches the current source primary: for an Orion source cluster, a proxy of the source cluster
(it forwards connections to the current primary), or a virtual IP or DNS name that moves with the primary.

Once converged, the standby cluster replicates the changes of the source primary:

```sql
-- on the source primary
CREATE TABLE standby (id int);
INSERT INTO standby VALUES (1), (2);

-- on the standby cluster, a moment later
SELECT count(*) FROM standby;  -- 2
```

It is read only: writes fail with `cannot execute INSERT in a read-only transaction`.

### What the keepers do

The leader sentinel chooses one of the registered keepers to be the master keeper of the standby cluster. That keeper
runs `dataRestoreCommand` (as for a [point in time recovery](pitr.md#what-the-keeper-does): with `/bin/sh -c`, after
removing its data directory, and again on its next check when it fails), and then starts PostgreSQL as a standby of
the source primary, with the `standbyConfig` settings.

The other keepers of the standby cluster replicate from the master keeper, not from the source primary: a cascading
setup, so only one connection crosses to the source. Apart from that, the standby cluster works like any other
cluster: when the master keeper fails, the sentinel chooses another keeper, which then replicates from the source
primary. Synchronous replication (`synchronousReplication`) isn't used in a standby cluster: the sentinel only sets it
up in a primary cluster.

## More standby settings

`standbyConfig` takes more settings than `primaryConninfo`:

| Setting | PostgreSQL parameter | What it does |
|---------|----------------------|--------------|
| `standbySettings.primaryConninfo` | `primary_conninfo` | the connection to the source primary |
| `standbySettings.primarySlotName` | `primary_slot_name` | the replication slot on the source primary to use |
| `standbySettings.recoveryMinApplyDelay` | `recovery_min_apply_delay` | applies the changes this long after the source, for example `1h` |
| `archiveRecoverySettings.restoreCommand` | `restore_command` | fetches WAL files from an archive when streaming can't |

For example, with a replication slot, so the source primary keeps the WAL the standby cluster still needs:

```json
{
  "standbyConfig": {
    "standbySettings": {
      "primaryConninfo": "host=<source primary> port=5432 user=<replication user> passfile=<file> sslmode=disable",
      "primarySlotName": "standby_cluster"
    }
  }
}
```

Orion doesn't create the slot on the source primary: create it there with
`SELECT pg_create_physical_replication_slot('standby_cluster')`. When the source is an Orion cluster, add it to the
`additionalMasterReplicationSlots` of the source cluster instead, so it is on whichever keeper is the primary; Orion
prefixes those slots with `orion_`, so the `primarySlotName` is then `orion_standby_cluster`. A slot keeps WAL on the
source primary for as long as the standby cluster doesn't fetch it: drop it (or remove it from
`additionalMasterReplicationSlots`) when you remove or promote the standby cluster.

You can change `standbyConfig` of a running standby cluster with `orion-cli update`: the master keeper restarts
PostgreSQL with the new settings.

## Promote the standby cluster

To make the standby cluster a primary cluster of its own, for example when the source cluster is lost, or to finish
a migration, change its role to `master`:

```bash
orion-cli update --patch '{"role": "master"}'
```

or with `orion-cli promote --wait` (`POST /v1/cluster/actions/promote`), which does the same.

The sentinel then tells its keeper to promote PostgreSQL. Once it did, the cluster accepts writes, and it no longer
follows the source cluster: changes on the source cluster don't reach it anymore, and changes on it don't reach the
source cluster.

A promoted cluster can't become a standby cluster again: the api refuses to change the role from `master` to
`standby`. To follow the source cluster again, create a new standby cluster.
