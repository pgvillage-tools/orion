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
# Passphrase for the CA keys:
# Repeat the passphrase:
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
- The api directory has no CA key. The CA keys in `~/.orion/cli/ca` are encrypted with a passphrase (see
  [The passphrase of the CA keys](#the-passphrase-of-the-ca-keys)). Keep the directory and the passphrase safe:
  whoever has both can create certificates that the api accepts.
- The keys of the api and the cli certificates are not encrypted: the api and the cli use them without asking for a
  passphrase.
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
| `--ca-passphrase-file` | | File with the passphrase for the CA keys (`-` for stdin). |
| `--org` | `PGVillage` | Organisation (O) in the subject of the certificates. |
| `--country` | | Two letter country code (C), e.g. `NL`. |
| `--state` | | State or province (ST). |
| `--locality` | | Locality or city (L). |

The api certificate is always valid for `localhost`, `127.0.0.1` and `::1`.

All certificates get the same subject (`--org`, `--country`, `--state` and `--locality`), each with its own common
name (CN), e.g. `orion-api` and `orion-cli`. There is no option for the organisational unit (OU) on purpose: the api
uses the OUs of a client certificate as the groups of the client.

### The passphrase of the CA keys

The private keys of the root and the intermediates are encrypted with a passphrase of at least 8 characters. The cli
takes the first of:

1. the file in `--ca-passphrase-file` (one trailing newline is removed; `-` reads stdin);
2. the environment variable `ORIONCLI_CA_PASSPHRASE`;
3. a prompt on the terminal (twice, to catch typos).

Without a terminal and without 1 or 2 (e.g. in a script), `pki init` fails and writes nothing.

The keys are standard encrypted PKCS#8 (PBES2: PBKDF2-HMAC-SHA256 and AES-256-CBC, what OpenSSL 3 writes by default),
so other tools can use them with the passphrase, e.g.:

```bash
openssl pkey -in ~/.orion/cli/ca/root.key -noout -text
# Enter pass phrase for /home/me/.orion/cli/ca/root.key:
```

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
| | `ORIONAPI_BOOTSTRAP_SECRET` | Generated and logged | The secret for [bootstrap mode](#starting-without-certificates). |
| `--bootstrap-secret-file` | `ORIONAPI_BOOTSTRAP_SECRET_FILE` | | A file with the secret for bootstrap mode, instead of the environment variable. |
| `--bootstrap-secret-ttl` | `ORIONAPI_BOOTSTRAP_SECRET_TTL` | `24h` | How long the secret of bootstrap mode is valid. |
| `--rebootstrap` | `ORIONAPI_REBOOTSTRAP` | | Start a new bootstrap when the cli didn't confirm the last one. |

- Without any of its three files in `--pki-dir` (and none set with `--tls-*-file`), the api starts in bootstrap mode:
  see [Starting without certificates](#starting-without-certificates). With only a part of them, it doesn't start.
- The api reads its certificates at start. After you replace them (e.g. after a renewal), restart the api.
- At start, and once a day, the api logs a warning for every certificate it loaded that expires within 30 days.
- The cli warns on stderr when its client certificate, the api certificate or one of their chains expires within 30
  days; the output and the exit code stay the same. With an expired certificate it stops with an error that names it.
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

## Starting without certificates

Instead of creating the certificates with `orion-cli pki init` and copying the api files, you can start the api
without any certificates, and let the cli bootstrap it over the network. The api then creates the CA and all
certificates, hands the CA keys over to the cli (encrypted to a key only the cli has), and deletes them. For how this
works and why it is safe, see [Bootstrap](bootstrap-design.md#bootstrap).

Start the api with an empty, writable `--pki-dir`, and a secret of at least 20 characters (or none: the api then
generates one and logs it once, at warn level):

```bash
export ORIONAPI_BOOTSTRAP_SECRET="$(openssl rand -base64 24)"
orion-api --pki-dir /var/lib/orion/api --cluster-name mycluster --store-backend etcdv3
# /status on the probe port reports {"mode":"bootstrap"}
```

Bootstrap it from the machine where you run the cli, with the same secret:

```bash
export ORIONCLI_BOOTSTRAP_SECRET=...   # or --bootstrap-secret-file, or a prompt
orion-cli bootstrap --host orion-api.example.com --api-san orion.example.com
# Passphrase for the CA keys:
# Repeat the passphrase:
# Created the bootstrap key pair in /home/me/.orion/cli
# The api proved that it has the bootstrap secret
# Stored the certificates and the CA keys in /home/me/.orion/cli
# The api is in normal mode and deleted the CA keys: the bootstrap is done
```

- The cli stores the same files as `orion-cli pki init`, with the CA keys encrypted with the passphrase. It refuses to
  start when one of them exists already.
- The api certificate gets the hostname and the addresses of the api host, the host of `--base-url`, and the names in
  `--api-san` (e.g. a load balancer or a DNS alias), on top of `localhost`, `127.0.0.1` and `::1`.
- `orion-cli bootstrap` takes the same subject options as `pki init` (`--org`, `--country`, `--state` and
  `--locality`). The api puts that subject on all certificates, and the cli checks it did.
- The secret can be used once, and is valid for `--bootstrap-secret-ttl` (24 hours) from the start of bootstrap mode.
  The api keeps it (and its expiry) across restarts until it is used up or expired; a generated secret is stored in
  `<pki-dir>/bootstrap-state.json` (mode `0600`) for that.
- After 5 wrong secrets within a minute, the api refuses bootstrap requests for the rest of that minute.
- When the last step (the confirmation) failed, run `orion-cli bootstrap confirm`. When the cli lost its files before it
  confirmed, restart the api with `--rebootstrap` to start over; after the confirmation, `--rebootstrap` changes
  nothing.
- The api needs to write to `--pki-dir`: in a container, mount a writable volume owned by uid 1000, not a read-only
  directory or a Kubernetes Secret. Bootstrap mode supports one api instance.

### Probe port

| Endpoint | Returns |
|----------|---------|
| `/livez` | 200 as long as the api process serves requests. It doesn't check the store. Use it for the liveness probe. |
| `/readyz` | 200 when the api serves, isn't shutting down and its store answers within a second; 503 otherwise. Use it for the readiness probe and load balancers. |
| `/status` | The mode of the api: `{"mode":"normal"}`, or `{"mode":"bootstrap"}` while it waits for `orion-cli bootstrap`. |
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

## Listing the certificates

`orion-cli pki list` shows the certificates in `--pki-dir` (the client certificate of the cli, the intermediates and
the root), and the certificates the api sends (its server certificate and the server intermediate), with their expiry:

```text
$ orion-cli pki list
SOURCE  FILE                   COMMON NAME    ISSUER         SERIAL      EXPIRES           STATUS
cli     tls.crt                orion-cli      api-clients    35432de4…   2027-11-26 07:31  valid (364d left)
cli     ca/api-clients/ca.crt  api-clients    Orion root CA  66d5f6ac…   2029-11-25 07:31  valid (1094d left)
cli     ca/server/ca.crt       server         Orion root CA  27b29d88…   2029-11-25 07:31  valid (1094d left)
cli     ca/root.crt            Orion root CA  Orion root CA  b9783946…   2031-11-25 07:31  valid (1824d left)
api     -                      orion-api      server         5e0c1a7d…   2027-11-26 07:31  valid (364d left)
api     -                      server         Orion root CA  27b29d88…   2029-11-25 07:31  valid (1094d left)

The certificate of the api at orion-api.example.com:8443 is trusted by the root of the cli
```

- **The serials** are shortened in this example; the cli prints them in full (hex).
- **The status** is `valid`, `expires soon` (within 30 days), `expired` or `not valid yet`.
- **For the api**, the cli only does a TLS handshake. It doesn't send its client certificate and needs no
  passphrase, so the list also works when the certificate of the cli or of the api expired. It then checks the
  certificates of the api against the root in `--pki-dir`, for the api host, and says when they don't match (e.g.
  `certificate signed by unknown authority` for an api of another CA).
- **`--local`** lists only the files in `--pki-dir`, without connecting to the api.
- **`--format json`** prints the certificates and the result of the check as JSON.
- When the api can't be reached, the cli lists its own certificates and exits with an error.

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
