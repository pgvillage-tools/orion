---
title: Getting started
summary: Run a local Orion cluster in containers, with etcd, the api, a sentinel, two keepers and a proxy, and fail it over with orion-cli
authors:
  - Sebas Mannem
date: 2026-11-28
---

# Getting started

This page runs a complete Orion cluster on one machine, in containers: etcd as the store, the api, a sentinel, two
keepers (a primary and a standby) and a proxy. You create the certificates and the cluster with `orion-cli`, connect
to PostgreSQL through the proxy, and fail over. It takes the same steps as the end-to-end test of the cli
(`tests/cli/bootstrap_test.go`).

It is a test setup: the data lives in the containers, and the passwords are on the command line. For a real
deployment, see [Installation](install.md) and [Orion on Kubernetes](k8s.md).

## What you need

- **docker** (or podman: replace `docker` with `podman` in the commands).
- **`orion-cli`** on your machine, from a release or built from source (see [Installation](install.md)).
- **`psql`**, to connect to PostgreSQL.

Use the container images and the `orion-cli` of the same release:

```bash
tag=latest         # or a release, e.g. v0.2.1
keeper_tag=18      # the PostgreSQL major version, or e.g. 18-v0.2.1
images=ghcr.io/pgvillage-tools/orion
```

## The network and etcd

All containers share a network, so they find each other by name. etcd stores the state of the cluster:

```bash
docker network create orion
docker run -d --name etcd --network orion quay.io/coreos/etcd:v3.6.7 \
  etcd --listen-client-urls http://0.0.0.0:2379 --advertise-client-urls http://etcd:2379
```

