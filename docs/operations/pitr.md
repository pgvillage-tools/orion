---
title: Point in time recovery
summary: Archive the WAL of an Orion cluster, back it up, and restore it into a new cluster, completely or up to a point in time
authors:
  - Sebas Mannem
date: 2026-11-20
---

# Point in time recovery

Orion doesn't take backups itself. You archive the WAL of a cluster with PostgreSQL's `archive_command`, and take base
backups with the tool of your choice. To restore, you create a new cluster with the init mode `pitr`: Orion runs your
restore command on one keeper, PostgreSQL replays the archived WAL (completely, or up to a target you choose), and the
restored database becomes the primary of the new cluster.

The steps on this page are the ones the end-to-end tests run for every change (`tests/system/pitr_test.go`), with a
volume that every keeper mounts at `/archive` as the backup location. Use your own backup storage and tools instead;
only the commands change.

## Archive the WAL

Set `archive_mode` and `archive_command` in the `pgParameters` of the cluster spec:

```bash
orion-cli update --patch '{
  "pgParameters": {
    "archive_mode": "on",
    "archive_command": "test ! -f /archive/wal/%f && cp %p /archive/wal/%f"
  }
}'
```

The keepers apply the parameters to every instance, so whichever keeper becomes the primary archives to the same
location. The archive must therefore be reachable from every keeper (a shared volume, NFS, an object store, ...). With
`archive_mode` `on` only the primary archives: the standbys don't, so they don't archive the same WAL files again.
Use `always` only if the standbys must archive too, and make sure they don't overwrite each other's files then.

