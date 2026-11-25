---
title: Custom pg_hba entries
summary: The pg_hba.conf that the keeper writes, the entries it adds itself, and your own entries in pgHBA
authors:
  - Sebas Mannem
date: 2026-11-24
---

# Custom pg_hba entries

The keeper writes `pg_hba.conf` itself, on every instance, and reloads PostgreSQL when it changes. Don't edit the file
on the host: the keeper overwrites it. Instead, put your own entries in the `pgHBA` field of the cluster spec:

```bash
orion-cli update --patch '{"pgHBA": ["host all all 0.0.0.0/0 scram-sha-256", "host all all ::0/0 scram-sha-256"]}'
```

`pg_hba.conf` then has, in this order:

1. the entries the keeper needs itself (see [The entries of the keeper](#the-entries-of-the-keeper));
2. your entries from `pgHBA`, or the default entries when `pgHBA` isn't set (see [pgHBA](#pghba)).

PostgreSQL uses the first entry that matches a connection, so your entries can't take away the access of the keeper.

## pgHBA

| `pgHBA` in the spec | Entries after the ones of the keeper |
|---------------------|--------------------------------------|
| Not set (`null`) | `host all all 0.0.0.0/0 scram-sha-256` and `host all all ::0/0 scram-sha-256`: every user, from every host, with a password |
| `[]` | None: only the superuser and the replication user can connect |
| A list of entries | Those entries, in that order |

The default entries take the connection type and the authentication method of the superuser
(`--pg-su-connection-type` and `--pg-su-auth-method` of the keeper, see [The entries of the keeper](#the-entries-of-the-keeper)).

Use `scram-sha-256` in your own entries rather than `md5`: PostgreSQL also accepts `md5` for a password stored as
SCRAM, but `scram-sha-256` never sends or accepts an MD5 hash.

`orion-cli init` without a spec leaves `pgHBA` unset, so a new cluster accepts every user with a password.

- **Set or replace your entries**: `orion-cli update --patch '{"pgHBA": [...]}'`. A JSON merge patch replaces the whole
  list, so give all entries.
- **Back to the default entries**: `orion-cli update --patch '{"pgHBA": null}'`.
- **Show them**: `orion-cli spec`.

The api only checks that an entry has no newline. PostgreSQL checks the rest: with an entry it doesn't understand, it
keeps using the `pg_hba.conf` it had at a reload (and logs the error), and it doesn't start at all after a restart.
Check the PostgreSQL log after a change.

The entries are in the [`pg_hba.conf` format](https://www.postgresql.org/docs/current/auth-pg-hba-conf.html) of
PostgreSQL, e.g. `hostssl app app_user 10.0.0.0/8 scram-sha-256`.

### Clients connect through the proxy

Clients connect through `orion-proxy`, which forwards the connection to the primary. For PostgreSQL, the connection
comes from the proxy, so the address in an entry must match the addresses of the proxies, not of the clients. You
can't tell clients apart by address in `pgHBA`: to give groups of clients different access, give each group its own
proxies, and allow the addresses of those proxies.

## The entries of the keeper

The keeper always adds entries for its own local connections, as the superuser and as the replication user:

```text
local postgres <superuser> <local auth method of the superuser>
local replication <replication user> <local auth method of the replication user>
```

For the connections between the instances (replication, and `pg_rewind` as the superuser), it adds entries that
depend on `defaultSUReplAccessMode` in the spec:

| `defaultSUReplAccessMode` | Entries |
|---------------------------|---------|
| `all` (default) | On every instance: the superuser (all databases) and the replication user (replication), from every IPv4 and IPv6 address |
| `strict` | Only on the primary (or, in a [standby cluster](standby-cluster.md), on the standby that follows the other cluster): the superuser and the replication user, from the address of each other keeper of the cluster (`/32`) |

```bash
orion-cli update --patch '{"defaultSUReplAccessMode": "strict"}'
```

With `strict`, the keeper updates the entries when a keeper joins or leaves, or another instance becomes the primary.

The connection type and the authentication method of these entries come from the flags of the keeper (or their
`ORIONKEEPER_` environment variables), not from the spec:

| Flag | Default | |
|------|---------|---|
| `--pg-su-username` | The user that runs the keeper | The superuser |
| `--pg-su-connection-type` | `host` | `host`, `hostssl`, `hostnossl`, `hostgssenc` or `hostnogssenc` |
| `--pg-su-auth-method` | `scram-sha-256` | `scram-sha-256`, `md5`, `trust`, `cert`, `ident` or `peer` |
| `--pg-su-local-auth-method` | `--pg-su-auth-method` | For the `local` entry; `cert` becomes `peer` there |
| `--pg-repl-username` | (required) | The replication user |
| `--pg-repl-connection-type` | `host` | As for the superuser |
| `--pg-repl-auth-method` | `scram-sha-256` | As for the superuser |
| `--pg-repl-local-auth-method` | `--pg-repl-auth-method` | As for the superuser |

## With synchronous replication

When a primary with `synchronousReplication` restarts, its keeper first writes only its own entries: not yours, and
not the default entries. Only the superuser and the replication user can connect, until the synchronous standbys are in
sync again. Then the keeper adds your entries and reloads. Otherwise a client could write data that is lost when the
primary fails before a standby has it.

The end-to-end test `tests/system/sync_lockout_test.go` runs this: with `pgHBA` set to `host all all 0.0.0.0/0 scram-sha-256`
and the same for `::0/0`, it kills PostgreSQL on the primary while its synchronous standby is paused. The superuser
connects again as soon as PostgreSQL runs; a normal user gets a `pg_hba.conf` error until the standby is back and in
sync, and then reads the data.

## Tests

- `tests/system/sync_lockout_test.go` sets `pgHBA`, and checks that a normal user can connect with it, and that the
  keeper leaves it out after a restart with synchronous replication.
- The other end-to-end tests create their clusters with `pgHBA: []` and connect as the superuser, through the entries
  of the keeper.
- The default entries (`pgHBA` not set), `strict`, and a change of `pgHBA` on a running cluster have no end-to-end test
  yet.
