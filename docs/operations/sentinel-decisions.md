---
title: Sentinel decisions
summary: How the sentinel picks the synchronous standbys, when it points the proxies to a new primary, and when it replaces a standby that can't catch up
authors:
  - Sebas Mannem
date: 2026-11-21
---

# Sentinel decisions

The leader sentinel decides, on every check, what each keeper and proxy should do. This page describes a few of its
decisions that you can see from the outside, and that are easy to get wrong. It is a start: these are the points the
unit tests of the sentinel (`internal/sentinel/update_cluster_units_test.go`) pin down.

## Synchronous standbys

With `synchronousReplication`, the sentinel keeps a set of synchronous standbys for the primary (PostgreSQL's
`synchronous_standby_names`), between `minSynchronousStandbys` and `maxSynchronousStandbys`. On every check it:

1. removes the standbys that no longer exist, and the ones that aren't healthy and converged;
2. removes standbys over `maxSynchronousStandbys`, for example after you lowered it;
3. adds the best standbys until there are `maxSynchronousStandbys`: healthy, converged, on the timeline of the
   primary, the ones furthest along in the WAL first;
4. adds a placeholder (`orionfakestandby`) when there are fewer real ones than `minSynchronousStandbys`, so commits
   wait instead of being acknowledged by fewer standbys than you asked for.

In step 2, the sentinel keeps the best ones: first the standbys the primary reports in sync, then the ones furthest
along in the WAL; when those are equal, the one with the lowest db uid. So after you lower `maxSynchronousStandbys`,
the standbys that are already acknowledging commits stay, and a standby that was just added (and isn't in sync yet)
is the first to go.

The sentinel changes the set in steps: after it changed it, it waits until the primary reports the new synchronous
standbys in sync before it changes the set again. When one of the new ones fails or disappears in the meantime, it
falls back to the ones that are already in sync.

## Proxies after a new primary

When the sentinel elects a new primary, it first tells the proxies to stop proxying (no primary in the proxy spec),
and waits until every proxy that reports in has done so: only then can no proxy still send clients to the old
primary. A proxy that stopped reporting is assumed to have closed its connections. Then it points the proxies to the
new primary.

Once the proxies point to a primary, a proxy that reports in is enabled right away, also before it has caught up with
the current proxy spec; a proxy that stops reporting is disabled.

## Standbys that can't catch up

A standby whose WAL position stops increasing while it is behind the primary may need WAL that the primary already
removed. The sentinel removes its db (so its keeper gets a new one, with a full copy of the primary: a resync) when
all of these hold:

- the primary reports the oldest WAL file it still has;
- the standby is healthy, past its initialization, and behind the primary;
- its WAL position didn't increase for more checks than `DefaultDBNotIncreasingXLogPosTimes` (10);
- the WAL file it needs is older than the oldest one the primary has.

A standby that is still catching up, that is unhealthy, or that is still initializing is left alone.
