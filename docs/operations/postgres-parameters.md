---
title: PostgreSQL parameters
summary: Set PostgreSQL parameters for all instances in the cluster spec, and what the keeper does with them
authors:
  - Sebas Mannem
date: 2026-11-24
---

# PostgreSQL parameters

You set the parameters of PostgreSQL in one place for the whole cluster: the `pgParameters` map of the cluster spec.
Every keeper writes them into the `postgresql.conf` of its instance, so all instances run with the same configuration.

```bash
orion-cli update --patch '{"pgParameters": {"work_mem": "16MB", "log_min_duration_statement": "1500"}}'
```

The values are strings, also for numbers. A patch merges into the map: it changes the parameters in it and keeps the
others. To remove a parameter, set it to `null`:

```bash
orion-cli update --patch '{"pgParameters": {"log_min_duration_statement": null}}'
```

`orion-cli spec` shows the current spec, with the parameters. You can also set them when you create the cluster
(`orion-cli init` with a spec, or the sentinel's `--initial-cluster-spec`).

## Reload and restart

The keeper checks the spec on every loop (`sleepInterval`). When the parameters changed, it writes `postgresql.conf`
and reloads PostgreSQL. That is enough for most parameters.

Some parameters only take effect after a restart, e.g. `max_connections` or `shared_buffers`. After the reload,
PostgreSQL marks them as `pending_restart` in `pg_settings`, and the keeper sets the metric
`orion_keeper_needs_restart` to 1. What happens then depends on `automaticPgRestart` in the cluster spec:

- **`false` (the default)**: the keeper doesn't restart PostgreSQL. The instance keeps the old value until you restart
  it, e.g. by restarting the keeper.
- **`true`**: the keeper restarts PostgreSQL itself, on every instance, primary included. Clients of the primary lose
  their connections during the restart.

```bash
orion-cli update --patch '{"automaticPgRestart": true}'
```

## Parameters that Orion sets

Orion needs some parameters to have certain values for replication and for the keeper to reach the instance. It sets
these itself, and ignores them in `pgParameters`:

| Parameter | Value |
|-----------|-------|
| `listen_addresses`, `port` | From the keeper's options. |
| `unix_socket_directories` | The socket directory of the keeper. |
| `hot_standby` | `on`. |
| `max_replication_slots` | `maxStandbys` of the spec. |
| `max_wal_senders` | 2 × `maxStandbys` + 2 + `additionalWalSenders` of the spec. |
| `wal_log_hints` | `on` with `usePgrewind`. |
| `synchronous_standby_names` | The synchronous standbys the sentinel picked, with `synchronousReplication` (see [sentinel decisions](sentinel-decisions.md)). |
| `primary_conninfo`, `primary_slot_name`, `restore_command`, `recovery_min_apply_delay`, `recovery_target*` | From the role of the instance, the [standby cluster](standby-cluster.md) settings or [point in time recovery](pitr.md). |

Two parameters you can set within limits:

- **`wal_level`**: Orion needs at least `replica`. It uses `logical` when you set that (e.g. for logical replication),
  and `replica` for any other value, e.g. `minimal`.
- **`wal_keep_size`**: Orion uses your value; without one it keeps 128MB (8 segments of 16MB).

To change the parameters with `max` in their name, change `maxStandbys` or `additionalWalSenders` in the spec instead.
They need a restart.

## ALTER SYSTEM

Parameters set with `ALTER SYSTEM` would override `postgresql.conf` on one instance only, and could override the ones
Orion sets. So the keeper makes `postgresql.auto.conf` a link to `/dev/null`: `ALTER SYSTEM` has no effect (depending
on the PostgreSQL version, it fails or it seems to work). Use `pgParameters` instead.

## A parameter PostgreSQL doesn't accept

When the spec has a parameter PostgreSQL doesn't know, or a value it doesn't accept, PostgreSQL refuses the whole
configuration file on the reload and logs why. It keeps running with the configuration it had: none of the other
changes of that update take effect either, and they aren't marked `pending_restart`, so the keeper doesn't restart it
into a configuration it can't start with. Once you fix or remove the parameter, the keeper applies the changes.

Orion doesn't check the parameters itself. Check the PostgreSQL log after a change, and before a restart: an instance
that can't start with its configuration is down, and when it is the primary, the sentinel fails over.

## The parameters of initdb

With `mergePgParameters` (the default), the parameters the new instance got from `initdb` (e.g. `max_connections`,
`shared_buffers`, the locale settings) end up in `pgParameters` of the spec once the cluster is initialized. From then
on, the spec has the full configuration, and every instance gets it. With `initMode: existing` or `pitr`, these are
the parameters of the existing instance or of the restored backup.

With `mergePgParameters: false`, the spec keeps only the parameters you set, and PostgreSQL uses its defaults for the
rest. Set it when you create the cluster.

## What the end-to-end tests check

`tests/system/pg_config_test.go` runs a cluster with one keeper and checks:

- the parameters of `initdb` (`max_connections`) end up in the spec with `mergePgParameters`, and stay out of it
  without it;
- a change of `max_connections` is `pending_restart` and not applied, and PostgreSQL doesn't restart, without
  `automaticPgRestart`; with it, the keeper restarts PostgreSQL and the new value is applied;
- `wal_level: minimal` keeps `replica`, and `wal_level: logical` is applied;
- with an unknown parameter in the spec, PostgreSQL keeps its old configuration (also for a `max_connections` change
  in the same update), and applies the change once the parameter is removed;
- `ALTER SYSTEM` doesn't change a setting.

`tests/system/v1_cluster_spec_test.go` checks that a patch adds and removes (`null`) a parameter in the spec.
