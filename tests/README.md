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
- the store-down specs (the api returns 503, readyz returns 503, the proxy fences)
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

A cluster is removed after its specs, also when one of them failed (its container logs are written to the output
first). To look at a cluster afterwards, set `ORION_TEST_KEEP=failed` (keep the clusters of failed specs) or
`ORION_TEST_KEEP=true` (keep all of them), and remove them yourself when done: clusters that keep running starve the
next ones on a local machine.

The output shows the last 300 lines of the log of each container of a failed cluster. Set `ORION_TEST_LOG_DIR` to a
directory to also get the full logs there, one file per container (e.g. `03-keeper-1-<name>.log`) in a directory per
cluster, named after its test file. CI does this and uploads them as the artifact `e2e-logs-<suite>-<pgversion>-<job>`
of a failed job.

CI runs the system suite in three jobs: `E2E_SHARD=2/3` runs the second of three parts of each suite in
`E2E_SUITES`. `scripts/e2e-shard.sh` divides the test files (every file is in one part), weighing each file by the
clusters it starts of its own. A new file needs nothing: it is divided like the others.
