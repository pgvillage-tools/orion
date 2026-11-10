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

The private key must be readable for the user that runs the api. In a container, see
[Running the api in a container](#running-the-api-in-a-container).

Check the connection from the cli, and create a cluster:

```bash
orion-cli --host orion-api.example.com connect
# Connected to the api as orion-cli (client-certificate)
orion-cli init           # a new cluster with the default settings; or pass a cluster spec
orion-cli status
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
| `--probe-port` | `ORIONAPI_PROBE_PORT` | `8080` | `/livez`, `/readyz`, `/status` and `/metrics`: plain HTTP, no authentication. |
| `--shutdown-delay` | `ORIONAPI_SHUTDOWN_DELAY` | `5s` | On SIGTERM, the time `/readyz` returns 503 before the api stops. |
| `--pki-dir` | `ORIONAPI_PKI_DIR` | `~/.orion/api` | The directory with `server.crt`, `server.key` and `client-ca.crt`. |
| `--tls-cert-file` | `ORIONAPI_TLS_CERT_FILE` | `<pki-dir>/server.crt` | Use another file for the server certificate. |
| `--tls-key-file` | `ORIONAPI_TLS_KEY_FILE` | `<pki-dir>/server.key` | Use another file for the private key. |
| `--tls-ca-file` | `ORIONAPI_TLS_CA_FILE` | `<pki-dir>/client-ca.crt` | Use another file for the CA of the client certificates. |

- The api doesn't start without its certificates. Starting without certificates (bootstrap mode) is planned.
- The api reads its certificates at start. After you replace them (e.g. after a renewal), restart the api.
- At start, and once a day, the api logs a warning for every certificate it loaded that expires within 30 days.
- The private key (`server.key`) must be readable for the user that runs the api, and for nobody else.

### Running the api in a container

The api image reads its certificates from `/etc/orion/api` (`ORIONAPI_PKI_DIR`), and runs as user `orion`, uid 1000.
Mount the directory with the api files there, read-only, and give it to uid 1000. `orion-cli pki init` creates it
with mode `0700`, and the key with `0600`: keep those, so only the api can read the key.

```bash
sudo chown -R 1000 /etc/orion/api
docker run -d -p 8443:8443 -p 8080:8080 \
  -v /etc/orion/api:/etc/orion/api:ro \
  -e ORIONAPI_STORE_ENDPOINTS=http://etcd:2379 \
  orion-api
```

- Owned by root (e.g. after `sudo cp -r`), the api can't read the directory or the key, and stops with
  `permission denied`.
- On a host with SELinux (Fedora, RHEL), add `,z` to the mount (`-v /etc/orion/api:/etc/orion/api:ro,z`), or the api
  can't read any file in it.
- On Kubernetes, mount them from a Secret. Its files are owned by root, with the pod's `fsGroup` as group: set
  `fsGroup: 1000` in the pod's security context, and `defaultMode: 0440` on the volume.

### Probe port

| Endpoint | Returns |
|----------|---------|
| `/livez` | 200 as long as the api process serves requests. It doesn't check the store. Use it for the liveness probe. |
| `/readyz` | 200 when the api serves, isn't shutting down and its store answers within a second; 503 otherwise. Use it for the readiness probe and load balancers. |
| `/status` | The mode of the api, e.g. `{"mode":"normal"}`. |
| `/metrics` | Prometheus metrics, see below. |

The probes follow the Kubernetes conventions: a store outage makes the api not ready (Kubernetes stops sending
requests to it), but doesn't make Kubernetes restart it, which wouldn't fix the store. On SIGTERM the api returns 503
on `/readyz` for `--shutdown-delay`, so load balancers stop sending requests, and then finishes the running requests
and stops. Keep `terminationGracePeriodSeconds` (30s by default) above the shutdown delay plus 10 seconds.

```yaml
livenessProbe:
  httpGet: {path: /livez, port: 8080}
readinessProbe:
  httpGet: {path: /readyz, port: 8080}
```

### Metrics

`/metrics` on the probe port returns the metrics of the api in the Prometheus format. The api doesn't use
`--metrics-listen-address`, which the other components use for their metrics.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `orion_api_certificate_expiry_timestamp_seconds` | gauge | `common_name`, `serial`, `ca` | Expiry of each certificate the api loaded at start (its certificate, its chain and the CA of the clients), in seconds since the unix epoch. |
| `orion_api_requests_total` | counter | `method`, `route`, `code` | Requests on the api port, after authentication. `route` is the path of the route, e.g. `/v1/cluster/keepers/{uid}`. |
| `orion_api_request_duration_seconds` | histogram | `method`, `route` | Duration of those requests. |
| `orion_cluster_identifier` | gauge | `cluster_name`, `component` | Always 1, with `component="api"`. The other components have it too. |

The api also serves the standard Go and process metrics (`go_*`, `process_*`).

To alert on certificates that expire within 30 days:

```yaml
- alert: OrionAPICertificateExpiresSoon
  expr: orion_api_certificate_expiry_timestamp_seconds - time() < 30 * 24 * 3600
  labels:
    severity: warning
  annotations:
    summary: "Certificate {{ $labels.common_name }} of the orion api expires within 30 days"
```

Because the api reads its certificates at start, the metric shows the certificates it uses, not the files on disk.
After you renew the certificates and restart the api, the alert clears.

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
| `permission denied` (api) | The api can't read its key or certificates: see [Running the api in a container](#running-the-api-in-a-container). |
| `client certificate not found` (cli) | The files are not in the `--pki-dir` of the cli. Run `orion-cli pki init`. |
| `certificate signed by unknown authority` | The cli and the api use certificates of different CAs. Copy the api files of the same `pki init` to the api. |
| `certificate is valid for ..., not ...` | The name you connect to is not on the api certificate. Add it with `--api-san`, and run `pki init --force`. |
| `tls: certificate required` (client) | The client didn't send a client certificate. |
