---
title: TLS and bootstrap design
summary: How the Orion api and cli get, use and manage their certificates, from an api started without any certificates to mTLS between all components
authors:
  - Sebas Mannem
date: 2026-11-07
---

# TLS and bootstrap design

This page describes how Orion secures the connections between its components with TLS, and how the api and the cli get
their certificates. See [Roadmap](#roadmap) for what is done and what is planned.

## TL;DR

- All connections use TLS, and clients log in with a client certificate (mTLS). There is no plain HTTP api, and no
  bearer token.
- Orion has its own CA: a root, one intermediate for all server certificates, and a separate client intermediate per
  service (api, PostgreSQL, etcd, RustFS, ...).
- The private keys of the root and the intermediates live with the cli, encrypted, and never stay next to the api.
- You can start the api without any certificates. It then starts in **bootstrap mode**: the cli connects with a shared
  secret, gets all certificates, and the api switches to **normal mode** with mTLS.
- You can also bring your own certificates (e.g. created with `orion-cli pki init`, or mounted from Kubernetes
  Secrets). The api then never sees a CA key.

```bash
# Generate the CA, the intermediates, the api server certificate and the cli client certificate
orion-cli pki init --host orion-api.example.com

# Copy ~/.orion/api to the api host, and start the api with it
orion-api --pki-dir ~/.orion/api
```

## Goals

- **Zero configuration**: the api can start without any certificates and create everything it needs.
- **Server and client certificates for everything**: the api, the cli, PostgreSQL, etcd, RustFS and the other Orion
  components all use TLS, with server certificates for the servers and client certificates for the clients.
- **CA keys away from the api**: the api is the component that is exposed the most. The private keys of the root and the
  intermediates are with the cli, encrypted with a passphrase. New certificates and renewals go through the cli.
- **The cli and the api manage the certificates**: no external tooling is needed (but you can use it, see
  [Bring your own PKI](#bring-your-own-pki)).

## The PKI

```text
Root CA (5y)
├── Server intermediate (3y)
│   ├── api server certificate (1y)          CN=orion-api
│   ├── PostgreSQL server certificates       (later)
│   ├── etcd server certificates             (later)
│   └── RustFS server certificates           (later)
├── api client intermediate (3y)
│   └── cli client certificate (1y)          CN=orion-cli
├── PostgreSQL client intermediate           (later)
├── etcd client intermediate                 (later)
└── RustFS client intermediate               (later)
```

- All server certificates can share the server intermediate.
- Client certificates get a separate intermediate per service. A server trusts only the client intermediate of its own
  service, so a client certificate for etcd can't be used to log in to the api, or to PostgreSQL.
- The CN of a client certificate is the identity of the client (e.g. `orion-cli`).

### Validity

| Certificate | Validity |
|-------------|----------|
| Root CA | 5 years |
| Intermediates | 3 years |
| Server and client certificates | 1 year |

- A certificate never outlives its parent: its expiry is capped at the expiry of the certificate that signs it.
- The start of the validity (`NotBefore`) is 5 minutes in the past, so hosts with a slightly late clock accept a new
  certificate.
- All validities can be configured (e.g. `orion-cli pki init --root-validity 5y --leaf-validity 1y`).
- The api logs a warning when its certificate, or a certificate in its chain, expires within 30 days.

The root is valid for 5 years instead of the usual 10, which leaves time to move to post-quantum signatures (see
[Cryptography](#cryptography)). Shorter lifetimes for the server and client certificates may follow, together with
automatic renewal.

### Names on the api certificate

A client only accepts a server certificate if the name it connects to is on it, as a Subject Alternative Name (SAN).
The api certificate always gets `localhost`, `127.0.0.1` and `::1`, plus:

| How the certificate is made | Detected names | Extra names |
|-----------------------------|----------------|-------------|
| `orion-cli pki init` | The api host of the cli (`--host`) | `--api-san` (repeatable) |
| Bootstrap mode | The hostname and IP addresses of the api host, and the host of `--base-url` | Sent by the cli |

In most cases the detected names are enough; the extra names are for load balancers, DNS aliases or Kubernetes Service
names (`orion-api.<namespace>.svc`, ...).

## Ports and modes

| Port | Protocol | Bootstrap mode | Normal mode |
|------|----------|----------------|-------------|
| api (`--port`, default 8443) | TLS | Temporary self-signed certificate, no client certificate, only the bootstrap endpoints (protected with HMAC) | Server certificate from the PKI, client certificate required, the normal api |
| probe (`--probe-port`, default 8080) | HTTP | `/livez`, `/readyz`, `/status` reports `bootstrap` | `/livez`, `/readyz`, `/metrics`, `/status` reports `normal` |

The api port has one mode at a time:

- In normal mode, the api port requires a valid client certificate during the TLS handshake
  (`RequireAndVerifyClientCert`). There are no exceptions: a connection without a valid client certificate never
  reaches an api handler.
- When the api switches from bootstrap mode to normal mode, it stops the bootstrap server, which closes all its
  connections, and starts the normal server on the same port. A bootstrap connection (without a client certificate)
  can't stay open into normal mode.

The probe port has no TLS and no authentication, and it serves nothing sensitive:

| Endpoint | Returns |
|----------|---------|
| `/livez` | 200 when the process is alive (liveness; it doesn't check the store). |
| `/readyz` | 200 when the api port serves, in either mode, isn't shutting down and the store answers; 503 otherwise. In bootstrap mode the api must be ready too, or Kubernetes doesn't route the bootstrap traffic to it. |
| `/status` | The mode, e.g. `{"mode":"bootstrap"}` or `{"mode":"normal"}`. The cli checks it before it bootstraps. |
| `/metrics` | Prometheus metrics, e.g. the expiry of the certificates. |

## Bootstrap

When the api starts without certificates, it starts in bootstrap mode.

### The shared secret

The api and the cli prove to each other that they know a shared secret.

| Setting | How | Default |
|---------|-----|---------|
| The secret | The environment variable `ORIONAPI_BOOTSTRAP_SECRET`, or a file with `--bootstrap-secret-file` (a trailing newline is ignored) | Generated: 128 random bits, as 26 base32 characters (e.g. `MZXW6YTBOI4DSMRVGA3TCNJZHE`), logged once at warn level |
| Its lifetime | `--bootstrap-secret-ttl`, from the start of bootstrap mode | `24h` |

- There is no flag for the secret itself: command-line arguments are visible in `ps`. Setting both the environment
  variable and the file is an error.
- A secret you choose yourself must have at least 20 characters: with an active man in the middle, a weak secret can be
  guessed offline.
- The secret can be used once: a successful hand-over of the archive uses it up. A proof alone doesn't: when the
  connection drops after the proof, the cli can try again.
- Wrong proofs are rate limited: after 5 within a minute (for all clients together), the api answers `429 Too Many
  Requests` with `Retry-After` until the oldest is a minute old.
- Once the secret expired, the api refuses every bootstrap request (`403`). Restart the api for a new secret.

### The flow

```text
cli                                                    api (bootstrap mode)
 |                                                      |
 |-- GET /status (probe port) ------------------------->|
 |<-------------------------------- {"mode":"bootstrap"}|
 |                                                      |
 |== TLS 1.3 (self-signed api certificate) ============>|
 |-- POST /v1/bootstrap/challenge --------------------->|
 |<------------------------------------ api nonce ------|
 |-- POST /v1/bootstrap/proof: cli nonce, cli proof --->|  check the cli proof
 |<------------------------------------ api proof ------|  check the api proof (cli)
 |                                                      |
 |-- POST /v1/bootstrap/archive: public key, SANs ---->|  generate root, intermediates,
 |                                                      |  api and cli certificates (memory)
 |<------------- certificates + keys sealed to the cli -|
 |  check and store the files                           |
 |-- POST /v1/bootstrap/stored ------------------------>|  store api files + sealed archive,
 |<-------------------------------------- stored -------|  secret used up, normal mode
 |                                                      |
 |== mTLS (cli client certificate) ====================>|
 |-- POST /v1/bootstrap/confirm ----------------------->|  delete the sealed archive
 |<----------------------------------- confirmed -------|
```

1. The cli checks on the probe port that the api is in bootstrap mode.
2. The cli generates its key pair in `~/.orion/cli/` (once), with the private key encrypted with the passphrase of
   the CA keys. See [The key pair of the cli](#the-key-pair-of-the-cli).
3. The cli connects to the api port. The api has a temporary self-signed certificate, so the cli can't verify it.
4. The api and the cli prove to each other that they know the secret, with HMAC. The HMAC also covers keying material
   exported from the TLS session (`ConnectionState.ExportKeyingMaterial`, see
   [RFC 9266](https://www.rfc-editor.org/rfc/rfc9266)). The secret itself is never sent. See
   [The bootstrap protocol](#the-bootstrap-protocol) for the details.
5. The cli sends its public key, and optionally extra names for the api certificate.
6. The api generates the root, the intermediates, its own server certificate and the cli client certificate, in memory,
   and sends the archive (see [The archive](#the-archive)):
   - all certificates, as they are (they are public);
   - the private key of the cli certificate and the private keys of the root and the intermediates, encrypted with the
     public key of the cli.
7. The cli checks the archive, stores it and reports `stored`.
8. The api stores its own certificate and key, the chain to verify clients, and the archive as it sent it (with the
   private keys encrypted to the cli, so the api can't read them). The secret is used up, and the api switches to
   normal mode.
9. The cli connects again, now with mTLS and its new client certificate, and confirms. This proves that its certificate
   works. The confirmation can be repeated (`orion-cli bootstrap confirm`).
10. The api deletes the archive.

The api never writes a CA key to disk, not even encrypted: it only has them in memory while it builds the archive.
After the bootstrap, the api has only its own server certificate and key, and the chain to verify clients. It can't
issue or renew any certificate by itself; that goes through the cli.

### The bootstrap protocol

The bootstrap endpoints are on the api port, in bootstrap mode only, and need TLS 1.3. In JSON, all byte strings are
base64 (standard, with padding). The shared parts of the protocol are in `pkg/bootstrap`, for the api and the cli.

| Endpoint | Request | Response |
|----------|---------|----------|
| `POST /v1/bootstrap/challenge` | No body | `{"nonce": <32 bytes>}`: the nonce of the api |
| `POST /v1/bootstrap/proof` | `{"nonce": <32 bytes>, "proof": <32 bytes>}`: the nonce and the proof of the cli | `{"proof": <32 bytes>}`: the proof of the api |
| `POST /v1/bootstrap/archive` | `{"publicKey": <the public key of the cli>, "sans": ["lb.example.com", ...]}` | The archive (see [The archive](#the-archive)) |
| `POST /v1/bootstrap/stored` | No body | `{"status": "stored"}`; then the api switches to normal mode |

In normal mode, with mTLS:

| Endpoint | Request | Response |
|----------|---------|----------|
| `POST /v1/bootstrap/confirm` | No body; only for the client certificate of the cli (`CN=orion-cli`) | `{"status": "confirmed"}`, also when repeated |

**One TLS connection.** The api binds a bootstrap to the TLS connection it starts on: the challenge, the proof, the
archive and `stored` must use the same connection. The api finds the bootstrap by the keying material of the connection
(see below), so there is no session token to steal: another connection, also one of a man in the middle, has other
keying material. A new challenge on a connection replaces the previous one, and a challenge without a proof expires
after 5 minutes. The api keeps at most 16 challenges without a proof (`429` after that).

**The proofs.** Both sides export 32 bytes of keying material from their TLS session, with the label
`EXPORTER-orion-bootstrap` and no context (RFC 5705, `tls.ConnectionState.ExportKeyingMaterial`). The cli picks a random
nonce of 32 bytes, and both compute an HMAC-SHA256 with the secret (as its UTF-8 bytes) as the key:

```text
ekm       = TLS-Exporter("EXPORTER-orion-bootstrap", no context, 32)
cli proof = HMAC-SHA256(secret, "orion-bootstrap cli" || 0x00 || api nonce || cli nonce || ekm)
api proof = HMAC-SHA256(secret, "orion-bootstrap api" || 0x00 || api nonce || cli nonce || ekm)
```

- The label at the start makes the two proofs different, so the api can't be made to send the proof of the cli, or the
  other way around. The parts after the label have a fixed size, so the input is unambiguous.
- The api checks the proof of the cli in constant time. A wrong proof ends the challenge: the cli asks for a new one.
- The cli checks the proof of the api the same way before it sends anything else, so it never sends its public key to
  an api that doesn't know the secret.
- TLS 1.3 is required: with TLS 1.2, exported keying material is only safe with the extended master secret extension
  (RFC 7627), which can't be checked from `crypto/tls`.

**Errors** are problem details (RFC 9457, `application/problem+json`):

| Status | When |
|--------|------|
| `400 Bad Request` | No TLS 1.3, an invalid body, a nonce or proof of the wrong size, an invalid public key, or an invalid name (or more than 32) for the api certificate |
| `403 Forbidden` | A wrong proof, the secret expired or is used up; a confirmation from another client than the cli |
| `409 Conflict` | A proof without a challenge on this connection, an archive without a proof, or `stored` without an archive |
| `429 Too Many Requests` | Too many wrong proofs, or too many open challenges (with `Retry-After`) |

### The archive

The api hands over the certificates and the private keys in one JSON object. In JSON, all byte strings are base64.

```json
{
  "certificates": {
    "root": "<PEM>",
    "serverIntermediate": "<PEM>",
    "apiClientIntermediate": "<PEM>",
    "apiServer": "<PEM>",
    "cliClient": "<PEM>"
  },
  "enc": "<the encapsulated key of HPKE>",
  "sealedKeys": "<the private keys, encrypted>"
}
```

- **The certificates** are public, so they are not encrypted. The api certificate has `localhost`, `127.0.0.1`, `::1`,
  the hostname and the addresses of the api host, the host of `--base-url` (unless it listens on all addresses), and
  the names the cli sent (`orion-cli bootstrap --api-san`).
- **The private keys** of the root, the two intermediates and the cli certificate (PKCS#8 PEM, as a JSON object) are
  encrypted with HPKE (RFC 9180, base mode) to the public key of the cli: X-Wing, HKDF-SHA256 and AES-256-GCM, with the
  info `orion-bootstrap archive v1`.
- **The additional data** of the encryption binds the keys to the TLS connection and to the certificates:

  ```text
  aad = "orion-bootstrap archive" || 0x00 || ekm || SHA-256(for each certificate: length (uint64) || PEM)
  ```

  The cli can only decrypt the archive on the connection it asked for it (a recorded archive is useless on another
  one), and nobody can swap a certificate without breaking the decryption.
- **Every request makes a new archive**, with a new CA: until the cli stored one, nothing trusts it. The api keeps the
  last archive of the connection in memory until `stored`.
- **The cli checks the archive** before it stores it: the root is a self-signed CA, the intermediates are CAs signed by
  it, the api certificate is a valid server certificate for `orion-api` and the cli certificate a valid client
  certificate for `orion-cli`, and every private key belongs to its certificate.
- **The cli stores the files** like `orion-cli pki init`: the root and the intermediates in `ca/` with their private
  keys encrypted with the passphrase of the CA keys, and `tls.crt`, `tls.key` and `ca.crt`. It doesn't overwrite any of
  these files (it refuses before it asks for the secret); when storing fails, it removes what it wrote.
- **On `stored`** the api writes `server.crt`, `server.key` and `client-ca.crt` to its `--pki-dir`, and
  `bootstrap-archive.json`: the archive as it sent it, with the keys it can't decrypt. The confirmation deletes it.

### The key pair of the cli

The api encrypts the archive to the public key of the cli, with HPKE (RFC 9180), so only the cli can read it.

- **The KEM** is ML-KEM-768 with X25519 (X-Wing, `hpke.MLKEM768X25519()`, HPKE KEM id `0x647A`): the encryption stays
  safe as long as either of the two holds, also against a quantum computer later.
- **The files** are in the directory of the cli (`--pki-dir`, default `~/.orion/cli`): `bootstrap.key` (the private key,
  encrypted, mode `0600`) and `bootstrap.pub` (the public key). `orion-cli bootstrap` creates them when they don't exist
  yet, and keeps them otherwise. One of the two without the other is an error: remove it to start over.
- **The passphrase** is the passphrase of the CA keys (`--ca-passphrase-file`, `ORIONCLI_CA_PASSPHRASE`, or a prompt):
  the archive holds the CA keys, which the cli stores encrypted with that passphrase anyway, so a second passphrase
  wouldn't protect anything more.
- **The encryption of the private key**: Argon2id (RFC 9106; 3 passes, 64 MiB, 4 threads, a random salt of 16 bytes)
  derives an AES-256-GCM key from the passphrase. The parameters are in the PEM headers, so they can change later
  without breaking older files. When reading a file, the cli accepts at most 16 passes, 1 GiB and 16 threads, so a
  crafted file can't make it use all memory.

```text
-----BEGIN ORION BOOTSTRAP ENCRYPTED PRIVATE KEY-----
Argon2-Memory: 65536
Argon2-Threads: 4
Argon2-Time: 3
KDF: argon2id
KEM: 0x647A
Nonce: <12 bytes, base64>
Salt: <16 bytes, base64>

<AES-256-GCM(the private key as HPKE SerializePrivateKey, additional data "orion bootstrap private key"), base64>
-----END ORION BOOTSTRAP ENCRYPTED PRIVATE KEY-----

-----BEGIN ORION BOOTSTRAP PUBLIC KEY-----
KEM: 0x647A

<the public key as HPKE SerializePublicKey, base64>
-----END ORION BOOTSTRAP PUBLIC KEY-----
```

### orion-cli bootstrap

```bash
ORIONCLI_BOOTSTRAP_SECRET=... orion-cli bootstrap --host orion-api.example.com
```

1. The cli asks the probe port (`--probe-port`, default `8080`) for the mode of the api, and stops unless it is
   `bootstrap`.
2. It reads the secret from `--bootstrap-secret-file` (`-` for stdin), else from `ORIONCLI_BOOTSTRAP_SECRET`, else it
   asks for it on the terminal (without echo). A secret shorter than 20 characters can't be the secret of the api, so
   the cli refuses it before it uses one of the few tries the api allows.
3. It creates its key pair when it doesn't have one yet.
4. It connects to the api port (`--host`, `--port`) with TLS 1.3, without verifying the self-signed certificate, and
   runs the challenge and the proof on that one connection. It checks the proof of the api before it sends anything
   else, and it fails when the connection changes in between.

5. It sends its public key and the names in `--api-san` (repeatable), decrypts the archive with its private key, checks
   it and stores the files in `--pki-dir`. It refuses to start when one of those files exists already.
6. It sends `stored`, waits (at most 30 seconds) until the probe port reports `normal`, and confirms over mTLS with its
   new client certificate, verifying the api with the new root. When the confirmation fails, run
   `orion-cli bootstrap confirm` to try it again.

### Why this is safe

- **A man in the middle can't take over the bootstrap.** The api certificate is self-signed, so the cli can't verify it,
  and a man in the middle could set up a TLS session with each side. But the HMAC covers keying material of the TLS
  session, which differs between the two sessions, so he can't forward the proofs. And he never sees the secret.
- **The api proves itself too.** Both sides prove that they know the secret, so the cli knows it talks to the real api.
  It doesn't need to pin a CA fingerprint.
- **The CA keys are encrypted to the cli.** Only the holder of the private key of the cli can read them, also when the
  archive is intercepted or recorded.
- **The api never stores the CA keys.** It has them in memory while it builds the archive; on disk it keeps only the
  archive, which it can't decrypt, until the cli has proven that it can use its certificate.

### State and recovery

The api stores the state of the bootstrap in `bootstrap-state.json` in its `--pki-dir` (mode `0600`), so it can continue
after a restart:

| State | api port | CA keys on the api |
|-------|----------|--------------------|
| `bootstrapping` | Bootstrap mode | In memory only, in an archive that wasn't stored yet |
| `handed-over` | Normal mode | Only in the archive, encrypted to the cli, until the cli confirms |
| `confirmed` | Normal mode | No |

- **Bootstrap mode** writes `bootstrapping`, the SHA-256 of the secret and its expiry at the start. A generated secret
  is stored too, since the api logs it only once; a secret you set (`ORIONAPI_BOOTSTRAP_SECRET` or
  `--bootstrap-secret-file`) is not, it stays where you keep it.
- **After a restart in bootstrap mode** the api continues with the same secret and expiry, as long as the secret is
  still valid (and, for a secret you set, it is the same one). Otherwise it starts over: a new generated secret (logged
  again) or the one you set, with a new expiry. A restart also makes a new CA at the next archive request: the old one
  was only in memory.
- **`stored`** writes the files of the api and the archive first, and then `handed-over` without the secret: the
  secret is used up. When the api stops in between, the archive is there while the state is still `bootstrapping`: at
  its next start, the api removes the archive and its files, and continues the bootstrap. The cli then has files
  already: move them away before bootstrapping again (the cli says so).
- **The confirmation** removes the archive and writes `confirmed`.
- **Certificates from elsewhere**: when the api finds all its files at start while the state is still `bootstrapping`
  (you put your own certificates there), it starts in normal mode and removes the state, with the secret.
- **When the cli never confirms** (e.g. it lost its files), nobody can connect to the api, which logs a warning at
  every start. Restart the api with `--rebootstrap`: it removes the files of the bootstrap (the certificate and key of
  the api, the archive and the state) and starts a new one. `--rebootstrap` only acts while the bootstrap isn't
  confirmed: after the confirmation, and with your own certificates (no state), it changes nothing and logs a warning,
  so a flag that stays set (e.g. `ORIONAPI_REBOOTSTRAP` in a manifest) can't remove a working PKI. With
  `--tls-cert-file`, `--tls-key-file` or `--tls-ca-file` it is an error.
- The api doesn't fall back to bootstrap mode by itself.

### When to bootstrap again

A bootstrap creates a new CA. Every certificate the old CA issued stops working: the ones of the api and the cli, and
later those of the other components and other cli users. So a bootstrap is for a missing PKI, not for one that expires:

| Situation | What to do |
|-----------|------------|
| The cli never confirmed the bootstrap | `--rebootstrap` (see above). |
| The CA is lost or compromised (passphrase forgotten, CA keys gone, a key leaked) | Remove the files of the api from its `--pki-dir`, and restart it: it starts in bootstrap mode. This needs access to the api host, which is the right bar for replacing the whole PKI. |
| A certificate expires | Renew it with the cli, which has the CA keys (#325). Not a bootstrap. |

An expired certificate doesn't put the api in bootstrap mode: that would let anyone who can wait for an expiry, or set
the clock of the api host forward, start a new bootstrap on a running PKI. Renewal works without a new CA:

- **The client certificate of the cli** (1 year): the cli signs a new one with the key of the client intermediate,
  locally.
- **The api certificate** (1 year): the cli signs a CSR of the api with the key of the server intermediate. The api
  keeps its private key, and never has a CA key. When the certificate has expired already, the cli trusts the
  certificate it knows from before for this one step.
- **An intermediate** (3 years): the cli issues a new one with the root key, locally, and gives the api the new chain
  in the same way.
- **The root** (5 years): a CA rotation, with a period in which both roots are trusted. That is planned work, well
  before the root expires.

The cli warns when its certificate or the one of the api expires within 30 days (#420); the api warns in its log and
with the metric `orion_api_certificate_expiry_timestamp_seconds` (see [certificates](certificates.md)).

## Bring your own PKI

When you give the api a certificate, a key and a CA (`--tls-cert-file`, `--tls-key-file` and `--tls-ca-file`), it uses
them and generates nothing: there is no bootstrap mode, and the api never has a CA key.

Use it to:

- create the certificates with the cli, so the CA keys never touch the api host:

  ```bash
  orion-cli pki init --host orion-api.example.com
  ```

- run the api on Kubernetes, with the certificates (created by the cli) mounted from Secrets;
- use certificates from your own PKI (e.g. cert-manager or Vault).

## Authentication

- In normal mode, the api port requires a client certificate signed by the api client intermediate.
- The api puts the identity of the client (the CN of its certificate) in the request context. The authentication is a
  chain of authenticators that all return the same identity (a name and groups), so other methods can be added without
  changing the handlers.
- The `--token` flag is removed (it was never checked), and so is `--tls=false`: TLS is always on.

OIDC/OAuth can be added later as another authenticator, for people who log in with an identity provider (Keycloak, Entra
ID, Okta, ...): the api checks JWT bearer tokens, and the cli logs in with the device flow. OIDC tokens come without a
client certificate, so the api port then verifies a client certificate only when it is given, and a middleware requires
either a certificate or a token.

## Cryptography

| What | Algorithm | Quantum safe |
|------|-----------|--------------|
| Certificate keys and signatures | ECDSA P-256, keys in PKCS#8 (configurable, e.g. RSA) | ❌ |
| Key pair of the cli, encryption of the archive | HPKE with ML-KEM-768 + X25519 (`crypto/hpke`, `MLKEM768X25519`) | ✅ |
| CA keys on disk (`~/.orion/cli/ca`) | Encrypted PKCS#8: PBKDF2-HMAC-SHA256 (600,000 iterations) → AES-256-CBC, readable by OpenSSL | ✅ |
| Private key of the cli on disk (`~/.orion/cli/bootstrap.key`) | Passphrase → Argon2id (3 passes, 64 MiB, 4 threads) → AES-256-GCM | ✅ |
| TLS key exchange | The Go default: X25519MLKEM768 and SecP256r1MLKEM768 | ✅ (Go clients and servers) |
| HMAC in the bootstrap | HMAC-SHA256 | ✅ |

- **Encryption** must be quantum safe now: an attacker can record the archive today and decrypt it when quantum
  computers exist ("harvest now, decrypt later"), and the archive holds the CA keys. The hybrid HPKE is safe as long as
  either ML-KEM or X25519 holds, and it is in the Go standard library.
- **Signatures** only need to hold while a certificate is valid, and they can't be broken afterwards. Go has no ML-DSA
  (FIPS 204) in `crypto/x509` yet, and most clients (libpq, JDBC, ...) can't use ML-DSA certificates. Orion uses ECDSA
  P-256 for now and moves to ML-DSA when Go and the clients support it.

## Storage

| Where | Directory | Contents |
|-------|-----------|----------|
| api | `~/.orion/api/` (`--pki-dir`) | Its server certificate and key, the chain to verify clients, the state of the bootstrap (`bootstrap-state.json`), and between the hand-over and the confirmation the archive (`bootstrap-archive.json`, the keys encrypted to the cli) |
| cli | `~/.orion/cli/` | Its bootstrap key pair (`bootstrap.key`, encrypted, and `bootstrap.pub`), its client certificate, the chains, and the encrypted CA keys |

The directories are only readable for their owner, and private keys are stored with mode `0600`.

## Roadmap

Implemented:

- [x] The PKI library (`pkg/tls`): root, server intermediate, client intermediate, server and client certificates,
  ECDSA P-256, validity rules.
- [x] `orion-cli pki init`: generates the CA, the intermediates, the api server certificate and the cli client
  certificate.
- [x] The api with your own certificates: mTLS on the api port, the identity of the client in the request context.
- [x] The probe port with `/livez`, `/readyz` and `/status` (Kubernetes conventions; graceful shutdown on SIGTERM).
- [x] The cli connects to the api with mTLS.
- [x] `/metrics` on the probe port, with the expiry of the certificates.
- [x] The CA keys on the cli are encrypted with a passphrase (encrypted PKCS#8, readable by OpenSSL).

The bootstrap:

- [x] Bootstrap mode on the api port: the temporary self-signed certificate, only the bootstrap routes, `/status`
  reports `bootstrap`, and the switch to normal mode (#319).
- [x] The shared secret and the mutual HMAC: the challenge and proof endpoints (#320).
- [x] `orion-cli bootstrap` up to the proofs, and the key pair of the cli (#321).
- [x] The archive, `stored`, the switch to normal mode and the confirmation (#322).
- [x] The stored state and secret, resuming after a restart, `--rebootstrap`, and enabling bootstrap mode (#323).

Later, each in its own PR:

- [ ] Certificate management in the cli and the api (issue, renew, list), and automatic renewal.
- [ ] Server and client certificates for PostgreSQL, etcd, RustFS and the other components.
- [ ] Several api instances (HA), with the bootstrap state in shared storage and a lock.
- [ ] Kubernetes: certificates created by the cli, mounted from Secrets.
- [ ] OIDC/OAuth authentication.
- [ ] ML-DSA signatures, when Go and the clients support them.

## Alternatives that were not chosen

| Alternative | Why not |
|-------------|---------|
| Bootstrap on the api, copy the certificates to the cli with ssh | Needs ssh access and manual steps. |
| Pin the fingerprint of the CA in the cli (like `kubeadm join`) | The shared secret also authenticates the api, so it is not needed. |
| Prove the cli key with an encrypted nonce | The shared secret replaces it. |
| Write the cli key pair on the api host when no cli key is given | The CA keys would be next to the key that decrypts them. A local cli bootstraps over `localhost` instead. |
| A separate bootstrap port | One api port with two modes is simpler, and a connection can't move from one mode to the other. |
| One port with a middleware that allows some endpoints without a client certificate | The TLS handshake can enforce mTLS by itself; the health endpoints are on the probe port. |
| Fall back to bootstrap mode automatically when the cli doesn't confirm | Hard to predict; `--rebootstrap` is explicit. |
| Bootstrap again when a certificate expires | A new CA breaks every certificate of the old one, and an expiry (or a clock set forward) would reopen bootstrap mode on a running PKI. Renewal with the cli keeps the CA. |
| RSA or Ed25519 for the certificates | RSA is slow and large; Ed25519 is not supported by all PostgreSQL clients. |
