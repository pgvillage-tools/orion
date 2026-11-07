---
title: TLS and bootstrap design
summary: How the Orion api and cli get, use and manage their certificates, from an api started without any certificates to mTLS between all components
authors:
  - Sebas Mannem
date: 2026-11-07
---

# TLS and bootstrap design

This page describes how Orion secures the connections between its components with TLS, and how the api and the cli get
their certificates. Most of it is not implemented yet: see [Roadmap](#roadmap) for what is done and what is planned.

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
| probe (`--probe-port`, default 8080) | HTTP | `/healthz`, `/readyz`, `/status` reports `bootstrap` | `/healthz`, `/readyz`, `/status` reports `normal`, later `/metrics` |

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
| `/healthz` | 200 when the process is alive (liveness). |
| `/readyz` | 200 when the api port serves, in either mode. In bootstrap mode the api must be ready too, or Kubernetes doesn't route the bootstrap traffic to it. |
| `/status` | The mode, e.g. `{"mode":"bootstrap"}` or `{"mode":"normal"}`. The cli checks it before it bootstraps. |
| `/metrics` | Prometheus metrics (later), e.g. the expiry of the certificates. |

## Bootstrap

When the api starts without certificates, it starts in bootstrap mode.

### The shared secret

The api and the cli prove to each other that they know a shared secret.

- Give the secret to the api in an environment variable or a file, not as a command-line argument (arguments are visible
  in `ps`).
- Without a secret, the api generates one (at least 128 bits) and logs it once.
- A secret you choose yourself must have a minimum length: with an active man in the middle, a weak secret can be
  guessed offline.
- The secret can be used once, expires (e.g. after 24 hours), and failed attempts are rate limited.

### The flow

```text
cli                                                    api (bootstrap mode)
 |                                                      |
 |-- GET /status (probe port) ------------------------->|
 |<-------------------------------- {"mode":"bootstrap"}|
 |                                                      |
 |== TLS (self-signed api certificate) ================>|
 |<-- challenge (api nonce) ----------------------------|
 |-- HMAC(secret, api nonce, TLS exporter), cli nonce ->|
 |<-------------- HMAC(secret, cli nonce, TLS exporter)-|
 |                                                      |
 |-- cli public key, extra SANs ----------------------->|  generate root, intermediates,
 |                                                      |  api and cli certificates
 |<-- archive ------------------------------------------|
 |-- stored -------------------------------------------->|
 |                                                      |  switch to normal mode
 |== mTLS (cli client certificate) ====================>|
 |-- confirm ------------------------------------------>|  delete CA keys and archive
 |<-- ok -----------------------------------------------|
```

1. The cli checks on the probe port that the api is in bootstrap mode.
2. The cli generates its key pair in `~/.orion/cli/`, with the private key encrypted with a passphrase.
3. The cli connects to the api port. The api has a temporary self-signed certificate, so the cli can't verify it.
4. The api and the cli prove to each other that they know the secret, with HMAC. The HMAC also covers keying material
   exported from the TLS session (`ConnectionState.ExportKeyingMaterial`, see
   [RFC 9266](https://www.rfc-editor.org/rfc/rfc9266)). The secret itself is never sent.
5. The cli sends its public key, and optionally extra names for the api certificate.
6. The api generates the root, the intermediates, its own server certificate and the cli client certificate, and sends
   the archive:
   - all certificates and chains, as they are (they are public);
   - the private key of the cli certificate and the private keys of the root and the intermediates, encrypted with the
     public key of the cli.
7. The cli stores the archive and reports `stored`.
8. The api switches to normal mode, but keeps the CA keys.
9. The cli connects again, now with mTLS and its new client certificate, and confirms. This proves that its certificate
   works. The confirmation can be repeated.
10. The api deletes the CA keys and the archive.

After the bootstrap, the api has only its own server certificate and key, and the chains to verify clients. It can't
issue or renew any certificate by itself; that goes through the cli.

### Why this is safe

- **A man in the middle can't take over the bootstrap.** The api certificate is self-signed, so the cli can't verify it,
  and a man in the middle could set up a TLS session with each side. But the HMAC covers keying material of the TLS
  session, which differs between the two sessions, so he can't forward the proofs. And he never sees the secret.
- **The api proves itself too.** Both sides prove that they know the secret, so the cli knows it talks to the real api.
  It doesn't need to pin a CA fingerprint.
- **The CA keys are encrypted to the cli.** Only the holder of the private key of the cli can read them, also when the
  archive is intercepted or recorded.
- **The api deletes the CA keys** as soon as the cli has proven that it can use its certificate.

### State and recovery

The api stores the state of the bootstrap, so it can continue after a restart:

| State | api port | CA keys on the api |
|-------|----------|--------------------|
| `bootstrapping` | Bootstrap mode | Not yet, or generated but not handed over |
| `handed-over` | Normal mode | Yes, until the cli confirms |
| `confirmed` | Normal mode | No |

- After a restart, the api continues in the state it was in. The secret stays valid until it expires, and a CA that was
  already generated is reused.
- If the cli never confirms (e.g. it lost the archive), nobody can connect to the api. Restart the api with
  `--rebootstrap` to start over. The api doesn't fall back to bootstrap mode by itself.

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
| Private key of the cli on disk | Passphrase → scrypt or Argon2id → AES-256-GCM | ✅ |
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
| api | `~/.orion/api/` (`--pki-dir`) | Its server certificate and key, the chains to verify clients, and during the bootstrap the bootstrap state and the archive |
| cli | `~/.orion/cli/` | Its key pair, its client certificate, the chains, and the encrypted CA keys |

The directories are only readable for their owner, and private keys are stored with mode `0600`.

## Roadmap

Implemented in the first PR:

- [x] The PKI library (`pkg/tls`): root, server intermediate, client intermediate, server and client certificates,
  ECDSA P-256, validity rules.
- [x] `orion-cli pki init`: generates the CA, the intermediates, the api server certificate and the cli client
  certificate.
- [x] The api with your own certificates: mTLS on the api port, the identity of the client in the request context.
- [x] The probe port with `/healthz`, `/readyz` and `/status`.
- [x] The cli connects to the api with mTLS.

Later, each in its own PR:

- [ ] Encrypt the CA keys on the cli with a passphrase. For now, `orion-cli pki init` stores them unencrypted, readable
  for the owner only.
- [ ] Bootstrap mode, with the shared secret, the HMAC, the archive, the confirmation and `--rebootstrap`.
- [ ] Certificate management in the cli and the api (issue, renew, list), and automatic renewal.
- [ ] `/metrics` on the probe port.
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
| RSA or Ed25519 for the certificates | RSA is slow and large; Ed25519 is not supported by all PostgreSQL clients. |
