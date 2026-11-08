---
title: Orion
summary: Orion is a cloud native cluster manager for PostgreSQL, a fork of stolon, with a REST api and a cli
authors:
  - Sebas Mannem
date: 2026-11-08
---

# Orion

Orion is a cluster manager for PostgreSQL. It runs PostgreSQL clusters with a primary and one or more standbys, fails
over automatically when the primary fails, and makes sure clients only connect to the current primary. You can run
Orion on virtual machines and on Kubernetes.

Orion is a fork of [stolon](https://github.com/sorintlab/stolon). It keeps stolon's keeper, sentinel and proxy, and adds
a REST api with a cli on top of it, secured with TLS and client certificates.

## Components

| Component | Binary | What it does |
|-----------|--------|--------------|
| keeper | `orion-keeper` | Manages one PostgreSQL instance and converges it to the cluster view of the leader sentinel. |
| sentinel | `orion-sentinel` | Discovers and monitors the keepers and proxies, and computes the optimal cluster view. |
| proxy | `orion-proxy` | The access point for clients. It only forwards connections to the current primary, and closes connections to old primaries. |
| api | `orion-api` | Manages Orion clusters with a REST api, with TLS and client certificates (mTLS). |
| cli | `orion-cli` | Consumes the api. |

All components keep their state in a highly available store (etcd, consul or the Kubernetes api server), so they can
run anywhere, come and go, and change their addresses.

## Documentation

- [API certificates](api/certificates.md): the certificates the api and the cli use, and how to create them.
- [Bootstrap design](api/bootstrap-design.md): the design of TLS and the bootstrap of the api.

!!! note
    The rest of the documentation still comes from stolon and describes stolon's behaviour. Until it is reviewed and
    updated for Orion, it lives in
    [docs/stolon](https://github.com/pgvillage-tools/orion/tree/main/docs/stolon) in the repository.
