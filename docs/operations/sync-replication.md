---
title: Synchronous replication
summary: Commits that wait for a synchronous standby, failover to a synchronous standby only, and a restarted primary that keeps normal users out until its standbys are in sync
authors:
  - Sebas Mannem
date: 2026-11-24
---

# Synchronous replication

By default, a standby replicates asynchronously: a commit on the primary returns before any standby has it. When the
primary fails, the standby that takes over can miss the last transactions. With synchronous replication, a commit
returns only once a synchronous standby has it, and on a failover the sentinel only elects a synchronous standby. So
a commit that returned is not lost on a failover.

Turn it on in the cluster spec:

```bash
orion-cli update --patch '{"synchronousReplication": true}'
```

| Field | Default | What it does |
|-------|---------|--------------|
| `synchronousReplication` | `false` | Use synchronous replication between the primary and its standbys. |
| `minSynchronousStandbys` | `1` | The fewest synchronous standbys a commit waits for. |
| `maxSynchronousStandbys` | `1` | The most standbys the sentinel makes synchronous. At least 1, and at least `minSynchronousStandbys`. |

You can turn it on and off at any time. For example, two to three synchronous standbys:

```bash
orion-cli update --patch '{"synchronousReplication": true, "minSynchronousStandbys": 2, "maxSynchronousStandbys": 3}'
```

The keepers apply a change of the spec without a restart of PostgreSQL: the sentinel sets the synchronous standbys of
the primary, and its keeper writes them to `synchronous_standby_names`.

## A commit waits for its synchronous standby

The end-to-end test (`tests/system/sync_replication_test.go`) runs a cluster with a primary and one standby, with
`synchronousReplication` on:

- the standby becomes the synchronous standby of the primary: `pg_stat_replication` on the primary shows it with
  `sync_state = 'sync'`;
- when the standby freezes (its container is paused), an `insert` through the proxy doesn't return: it waits;
- once the standby is back, it has the row, and commits return again.

The transaction that waited is committed on the primary already; it only waits for the acknowledgement of the
standby. So it is not undone when the client gives up waiting: the client just doesn't know that it was committed.

Which standbys the sentinel makes synchronous, and how it changes them, is in
[Sentinel decisions](sentinel-decisions.md#synchronous-standbys). When there are fewer healthy standbys than
`minSynchronousStandbys`, it adds a placeholder (`orionfakestandby`), so commits wait instead of returning with fewer
synchronous standbys than you asked for.

A keeper started with `--can-be-synchronous-replica=false` is never made synchronous; the sentinel logs
`fewer synchronous standbys than maxSynchronousStandbys` when that leaves it short.

### Availability and minSynchronousStandbys

With `minSynchronousStandbys` at 1 or more, commits wait when there are fewer healthy standbys than that: the cluster
chooses consistency over availability. To keep accepting commits when a standby fails, have at least
`minSynchronousStandbys` + 1 standbys.

`minSynchronousStandbys` can also be 0. The sentinel then makes healthy standbys synchronous (up to
`maxSynchronousStandbys`), but adds no placeholder: when there is no healthy standby, commits return without one.
That can lose committed transactions, for example when the primary restarts while it has no synchronous standby and
fails after that. Only use it with one primary and one standby, when you value availability over consistency.

## Failover

When the primary fails, the sentinel only elects a standby that is synchronous both in the spec of the primary and in
what the primary last reported (`synchronous_standby_names`). When there is no such standby, it doesn't elect a new
primary, and logs `no eligible masters`: electing an asynchronous standby could lose transactions that were
acknowledged. The new primary gets the synchronous standbys of the old one, with the old primary in its own place.

## A primary that restarts

PostgreSQL has a gap here ([its docs](https://www.postgresql.org/docs/current/warm-standby.html#SYNCHRONOUS-REPLICATION)):
when the primary restarts while commits wait for a synchronous standby, the waiting transactions are committed once
it is back, also when no standby has them. For example:

1. the synchronous standby goes down;
2. a client commits, and waits for the standby;
3. the primary restarts: the transaction is committed, and other clients see it;
4. the primary fails, and the standby comes back;
5. the sentinel elects the standby (it is a synchronous standby), which doesn't have the transaction: it is lost.

The sentinel can't tell whether a synchronous standby is really in sync while the primary is down, but the keeper
knows when the primary restarts. When the keeper finds PostgreSQL stopped and starts it again with synchronous replication on, it only lets in the
superuser (for the keeper itself) and the replication user, until all synchronous standbys in the spec are in sync
again. The `pgHBA` entries of the spec, for the normal users, are only added after that.

The end-to-end test (`tests/system/sync_lockout_test.go`) checks this with a normal user that connects with a
password, through a `pgHBA` entry:

- it freezes the synchronous standby, and kills PostgreSQL on the primary;
- the keeper starts PostgreSQL again: the superuser can connect, the normal user can't (`no pg_hba.conf entry`);
- once the standby is back in sync, the normal user can connect again, and has all the data.

The keeper logs `not allowing connection as normal users since synchronous replication is enabled and instance was
down` while it waits. Don't let applications connect as the superuser or the replication user: they would get in
during this time, and they use up the connections the keeper needs.

## Not in a standby cluster

A [standby cluster](standby-cluster.md) doesn't use synchronous replication: its primary is a standby of another
cluster itself, so the sentinel ignores `synchronousReplication` there.
