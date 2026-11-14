# End-to-end tests

Each directory is a Ginkgo suite. Unit tests live next to the code, not here.

| Suite | What it covers | Runs on | Make target |
|-------|----------------|---------|-------------|
| `system` | What orion does, whatever the store: the api and its probes, failover, replication, proxy fencing, sync replication | containers (podman or docker) with etcd as the store | `make e2e-test` |
| `cli` | The path of a user: bootstrap and manage a cluster with only `orion-cli` | containers, etcd | `make e2e-test` |
| `k8s` | The kubernetes store backend: orion as pods on kind, the Lease election, pod annotations | kind | `make k8s-e2e-test` |

`helpers` holds what the container suites share (`Cluster`, the containers, Postgres helpers).

## etcd is the basis for `system`

`system` runs on etcd, and that also covers the etcd backend. There is no
separate etcd suite:

- the orion code for etcd (`internal/consensus/etcdv3.go`: election, leases, atomic
  writes) is covered by unit tests, and every `system` spec runs on top of it;
- the store-down specs (the api returns 500, readyz returns 503, the proxy fences)
  pause the etcd container;
- what is left would test etcd itself or its client library, not orion.

So a spec goes into `system` unless it is about the kubernetes backend (`k8s`) or
the path of a user with the cli (`cli`). Postgres scenarios (replication, failover,
sync replication) stay in `system` only: keepers and sentinels behave the same on
both backends.

## Running

```sh
make build-images                        # once, and after changing code
make e2e-test                            # every suite in E2E_SUITES (system cli)
make e2e-test E2E_SUITES=cli             # one suite
make kind-e2e-setup k8s-e2e-test         # the k8s suite on a kind cluster
```

With podman, set `CONTAINER_TOOL=podman` and point `DOCKER_HOST` at the podman socket.

## tests/integration

The integration tests from stolon. They need the binaries (`ORIONKEEPER_BIN` and
the like) and don't run in CI; they are kept as a source for scenarios to port to
`system`.
