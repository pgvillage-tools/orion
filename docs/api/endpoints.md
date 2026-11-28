---
title: REST api
summary: The endpoints of orion-api, what they return, their status codes, and the cli commands that use them
authors:
  - Sebas Mannem
date: 2026-11-28
---

# REST api

`orion-api` manages an Orion cluster through a REST api. `orion-cli` uses it for all its commands, but any HTTP client
with the client certificate can call it. This page describes every endpoint. For the options of `orion-api`, see the
[command reference](../reference/commands/orion-api.md).

## Ports

The api listens on two ports:

| Port | Default | Protocol | Endpoints |
|------|---------|----------|-----------|
| api port (`--port`) | 8443 | HTTPS (TLS 1.3); in normal mode a client certificate is required | `/v1/...` |
| probe port (`--probe-port`) | 8080 | Plain HTTP, no authentication | `/livez`, `/readyz`, `/status`, `/metrics` |

The api port has two modes (see [Ports and modes](bootstrap-design.md#ports-and-modes)):

- **Normal mode:** the api serves with its certificate, and only accepts clients with a client certificate signed by
  the api client intermediate (see [API certificates](certificates.md)). All endpoints below, apart from the
  bootstrap ones, are in normal mode.
- **Bootstrap mode:** the api has no certificates yet and waits for `orion-cli bootstrap`. Only the
  [bootstrap endpoints](#bootstrap) exist then. The probe port tells which mode the api is in (`GET /status`).

## Calling the api

With curl and the files of the cli (see [Using the cli](certificates.md#using-the-cli)):

```bash
curl --cacert ~/.orion/cli/ca.crt --cert ~/.orion/cli/tls.crt --key ~/.orion/cli/tls.key \
  https://orion-api.example.com:8443/v1/cluster/status
```

```json
{"proxies":[{"uid":"a1b2c3d4","generation":3}],"keepers":[{"uid":"keeper0","listenAddress":"10.0.0.11:5432","healthy":true,"pgHealthy":true,"pgWantedGeneration":2,"pgCurrentGeneration":2}],"sentinels":[{"uid":"e5f6a7b8","leader":true}],"status":{"available":true,"primaryDB":"9c8d7e6f","primaryKeeper":"keeper0","role":"master"}}
```

To change the spec, send JSON, e.g. a JSON merge patch:

```bash
curl --cacert ~/.orion/cli/ca.crt --cert ~/.orion/cli/tls.crt --key ~/.orion/cli/tls.key \
  -X PATCH -H 'Content-Type: application/merge-patch+json' -d '{"maxStandbys": 4}' \
  https://orion-api.example.com:8443/v1/cluster/spec
```

### Authentication

- **A client certificate is required.** Without one, or with one of another CA or one that expired, the TLS
  handshake fails, before any HTTP request.
- **The identity** of the client is the common name (CN) of its certificate, and its groups are the organizational
  units (OU). `GET /v1/whoami` returns it. See [Authentication](bootstrap-design.md#authentication).
- **Every client of the CA can call every endpoint**: there is no authorization by identity yet. The only exception
  is `POST /v1/bootstrap/confirm`, which only `orion-cli` may call (403 otherwise).

### Errors

Every error response is a problem details object ([RFC 9457](https://www.rfc-editor.org/rfc/rfc9457)), with
`Content-Type: application/problem+json`:

```json
{"type":"about:blank","title":"Not Found","status":404,"detail":"keeper keeper3 doesn't exist"}
```

`title` is the text of the status code, and `detail` explains this occurrence. The cli prints the detail.

| Code | When |
|------|------|
| 400 Bad Request | The body is not valid JSON, or the spec is invalid. |
| 401 Unauthorized | The request has no identity (it can't happen with a TLS connection with a client certificate). |
| 403 Forbidden | The identity may not call this endpoint (only `POST /v1/bootstrap/confirm`). |
| 404 Not Found | There is no cluster yet, or the keeper, proxy or sentinel doesn't exist. |
| 409 Conflict | The change isn't allowed now, e.g. creating a cluster that exists, or removing the keeper of the primary. Also when the cluster data changed while the api wrote it: try again. |
| 412 Precondition Failed | The `If-Match` header doesn't match the current ETag: read again and redo the change. |
| 415 Unsupported Media Type | A `PATCH` without `Content-Type: application/merge-patch+json` (the answer has an `Accept-Patch` header). |
| 500 Internal Server Error | Something else failed; the api logs the cause. |
| 503 Service Unavailable | The store didn't answer in time (`--store-timeout`). The answer has `Retry-After: 2`. |

The api logs the cause of a 500 or 503 instead of returning it, because store errors can name store internals.

A path that doesn't exist gets a plain text 404, and a method that the path doesn't have a plain text 405, with an
`Allow` header.

### ETags and If-Match

Some answers have an `ETag` header, and some changes take an `If-Match` header, so a change only applies to the
version you read (optimistic locking):

- **The cluster spec** (`/v1/cluster/spec`) has an ETag that only changes when the spec changes. Use it with `PATCH`
  and `PUT` on the spec. See [Changing the spec safely](../cluster/spec.md#changing-the-spec-safely).
- **The cluster data** (`/v1/cluster`) has an ETag of its revision in the store. The sentinel writes the cluster data
  all the time, so this ETag changes often.
- `If-Match` is optional: without it, the change applies to whatever is there. `If-Match: *` matches anything that
  exists. A weak ETag (`W/"..."`) is compared like a strong one.

### Asynchronous changes

Some changes are done by the sentinel after the request: the api answers **202 Accepted**, without a body, and with a
`Location` header that points to `/v1/cluster/status`. The status shows when the change is done; `orion-cli --wait`
polls it.

## The cluster

| Method and path | What it does | Answer | cli |
|-----------------|--------------|--------|-----|
| `GET /v1/cluster/status` | The keepers, proxies and sentinels, and the status of the cluster | 200 | `status`, `--wait` |
| `GET /v1/cluster` | All cluster data (the spec, the status, the keepers and their databases, as the sentinel writes them) | 200 with `ETag`; 404 without a cluster | `status`, `init`, `clusterdata read` |
| `PUT /v1/cluster` | Replaces all cluster data | 200 (with the new `ETag` when `If-Match` was sent); 400, 412 | `clusterdata write` |
| `POST /v1/cluster` | Creates the cluster with the spec in the body; an empty body creates one with the default spec (init mode `new`) | 201 with the cluster data and `Location: /v1/cluster`; 409 when there is a cluster | `init` |
| `POST /v1/cluster/actions/promote` | Promotes a standby cluster to a primary cluster | 202; 200 when it is a primary cluster already; 404 | `promote` |

`GET /v1/cluster/status` answers 200 also without a cluster: then the lists are empty, and `available` is false. The
`status` object has:

| Field | Meaning |
|-------|---------|
| `available` | True when the cluster has a primary database whose keeper is in the cluster data. In a [standby cluster](../operations/standby-cluster.md), that is the database that replicates from the external primary. |
| `primaryDB`, `primaryKeeper` | The uid of that database and its keeper (only when available). |
| `role` | The role of the cluster: `master` or `standby` (only when there is a cluster). |

`PUT /v1/cluster` replaces everything the sentinel and the keepers rely on, like `stolonctl clusterdata write`. It is
meant for repairs: use the spec endpoints for normal changes. The cluster data in the body needs a cluster with a
valid spec.

See [Initialization](../cluster/initialization.md) for creating a cluster, and
[Standby cluster](../operations/standby-cluster.md) for promoting one.

## The cluster spec

| Method and path | What it does | Answer | cli |
|-----------------|--------------|--------|-----|
| `GET /v1/cluster/spec` | The spec | 200 with `ETag`; 404 without a cluster | `spec` |
| `PATCH /v1/cluster/spec` | Applies a JSON merge patch ([RFC 7386](https://www.rfc-editor.org/rfc/rfc7386)) with `Content-Type: application/merge-patch+json` | 200 with the new `ETag`; 400, 404, 409, 412, 415 | `update --patch` |
| `PUT /v1/cluster/spec` | Replaces the spec | 200 with the new `ETag`; 400, 404, 409, 412 | `update` |

The api checks the new spec before it stores it: an invalid spec gets 400, and a change that isn't allowed (e.g.
another init mode) gets 409. See [Cluster spec](../cluster/spec.md) for the fields, what the api checks, and what a
change does.

## Keepers, proxies and sentinels

| Method and path | What it does | Answer | cli |
|-----------------|--------------|--------|-----|
| `GET /v1/cluster/keepers` | The keepers, sorted on uid | 200 (an empty list without a cluster) | |
| `GET /v1/cluster/keepers/{uid}` | One keeper | 200; 404 | |
| `DELETE /v1/cluster/keepers/{uid}` | Removes a keeper, and its database, from the cluster data | 202; 404; 409 for the keeper of the primary | `removekeeper` |
| `POST /v1/cluster/keepers/{uid}/actions/fail` | Marks the keeper as failed, so the sentinel fails over when it runs the primary | 202; 404; 409 when no other keeper can take over | `failkeeper` |
| `GET /v1/cluster/proxies` | The proxies, sorted on uid | 200 | |
| `GET /v1/cluster/proxies/{uid}` | One proxy | 200; 404 | |
| `GET /v1/cluster/sentinels` | The sentinels, sorted on uid, with the leader | 200 | |
| `GET /v1/cluster/sentinels/{uid}` | One sentinel | 200; 404 | |

The same lists are in `GET /v1/cluster/status`. Their fields:

| Member | Fields |
|--------|--------|
| keeper | `uid`, `listenAddress` (host:port of its PostgreSQL; `(no db assigned)` or `(unknown)` otherwise), `healthy` (the keeper), `pgHealthy` (its PostgreSQL), `pgWantedGeneration` and `pgCurrentGeneration` (equal when the keeper converged to what the sentinel wants) |
| proxy | `uid`, `generation` (the generation of the proxy configuration it runs) |
| sentinel | `uid`, `leader` (true for the sentinel that makes the decisions) |

- **Removing a keeper** is for a keeper that is gone for good: a keeper that still runs registers itself again, and
  the sentinel adds it back. The keeper of the primary can't be removed: fail it first, and remove it once another
  keeper runs the primary.
- **Failing a keeper** that runs the primary needs another keeper that can take over: healthy, with a healthy
  database that converged and is on the timeline of the primary. Otherwise the api answers 409, because the sentinel
  would keep the primary. See [Failover and switchover](../operations/failover-switchover.md).

## Identity

| Method and path | What it does | Answer | cli |
|-----------------|--------------|--------|-----|
| `GET /v1/whoami` | The identity the api sees: `name` (the CN of the client certificate), `groups` (its OUs) and `method` (`client-certificate`) | 200 | `connect` |

`whoami` doesn't use the store, so it also answers while the store is down. `orion-cli connect` uses it to check the
connection.

## Bootstrap

In bootstrap mode, the api port has a self-signed certificate, doesn't ask for a client certificate, and only has
these endpoints. They must be called in this order, on one TLS connection: `orion-cli bootstrap` does that. See
[The bootstrap protocol](bootstrap-design.md#the-bootstrap-protocol) for the bodies and the checks.

| Method and path | What it does |
|-----------------|--------------|
| `POST /v1/bootstrap/challenge` | Starts the bootstrap: returns the nonce of the api. |
| `POST /v1/bootstrap/proof` | Takes the nonce and the proof of the cli, and returns the proof of the api. |
| `POST /v1/bootstrap/archive` | Takes the public key of the cli, extra names for the api certificate and the subject, and returns the certificates with the private keys encrypted to the cli. |
| `POST /v1/bootstrap/stored` | The cli stored the archive: the api stores its certificate and switches to normal mode. |

In normal mode:

| Method and path | What it does | Answer | cli |
|-----------------|--------------|--------|-----|
| `POST /v1/bootstrap/confirm` | The cli confirms that its new client certificate works: the api deletes the archive. It can be repeated. | 200 with `{"status":"confirmed"}`; 403 for another client than `orion-cli` | `bootstrap` |

## Probe port

| Method and path | What it does |
|-----------------|--------------|
| `GET /livez` | 200 as long as the process serves requests. It doesn't check the store. |
| `GET /readyz` | 200 when the api serves, isn't shutting down and its store answers within a second; 503 otherwise. |
| `GET /status` | The mode of the api port: `{"mode":"normal"}` or `{"mode":"bootstrap"}`. |
| `GET /metrics` | The Prometheus metrics of the api. |

See [Probe port](certificates.md#probe-port) and [Metrics](certificates.md#metrics) for the probes in Kubernetes and the
metrics.
