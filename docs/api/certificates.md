---
title: Certificates for the api and the cli
summary: Create the CA and the certificates of the Orion api and cli with orion-cli pki init, and run the api with them
authors:
  - Sebas Mannem
date: 2026-11-07
---

# Certificates for the api and the cli

The Orion api only accepts TLS connections with a client certificate (mTLS). This page describes how to create the
certificates with `orion-cli pki init`, and how to run the api and the cli with them. For the design behind it, see
[TLS and bootstrap design](bootstrap-design.md).

## TL;DR

On the machine where you run the cli:

```bash
orion-cli pki init --host orion-api.example.com
```

Copy `~/.orion/api` to the api host (e.g. to `/etc/orion/api`), and start the api:

```bash
orion-api --pki-dir /etc/orion/api --cluster-name mycluster --store-backend etcdv3
```

Check the connection from the cli:

```bash
orion-cli --host orion-api.example.com connect
# Connected to the api as orion-cli (client-certificate)
```

## What `orion-cli pki init` creates

```text
Root CA
├── server intermediate
│   └── orion-api      (server certificate of the api)
└── api-clients intermediate
    └── orion-cli      (client certificate of the cli)
```

| Directory | File | Contents |
|-----------|------|----------|
| `~/.orion/api` (`--api-dir`) | `server.crt` | The server certificate of the api, followed by the server intermediate |
| | `server.key` | The private key of the server certificate |
| | `client-ca.crt` | The api-clients intermediate: the api only accepts client certificates signed by it |
| `~/.orion/cli` (`--pki-dir`) | `tls.crt`, `tls.key` | The client certificate of the cli, and its private key |
| | `ca.crt` | The root, to verify the api |
| | `ca/` | The certificates and private keys of the root and the intermediates |

- Private keys are only readable for their owner (mode `0600`).
- The api directory has no CA key. Keep `~/.orion/cli/ca` safe: whoever has it can create certificates that the api
  accepts. For now the CA keys are stored unencrypted; encryption with a passphrase is planned.
- `orion-cli pki init` doesn't overwrite existing files, unless you add `--force`.

### Options

| Option | Default | Description |
|--------|---------|-------------|
| `--host`, `-H` | The configured api host | Added to the api certificate. |
| `--api-san` | | Extra names or IP addresses for the api certificate (can be repeated), e.g. a load balancer or a Kubernetes Service name. |
| `--api-dir` | `~/.orion/api` | Where the api files are stored. |
| `--pki-dir` | `~/.orion/cli` | Where the cli files are stored. |
| `--key-algorithm` | `ecdsa-p256` | `ecdsa-p256`, `rsa-2048`, `rsa-3072` or `rsa-4096`. |
| `--root-validity` | `5y` | Validity of the root. Accepts years (`y`), days (`d`) and Go durations (e.g. `8760h`). |
| `--intermediate-validity` | `3y` | Validity of the intermediates. |
| `--leaf-validity` | `1y` | Validity of the api and cli certificates. |
| `--force` | | Overwrite existing files. |

The api certificate is always valid for `localhost`, `127.0.0.1` and `::1`.

## Running the api

| Option | Environment variable | Default | Description |
|--------|----------------------|---------|-------------|
| `--port` | `ORIONAPI_PORT` | `8443` | The api: TLS, a client certificate is required. |
| `--probe-port` | `ORIONAPI_PROBE_PORT` | `8080` | `/healthz`, `/readyz` and `/status`: plain HTTP, no authentication. |
| `--pki-dir` | `ORIONAPI_PKI_DIR` | `~/.orion/api` | The directory with `server.crt`, `server.key` and `client-ca.crt`. |
| `--tls-cert-file` | `ORIONAPI_TLS_CERT_FILE` | `<pki-dir>/server.crt` | Use another file for the server certificate. |
| `--tls-key-file` | `ORIONAPI_TLS_KEY_FILE` | `<pki-dir>/server.key` | Use another file for the private key. |
| `--tls-ca-file` | `ORIONAPI_TLS_CA_FILE` | `<pki-dir>/client-ca.crt` | Use another file for the CA of the client certificates. |

- The api doesn't start without its certificates. Starting without certificates (bootstrap mode) is planned.
- The api reads its certificates at start. After you replace them (e.g. after a renewal), restart the api.
- At start, and once a day, the api logs a warning for every certificate it loaded that expires within 30 days.
- The api container image reads its certificates from `/etc/orion/api` (`ORIONAPI_PKI_DIR`). Mount them there, e.g.
  from a Kubernetes Secret, and make sure the `orion` user can read the private key.

### Probe port

| Endpoint | Returns |
|----------|---------|
| `/healthz` | 200 when the api and its store are healthy. |
| `/readyz` | 200 when the api serves. |
| `/status` | The mode of the api, e.g. `{"mode":"normal"}`. |

## Using the cli

All cli commands that talk to the api log in with the client certificate in `--pki-dir` (default `~/.orion/cli`).
`orion-cli connect` checks the connection: it calls `/v1/whoami`, which returns the identity the api sees.

You can also use another client, e.g. curl:

```bash
curl --cacert ~/.orion/cli/ca.crt --cert ~/.orion/cli/tls.crt --key ~/.orion/cli/tls.key \
  https://orion-api.example.com:8443/v1/whoami
```

## Troubleshooting

| Error | Cause |
|-------|-------|
| `certificates not found` (api) | The files are not in `--pki-dir`. Run `orion-cli pki init`, and copy the api files. |
| `client certificate not found` (cli) | The files are not in the `--pki-dir` of the cli. Run `orion-cli pki init`. |
| `certificate signed by unknown authority` | The cli and the api use certificates of different CAs. Copy the api files of the same `pki init` to the api. |
| `certificate is valid for ..., not ...` | The name you connect to is not on the api certificate. Add it with `--api-san`, and run `pki init --force`. |
| `tls: certificate required` (client) | The client didn't send a client certificate. |
