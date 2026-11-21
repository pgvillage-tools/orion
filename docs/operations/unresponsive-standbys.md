---
title: Unresponsive standbys
summary: Set tcp_user_timeout, so a standby that stops reading can't block the primary's walsender, checkpoints and slot invalidation
authors:
  - Sebas Mannem
date: 2026-11-21
---

# Unresponsive standbys

A standby can stop reading its replication connection while its host still answers on the network: a frozen
container or VM, a process that is stopped (`SIGSTOP`), a host that hangs. The primary then can't send it any more
WAL. Set `tcp_user_timeout` on the cluster, so the primary closes such a connection instead of waiting for it:

```bash
orion-cli update --patch '{"pgParameters": {"tcp_user_timeout": "10000"}}'
```

Orion doesn't set it by default.

## What happens without it

The end-to-end test of a standby that loses its replication slot (`tests/system/wal_recycled_test.go`) found this.
It pauses the standby's container (`podman pause`) and writes more WAL on the primary than `max_slot_wal_keep_size`
allows, so a checkpoint must invalidate the standby's slot. In a run without `tcp_user_timeout` (PostgreSQL 18.7,
Linux 7.1), on the primary:

- the walsender of the paused standby waited in `ClientWrite`: sending WAL to the standby, which didn't read it;
- the checkpointer waited in `ReplicationSlotDrop`: for the walsender to release the slot, so it could invalidate it;
- the `CHECKPOINT` command waited in `CheckpointDone`: for the checkpointer.

This lasted 13 minutes, until the standby was unpaused by hand; it may never end on its own. `wal_sender_timeout`
(60s by default) didn't end it. PostgreSQL removes old WAL files at the end of a checkpoint, so while it waits, the
WAL on the primary keeps growing.

With `tcp_user_timeout` set to 10 seconds, the same test passes: the slot is invalidated within the minute the test
waits for it, and the standby gets a new copy of the primary (a resync) once it is back.

## How it works

`tcp_user_timeout` is the time, in milliseconds, that data the server sent may stay unacknowledged before the kernel
closes the connection (`TCP_USER_TIMEOUT`). A standby that doesn't read fills its receive buffer, and then the primary
can't get its WAL across anymore; in the test, the kernel closed that connection after the timeout. The walsender then
ends, and the checkpoint can continue. The keepalive settings (`tcp_keepalives_*`) are no replacement: TCP only
sends keepalives on an idle connection, and this one has data waiting.

The setting applies to every TCP connection of the server, also those of clients. Choose a value that is longer than
the network interruptions you want to survive: a connection whose data stays unacknowledged for that long is closed.
It works on systems with `TCP_USER_TIMEOUT` (Linux); see
[tcp_user_timeout](https://www.postgresql.org/docs/current/runtime-config-connection.html#GUC-TCP-USER-TIMEOUT) in
the PostgreSQL documentation.
