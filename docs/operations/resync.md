---
title: Standby resync
summary: How a keeper gets a new standby from the primary (pg_rewind or pg_basebackup), and the spec options that make it faster
authors:
  - Sebas Mannem
date: 2026-11-21
---

# Standby resync

When the sentinel gives a keeper a new standby db (with init mode `resync`), the keeper resyncs it: it brings its
data directory in line with the db it follows (the primary), and starts PostgreSQL as a standby of it. The sentinel
does that for a free keeper (one without a db) while the cluster has fewer standbys than `maxStandbysPerSender`: a new
keeper, a keeper whose standby was removed because it failed or couldn't catch up, or the old primary after a
failover.

## pg_rewind or pg_basebackup

The keeper resyncs in one of two ways:

- **pg_rewind**, when `usePgrewind` is true in the cluster spec, the keeper has a superuser (`--pg-su-username`, with a
  password or certificate authentication), its data directory is initialized and comes from the same database (the
  same system identifier as the primary). pg_rewind only copies what differs from the primary, which is fast for an
  old primary after a failover. If it fails, or the standby doesn't start streaming afterwards, the keeper falls back
  to pg_basebackup.
- **pg_basebackup** otherwise: the keeper removes its data directory and copies the whole database from the primary
  (`pg_basebackup -R -Xs`), with a replication slot for the new standby.

## A fast checkpoint for pg_basebackup

`pg_basebackup` starts with a checkpoint on the primary. By default that is a spread checkpoint: PostgreSQL spreads
its writes over `checkpoint_completion_target` × `checkpoint_timeout`, so with the default settings the backup can
wait up to 4.5 minutes before it copies anything. Until it is done, the cluster has a standby less.

Set `resyncFastCheckpoint` to make the keeper ask for an immediate checkpoint (`pg_basebackup --checkpoint=fast`):

```bash
orion-cli update --patch '{"resyncFastCheckpoint": true}'
```

The standby is back sooner, at the cost of an IO spike on the primary: it writes all its dirty buffers at once. It is
false by default, as in PostgreSQL's `pg_basebackup` itself. Leave it off when the primary's IO is the bottleneck
(large shared buffers, slow disks, busy hours); turn it on when getting the standby back quickly matters more.

The end-to-end test of a standby that lost the WAL it needs (`tests/system/wal_recycled_test.go`) sets it: in a run
on a laptop, the whole step, from the standby losing its slot to streaming again after the resync, took 107 seconds,
where the spread checkpoint alone would have taken 4.5 minutes.

The setting only affects the `pg_basebackup` of a resync. A `dataRestoreCommand` (init mode `pitr`) runs your own
command, so there you choose the options yourself.
