---
title: Failover and switchover
summary: When the sentinel fails over to a standby, what happens to the old primary, and how to switch the primary over yourself with orion-cli failkeeper
authors:
  - Sebas Mannem
date: 2026-11-24
---

# Failover and switchover

The sentinel fails over by itself when the primary fails: it elects a standby as the new primary, the proxies follow
it, and the old primary comes back as a standby. To move the primary yourself (a switchover, e.g. for maintenance),
fail its keeper with `orion-cli failkeeper`.

## Failover

The sentinel checks the keepers every `sleepInterval` (5s by default). A keeper that doesn't report for longer than
`failInterval` (20s by default) is unhealthy, and so is a database that fails for that long. When the keeper or the
database of the primary is unhealthy, the sentinel elects a new primary from the standbys that:

- run on a healthy keeper, with a healthy database that converged (it runs as the sentinel wants it);
- are on the timeline of the primary;
- without synchronous replication: are at most `maxStandbyLag` (1MiB by default) behind the last position the primary
  reported;
- with synchronous replication: are synchronous standbys, both in what the sentinel set and in what the primary last
  reported (see [Sentinel decisions](sentinel-decisions.md)); when there is none, the sentinel doesn't fail over;
- run on a keeper that can be master (a keeper started with `--can-be-master=false` is never elected).

Of those, it elects the one that is furthest ahead, so with asynchronous replication the least data is lost. The
proxies then send new connections to the new primary (see [Sentinel decisions](sentinel-decisions.md#proxies-after-a-new-primary)).

The end-to-end tests fail over in three ways, each on a cluster with a primary and one standby with the same data:

| What happens to the primary | Test | What the tests check |
|-----------------------------|------|----------------------|
| It crashes (its container is killed) | `tests/system/crash_failover_test.go` | The standby takes over with all the rows, the proxy follows it and it takes writes. |
| It freezes (its container is paused: no answers, but no refused connections either) | `tests/system/frozen_primary_test.go` | The same, and when the old primary wakes up it doesn't go on as a primary (no split brain): it steps down and follows the new one. |
| Its data directory disappears (a lost or wrongly mounted volume) | `tests/system/wiped_primary_test.go` | The keeper doesn't start a new, empty primary in its place; the standby takes over with the data. |

### The old primary

When the old primary comes back, it doesn't take over again: the sentinel gives its keeper a new standby that follows
the new primary. The tests check that it has the rows written after the failover, and is in recovery. Its keeper
resyncs it, with pg_rewind when `usePgrewind` is set, else with pg_basebackup: see [Standby resync](resync.md).

## Switchover

To move the primary to another keeper, fail the keeper of the primary:

```bash
orion-cli status                       # the keepers, and which one runs the primary
orion-cli failkeeper <keeper uid> --wait
```

or over the api: `POST /v1/cluster/keepers/{uid}/actions/fail` (`202 Accepted`).

A fail is a one-shot signal: the sentinel treats the keeper as failed in its next check only, and elects a new primary
as in a failover. After that check the keeper counts as healthy again; it gets a standby of the new primary, like the
old primary after a failover.

- **The standby must be able to take over in that one check**: healthy, with a converged database on the timeline of
  the primary. When none can, the primary just stays. The api refuses to fail the primary when it sees no keeper that
  could take over (`409 Conflict`, "no other keeper can take over now"), e.g. in a cluster with one keeper. It checks
  the basics only: a standby that is too far behind (`maxStandbyLag`) or can't be master still doesn't take over.
- **With `--wait`**, `orion-cli failkeeper` waits until another keeper runs the primary and every healthy keeper
  converged, and then prints "the cluster applied the change". `--wait-timeout` sets how long it waits (5 minutes by
  default). Without `--wait` it returns as soon as the api accepted the fail.
- **Failing a standby** doesn't move the primary: the standby is failed for one check, and comes back. The api tests
  (`tests/system/v1_fail_keeper_test.go`) check that the primary stays and keeps serving through the proxy, and that
  the standby converges again.
- An unknown keeper gives `404 Not Found`, and so does a cluster that doesn't exist yet. While the store doesn't
  answer, the api returns `503 Service Unavailable`.

### Without losing transactions

With asynchronous replication, a commit returns before a standby has it, so a switchover can lose the last
transactions: the standby takes over with what it had (within `maxStandbyLag`). To switch over without losing any:

- **With synchronous replication** on, just fail the primary: the sentinel only elects a synchronous standby, which has
  every committed transaction.
- **Without it**, turn it on for the switchover:

    ```bash
    orion-cli update --patch --wait '{"synchronousReplication": true}'
    ```

    Wait until the primary has a synchronous standby (`sync_state` is `sync` in `pg_stat_replication` on the primary),
    fail the primary with `orion-cli failkeeper <keeper uid> --wait`, and turn it off again:

    ```bash
    orion-cli update --patch --wait '{"synchronousReplication": false}'
    ```

    Meanwhile, a commit waits until a synchronous standby has it: when that standby stops answering, commits wait for
    it (`tests/system/sync_replication_test.go`).

The cli end-to-end test (`tests/cli/bootstrap_test.go`) waits until both keepers converged and stay so for a while,
runs `orion-cli failkeeper <primary> --wait --wait-timeout 3m`, and checks with `orion-cli status` that another keeper
runs the primary.

## Promote

`orion-cli promote` (`POST /v1/cluster/actions/promote`) is not a switchover: it turns a standby cluster into a primary
cluster of its own (see [Standby cluster](standby-cluster.md)). On a cluster that is primary already, it changes
nothing.