The images of Orion use etcd by default (`--store-backend etcdv3`), for the cluster `orion` (`--cluster-name`). Each
component only needs to know where etcd is: `--store-endpoints`, here as an environment variable (see
[Environment variables](../reference/commands/index.md#environment-variables)).

## The certificates

The api only accepts clients with a client certificate. Create the CA, the certificate of the api and the client
certificate of the cli with `orion-cli pki init`:

```bash
mkdir orion-getting-started && cd orion-getting-started
orion-cli pki init --api-dir ./api
# Passphrase for the CA keys:
# Repeat the passphrase:
# Created the CA and the cli client certificate in /home/me/.orion/cli
# Created the api certificates in ./api: copy them to the --pki-dir of the api
# The private keys of the CA are in /home/me/.orion/cli/ca, encrypted with the passphrase: keep both safe, and away from the api
```

- The cli keeps its files in `~/.orion/cli` (`--pki-dir`), with the keys of the CA, encrypted with the passphrase.
- `./api` holds the files of the api: `server.crt`, `server.key` and `client-ca.crt`.
- The certificate of the api is valid for `localhost` and `127.0.0.1`, which is where the cli connects to below.

See [API certificates](../api/certificates.md) for the options, and how to renew the certificates.

## The api

The api image runs as uid 1000, and reads its files from `/etc/orion/api`. On Linux, give the directory to that user
first (see [Running the api in a container](../api/certificates.md#running-the-api-in-a-container), also for
SELinux):

```bash
sudo chown -R 1000 ./api
docker run -d --name api --network orion -p 8443:8443 -p 8080:8080 \
  -v "$PWD/api:/etc/orion/api:ro" \
  -e ORIONAPI_STORE_ENDPOINTS=http://etcd:2379 \
  $images/api:$tag
```

Port 8443 is the api (TLS, with a client certificate), port 8080 has the probes (`/livez`, `/readyz`, `/status`,
`/metrics`) without authentication.

Check that the cli can reach the api with its certificate:

```bash
orion-cli connect
# Connected to the api as orion-cli (client-certificate)
```

The cli connects to `127.0.0.1:8443` by default (`--host` and `--port`). `orion-cli connect` saves them in
`~/.orion/config.yml`, so the next commands use them too.

## The sentinel, the keepers and the proxy

The sentinel decides which keeper runs the primary. Every keeper runs one PostgreSQL instance. The proxy forwards
clients to the primary:

```bash
docker run -d --name sentinel --network orion \
  -e ORIONSENTINEL_STORE_ENDPOINTS=http://etcd:2379 \
  $images/sentinel:$tag

for keeper in keeper0 keeper1; do
  docker run -d --name $keeper --network orion \
    -e ORIONKEEPER_STORE_ENDPOINTS=http://etcd:2379 \
    -e ORIONKEEPER_UID=$keeper \
    -e ORIONKEEPER_PG_LISTEN_ADDRESS=$keeper \
    -e ORIONKEEPER_PG_SU_PASSWORD=supassword \
    -e ORIONKEEPER_PG_REPL_PASSWORD=replpassword \
    $images/keeper:$keeper_tag
done

docker run -d --name proxy --network orion -p 25432:25432 \
  -e ORIONPROXY_STORE_ENDPOINTS=http://etcd:2379 \
  $images/proxy:$tag
```

- **`ORIONKEEPER_UID`** names the keeper in the cluster (lower-case letters, numbers and `_`); without it, the keeper
  makes up a random one.
- **`ORIONKEEPER_PG_LISTEN_ADDRESS`** is the address of PostgreSQL that the other keepers and the proxy connect to:
  here the name of the container.
- **The passwords** of the superuser (`postgres`, the user the keeper runs as) and of the replication user
  (`repluser`) must be the same on all keepers. For a real deployment, use `--pg-su-passwordfile` and
  `--pg-repl-passwordfile` instead.
- The keepers wait until there is a cluster: there is none yet.

See the [command reference](../reference/commands/index.md) for all options.

## The cluster

Create the cluster with the default settings, and wait until it runs:

```bash
orion-cli init --wait
```

The sentinel picks one keeper to initialize a new database, and makes the other keeper a standby of it. With
`--wait`, the cli waits until the cluster has a primary and every healthy keeper has converged, and prints "the
cluster applied the change". See [Initialization](../cluster/initialization.md) for a cluster with your own settings,
or from an existing database or a backup.

```bash
orion-cli status
```

`orion-cli status` lists the sentinel, the proxy and the keepers, with `Master Keeper: <uid>` for the keeper that runs
the primary, and the keepers as a tree, with `(master)` behind the primary. `orion-cli status -f json` gives the same
as JSON.

## Connect to PostgreSQL

Clients connect to the proxy, never to a keeper. The proxy only forwards to the current primary:

```bash
psql "host=127.0.0.1 port=25432 user=postgres dbname=postgres"   # password: supassword
```

## Fail over

Move the primary to the other keeper, with the uid that `orion-cli status` shows as `Master Keeper`:

```bash
orion-cli failkeeper keeper0 --wait
# the cluster applied the change
orion-cli status
```

The other keeper now runs the primary, and the old primary becomes its standby. The proxy closed the connections to
the old primary: connect again with `psql`, and you are on the new one.

To see an automatic failover, stop the container of the primary (`docker stop keeper1`). The sentinel elects the
standby once the primary has been unreachable for `failInterval` (20 seconds by default). See
[Failover and switchover](../operations/failover-switchover.md).

## Change the cluster

The cluster spec holds the settings of the cluster, e.g. the PostgreSQL parameters:

```bash
orion-cli spec
orion-cli update --patch --wait '{"pgParameters": {"work_mem": "8MB"}}'
```

See [Cluster spec](../cluster/spec.md) and [PostgreSQL parameters](../operations/postgres-parameters.md).

## Clean up

```bash
docker rm -f proxy keeper0 keeper1 sentinel api etcd
docker network rm orion
```

The cli files stay in `~/.orion` (the certificates in `~/.orion/cli`, the host and port in `~/.orion/config.yml`).
Remove them too when you are done, or keep them for the next cluster of this CA: `orion-cli pki init` refuses to
overwrite them without `--force`.
