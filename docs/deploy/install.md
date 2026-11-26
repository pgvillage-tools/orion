---
title: Installation
summary: Get the Orion binaries from a release, the container images from ghcr.io, or build them from source
authors:
  - Sebas Mannem
date: 2026-11-25
---

# Installation

Orion has five binaries: `orion-api`, `orion-cli`, `orion-keeper`, `orion-sentinel` and `orion-proxy` (see
[Components](../index.md#components)). There are no packages (like rpm, deb or brew) yet.

## Binaries from a release

Every [release](https://github.com/pgvillage-tools/orion/releases) has an archive per platform, with all five
binaries: `orion_v<version>_<os>_<arch>.tar.gz`, for `linux` and `darwin` (macOS), on `amd64` and `arm64`. Check the
archive against `orion_<version>_checksums.txt`.

```bash
version=0.2.1
curl -LO https://github.com/pgvillage-tools/orion/releases/download/v${version}/orion_v${version}_linux_amd64.tar.gz
curl -LO https://github.com/pgvillage-tools/orion/releases/download/v${version}/orion_${version}_checksums.txt
sha256sum --check --ignore-missing orion_${version}_checksums.txt
tar -xzf orion_v${version}_linux_amd64.tar.gz
```

`orion-keeper` runs the PostgreSQL binaries (`initdb`, `pg_ctl`, `postgres`, `pg_basebackup`, `pg_rewind`) of the
host: install PostgreSQL on the hosts of the keepers, and point `--pg-bin-path` to its `bin` directory (else the keeper
looks for them in `PATH`).

## Container images

Every release also publishes container images on ghcr.io:

| Image | Tags |
|-------|------|
| `ghcr.io/pgvillage-tools/orion/api` | `<version>` (e.g. `v0.2.1`), `latest` |
| `ghcr.io/pgvillage-tools/orion/cli` | As for the api |
| `ghcr.io/pgvillage-tools/orion/sentinel` | As for the api |
| `ghcr.io/pgvillage-tools/orion/proxy` | As for the api |
| `ghcr.io/pgvillage-tools/orion/keeper` | `<PostgreSQL major>` (e.g. `18`), and `<PostgreSQL major>-<version>` (e.g. `18-v0.2.1`) |

There is a keeper image for each PostgreSQL major version from 14 to 18. It is the `postgres` image of that version
with `orion-keeper` in it, and runs as the `postgres` user. [Orion on Kubernetes](k8s.md) uses these images.

## From source

With Go (the version in `go.mod`) and `make`:

```bash
git clone https://github.com/pgvillage-tools/orion.git
cd orion
make build
```

The binaries are in `bin/`. `make build-images` builds the container images that the end-to-end tests use
(with `docker`, or e.g. `make build-images CONTAINER_TOOL=podman`).
