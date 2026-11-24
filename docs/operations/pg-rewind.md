---
title: Rejoining with pg_rewind
summary: How an old primary rejoins as a standby with pg_rewind instead of a full copy, what it needs, and when the keeper falls back to pg_basebackup
authors:
  - Sebas Mannem
date: 2026-11-24
---

# Rejoining with pg_rewind

After a failover, the old primary comes back as a standby of the new one: the sentinel gives its keeper a new standby
db (init mode `resync`), and the keeper resyncs it (see [Standby resync](resync.md)). By default the keeper does that
with `pg_basebackup`: it removes the data directory and copies the whole database from the primary. For a large
database that takes long, while the cluster has a standby less.

With `usePgrewind`, the keeper first tries
[pg_rewind](https://www.postgresql.org/docs/current/app-pgrewind.html): it only copies the blocks that changed since
the old primary and the new one diverged, and the WAL from that point. For an old primary that was only a little ahead
of the new one, that is much faster than a full copy.

## Enabling it

`usePgrewind` is a cluster spec setting, false by default:

```bash
orion-cli update --patch '{"usePgrewind": true}'
```

pg_rewind needs:

- **`wal_log_hints` or data checksums**: with `usePgrewind` on, the keepers set `wal_log_hints = on` themselves. It
  only takes effect after a restart of PostgreSQL: with `automaticPgRestart` (false by default) the keepers restart it,
  otherwise restart the keepers yourself. A cluster created with data checksums (`newConfig.dataChecksums`) has what
  pg_rewind needs already. Only changes made after the setting takes effect can be rewound: until then, a resync falls
  back to `pg_basebackup`.
- **A superuser**: pg_rewind connects to the primary as the superuser of the keepers (`--pg-su-username`), with its
  password (`--pg-su-password` or `--pg-su-passwordfile`, the same on all keepers) or with certificate authentication
  (`--pg-su-auth-method cert`). Without one of those, the keeper doesn't try pg_rewind.
- **Access to the primary as that superuser**: the primary accepts it from the other keepers with
  `defaultSUReplAccessMode` `all` (the default), and with `strict` (only from the addresses of the other keepers).

## What the keeper does

On a resync, the keeper tries pg_rewind only when all of these hold:

- `usePgrewind` is true, and the keeper has a superuser (see above);
- its data directory is initialized, with the same system identifier as the db it follows: it comes from the same
  database, as an old primary does (a new keeper, or one whose data directory was removed, gets a `pg_basebackup`);
- the cluster has a primary: pg_rewind always runs against the current primary, as it can't run against a standby.

Then the keeper:

1. stops PostgreSQL, and runs `pg_rewind --source-server` against the primary, with `synchronous_commit` off for its
   own connection, so it can't hang on synchronous replication without a standby to confirm;
2. sets up its data directory as a standby of the primary, and starts PostgreSQL;
3. waits until it accepts connections (`dbWaitReadyTimeout`, 60 seconds by default), and checks that it is on the
   timeline of the primary.

It falls back to a full resync with `pg_basebackup` (with `resyncFastCheckpoint` when set, see
[Standby resync](resync.md)) when:

- pg_rewind fails, e.g. because the WAL it needs is gone from the primary, `wal_log_hints` wasn't on yet, or the
  binary is missing;
- the standby doesn't accept connections within `dbWaitReadyTimeout`: it probably waits for WAL the primary doesn't
  have anymore;
- the standby ended up on another timeline branch than the primary: pg_rewind considers two databases on the same
  timeline to be in sync, even when they diverged on an earlier one.

So turning it on doesn't make a resync fail: at worst it takes the time of the failed pg_rewind longer. The keeper
logs `syncing using pg_rewind` when it tries, and `error syncing with pg_rewind` or `pg_rewinded standby is not
accepting connection` when it falls back.

## Tested

The end-to-end tests cover the old primary rejoining after a failover (`tests/system/crash_failover_test.go`,
`v1_fail_keeper_test.go`), with the default spec: that is with `pg_basebackup`. There is no end-to-end test with
`usePgrewind` on yet. The unit tests of the keeper (`internal/keeper/resync_test.go`) cover when it uses pg_rewind and
when `pg_basebackup` (e.g. without a superuser password); the fallbacks after pg_rewind (it fails, the standby doesn't
start, or it is on another timeline branch) are not covered by a test yet.