The command must fail when the file already exists (`test ! -f`), and only return 0 when the WAL file is safely stored;
see [continuous archiving](https://www.postgresql.org/docs/current/continuous-archiving.html) in the PostgreSQL
documentation.

## Take a base backup

Take base backups at the times and with the scheduler of your choice. For example with `pg_basebackup` from the
primary, as the replication user (`--pg-repl-username` of the keepers):

```bash
pg_basebackup -h <primary> -U <replication user> -D /archive/base -X none -c fast
```

With `-X none` the backup holds no WAL: a restore then needs the archived WAL. `pg_basebackup` waits until the WAL of
the backup is archived, so the backup is complete once it returns.

## Restore into a new cluster

A restore creates a new cluster: `orion-cli init` (`POST /v1/cluster`) refuses a cluster that already exists. Start
the keepers, sentinels, proxies and api of the new cluster (with their own `--cluster-name`), with the backup and the
archive reachable from the keepers, and initialize it with this spec:

```json
{
  "initMode": "pitr",
  "pitrConfig": {
    "dataRestoreCommand": "cp -a /archive/base/. %d && chmod 700 %d",
    "archiveRecoverySettings": {
      "restoreCommand": "cp /archive/wal/%f %p"
    }
  },
  "mergePgParameters": false
}
```

```bash
orion-cli init --file pitr.json --wait
```

- `dataRestoreCommand` restores the base backup. `%d` is the data directory of the keeper. PostgreSQL refuses a data
  directory that others can read, hence the `chmod 700`.
- `archiveRecoverySettings.restoreCommand` becomes PostgreSQL's `restore_command`, which fetches the archived WAL
  files (`%f`) to `%p`.
- `mergePgParameters: false` makes the new cluster use the `pgParameters` of its own spec only. With `true` it would
  keep the parameters of the backed up cluster, including its `archive_command`: the restored cluster would then
  archive into the archive of the backed up cluster.

The restored database keeps the roles of the backed up cluster: give the keepers of the new cluster the same
superuser and replication passwords (`--pg-su-passwordfile`, `--pg-repl-passwordfile`) as the backed up cluster.

Once converged, the new cluster is a primary cluster of its own, on a new timeline: it doesn't change the backed up
cluster or its archive. Keepers you add get their data from its primary, as in any other cluster.

### What the keeper does

The leader sentinel chooses one of the registered keepers to restore the database. That keeper:

1. Removes its data directory.
2. Runs `dataRestoreCommand` with `/bin/sh -c`, with `%d` replaced by its data directory, `%w` by its WAL directory
   (`--wal-dir`, when set) and `%%` by `%`. The command must exit 0 only when the restore succeeded.
3. Starts PostgreSQL in archive recovery, with `restore_command` and the recovery target settings, and waits until the
   recovery is done. With a `syncTimeout` in the cluster spec it gives up after that time; by default it waits as
   long as the recovery takes.
4. Becomes the primary of the new cluster. The other keepers then get their data from it.

When a step fails, the keeper logs the error, removes the half restored data directory and tries again on its next
check. Look in the keeper's log for errors of `dataRestoreCommand`, and in the PostgreSQL log for errors of
`restore_command`. The keeper keeps the `pitrConfig` it got when the restore started: changing the cluster spec
doesn't change a restore in progress, so fix what the command needs (files, permissions, credentials) instead.

## Restore up to a point in time

Without a target, PostgreSQL replays all the archived WAL (Orion sets `recovery_target_timeline` to `latest`). To stop
earlier, add `recoveryTargetSettings` to the
`pitrConfig`. For example up to a restore point, created on the backed up cluster with
`SELECT pg_create_restore_point('before_3')`:

```json
{
  "initMode": "pitr",
  "pitrConfig": {
    "dataRestoreCommand": "cp -a /archive/base/. %d && chmod 700 %d",
    "archiveRecoverySettings": {
      "restoreCommand": "cp /archive/wal/%f %p"
    },
    "recoveryTargetSettings": {
      "recoveryTargetName": "before_3"
    }
  },
  "mergePgParameters": false
}
```

The settings are PostgreSQL's
[recovery target settings](https://www.postgresql.org/docs/current/runtime-config-wal.html#RUNTIME-CONFIG-WAL-RECOVERY-TARGET):

| Setting | PostgreSQL parameter | Stops at |
|---------|----------------------|----------|
| `recoveryTarget` | `recovery_target` | `immediate`: as soon as the backup is consistent |
| `recoveryTargetName` | `recovery_target_name` | a restore point created with `pg_create_restore_point()` |
| `recoveryTargetTime` | `recovery_target_time` | a timestamp |
| `recoveryTargetXid` | `recovery_target_xid` | a transaction id |
| `recoveryTargetLsn` | `recovery_target_lsn` | a WAL location |
| `recoveryTargetTimeline` | `recovery_target_timeline` | the timeline to recover into |

With a target, Orion sets `recovery_target_action` to `promote`: once PostgreSQL reaches the target it becomes the
primary of the new cluster.

## With wal-g

[wal-g](https://github.com/wal-g/wal-g) takes base backups and archives WAL to object stores (S3, GCS, Azure, ...).
Keep its credentials out of the cluster spec and the PostgreSQL parameters, which every database user can read: put
them in environment files on the keepers, and load them with `envdir` (or a small script that sets them):

```bash
orion-cli update --patch '{
  "pgParameters": {
    "archive_mode": "on",
    "archive_command": "envdir /etc/wal-g.d/env wal-g wal-push %p"
  }
}'
```

Take base backups with `wal-g backup-push` on the primary. Restore the latest one into a new cluster with:

```json
{
  "initMode": "pitr",
  "pitrConfig": {
    "dataRestoreCommand": "envdir /etc/wal-g.d/env wal-g backup-fetch %d LATEST",
    "archiveRecoverySettings": {
      "restoreCommand": "envdir /etc/wal-g.d/env wal-g wal-fetch %f %p"
    }
  },
  "mergePgParameters": false
}
```

A wal-g backup doesn't hold `postgresql.conf` and `pg_hba.conf`. Orion writes both itself, so put the PostgreSQL
parameters the restored cluster needs in the `pgParameters` of its spec.
