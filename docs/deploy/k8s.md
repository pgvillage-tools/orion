---
title: Orion on Kubernetes
summary: Run the Orion keepers, sentinels, proxies and api on Kubernetes, with Kubernetes itself as the store (the kubernetes store backend)
authors:
  - Sebas Mannem
date: 2026-11-13
---

# Orion on Kubernetes

This page describes how to run Orion on Kubernetes with the `kubernetes` store backend: Kubernetes itself stores the
cluster data and does the sentinel leader election, so you don't need etcd or consul.

!!! warning "Not tested on a real cluster yet"
    This page is written from the code, and the kubernetes backend is only tested against a fake Kubernetes api in the
    unit tests. Nobody ran these steps on a real cluster yet. The manifests below are a starting point, not a
    production setup. End-to-end tests on kind, with maintained manifests, are planned. The manifests in
    `examples/kubernetes` are outdated: don't use them.

## Prerequisites

- A Kubernetes cluster with a default StorageClass (for the keeper volumes), and `kubectl` access to a namespace in
  which you can create Roles and RoleBindings. This page uses the namespace `orion` and the cluster name `mycluster`.
- `orion-cli` on your machine, to create the certificates and to talk to the api.
- Images of the components. The release workflow publishes them as:

    | Component | Image |
    |-----------|-------|
    | keeper | `ghcr.io/pgvillage-tools/orion/keeper:<pg version>` (e.g. `18`), or `<pg version>-<release>` (e.g. `18-v0.2.1`) |
    | sentinel, proxy, api, cli | `ghcr.io/pgvillage-tools/orion/<component>:<release>` (e.g. `ghcr.io/pgvillage-tools/orion/api:v0.2.2`) or `:latest` |

    The images of older releases miss the fixes the kubernetes backend needs (the Lease election). Use images of a
    release that has them, or build your own from the repository:

    ```bash
    docker build -f Dockerfile.sentinel -t registry.example.com/orion/sentinel:dev .
    docker build -f Dockerfile.keeper --build-arg PGVERSION=18 -t registry.example.com/orion/keeper:18-dev .
    # and the same for Dockerfile.proxy and Dockerfile.api
    ```

## How the kubernetes backend works

| What | Where |
|------|-------|
| The cluster data (spec and status) | Annotation `orion-clusterdata` on the ConfigMap `orion-cluster-<cluster name>`. The sentinel (or the api, on `orion-cli init`) creates it. |
| The state of each keeper, sentinel and proxy | Annotation `orion-status` on its own pod. Each component finds the others by the pod labels `component` (`orion-keeper`, `orion-sentinel` or `orion-proxy`) and `orion-cluster` (the cluster name). |
| The sentinel leader | The Lease `orion-cluster-<cluster name>` (`coordination.k8s.io/v1`). The sentinels compete for it; the api reads it. |

All objects live in the namespace of the pods. Every component (keeper, sentinel, proxy and api) needs:

| Option | Environment variable | Value |
|--------|----------------------|-------|
| `--cluster-name` | `ORION<COMPONENT>_CLUSTER_NAME` | The cluster name. It must match the `orion-cluster` label of the pods. |
| `--store-backend` | `ORION<COMPONENT>_STORE_BACKEND` | `kubernetes` |
| `--kube-resource-kind` | `ORION<COMPONENT>_KUBE_RESOURCE_KIND` | `configmap`, the only supported (and required) value. |
| | `POD_NAME` | The name of its own pod (from the downward api). The components write their state to it. |

`<COMPONENT>` is `KEEPER`, `SENTINEL`, `PROXY` or `API`; every option can be set with its environment variable
(`--some-flag` is `ORION<COMPONENT>_SOME_FLAG`). The components use the in-cluster configuration of their
ServiceAccount, and the namespace of their pod.

## RBAC

What each component does with the Kubernetes api:

| Component | pods | configmaps | leases (`coordination.k8s.io`) | events |
|-----------|------|------------|--------------------------------|--------|
| keeper | get, patch (its own pod) | get | | |
| proxy | get, patch (its own pod) | get | | |
| sentinel | get, patch (its own pod), list | get, create, update | get, create, update | create, patch |
| api | list | get, create, update | get | |

The sentinel records events about the leader election. One Role for all components is the simplest start; split it
per component (with a ServiceAccount each) to give each only what it needs.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: orion
  namespace: orion
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: orion
  namespace: orion
rules:
  - apiGroups: [""]
    resources: [pods]
    verbs: [get, list, patch]
  - apiGroups: [""]
    resources: [configmaps]
    verbs: [get, create, update]
  - apiGroups: [""]
    resources: [events]
    verbs: [create, patch]
  - apiGroups: [coordination.k8s.io]
    resources: [leases]
    verbs: [get, create, update]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: orion
  namespace: orion
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: orion
subjects:
  - kind: ServiceAccount
    name: orion
    namespace: orion
```

## Certificates for the api

The api only accepts mTLS connections, see [Certificates for the api and the cli](../api/certificates.md). Create
the certificates on your machine, with the names of the api Service on the api certificate:

```bash
orion-cli pki init --api-san orion-api --api-san orion-api.orion.svc
```

The api certificate is always valid for `localhost` and `127.0.0.1` too, so `kubectl port-forward` works without extra
names. Store the api files in a Secret:

```bash
kubectl -n orion create secret generic orion-api-pki --from-file="$HOME/.orion/api/"
```

The api image runs as uid 1000 and reads its certificates from `/etc/orion/api`. Secret files are owned by root, with
the pod's `fsGroup` as group: mount the Secret with `defaultMode: 0440` and set `fsGroup: 1000` (see the api
Deployment below). Keep `~/.orion/cli` (the CA keys and the client certificate of the cli) on your machine: it doesn't
belong in the cluster.

## Passwords for PostgreSQL

The keepers need a superuser password and a replication password, the same for all keepers:

```bash
kubectl -n orion create secret generic orion-pg \
  --from-literal=su-password="$(openssl rand -base64 24)" \
  --from-literal=repl-password="$(openssl rand -base64 24)"
```

## Sentinels

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: orion-sentinel
  namespace: orion
spec:
  replicas: 2
  selector:
    matchLabels: {component: orion-sentinel, orion-cluster: mycluster}
  template:
    metadata:
      labels: {component: orion-sentinel, orion-cluster: mycluster}
    spec:
      serviceAccountName: orion
      containers:
        - name: sentinel
          image: ghcr.io/pgvillage-tools/orion/sentinel:<release>
          env:
            - name: POD_NAME
              valueFrom: {fieldRef: {fieldPath: metadata.name}}
            - {name: ORIONSENTINEL_CLUSTER_NAME, value: mycluster}
            - {name: ORIONSENTINEL_STORE_BACKEND, value: kubernetes}
            - {name: ORIONSENTINEL_KUBE_RESOURCE_KIND, value: configmap}
            - {name: ORIONSENTINEL_METRICS_LISTEN_ADDRESS, value: "0.0.0.0:8080"}
          ports:
            - {name: metrics, containerPort: 8080}
```

## Keepers

A StatefulSet, with a volume per keeper. The keeper image runs as `postgres` (uid 999), with `/pgdata` as data
directory and `/pgwal/pgwal` as WAL directory: both need a persistent volume, or a restarted keeper loses its data.
The keeper generates its uid on the first start, and keeps it in its data directory.

```yaml
apiVersion: v1
kind: Service
metadata:
  name: orion-keeper
  namespace: orion
spec:
  clusterIP: None
  selector: {component: orion-keeper, orion-cluster: mycluster}
  ports:
    - {name: postgres, port: 5432}
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: orion-keeper
  namespace: orion
spec:
  serviceName: orion-keeper
  replicas: 2
  selector:
    matchLabels: {component: orion-keeper, orion-cluster: mycluster}
  template:
    metadata:
      labels: {component: orion-keeper, orion-cluster: mycluster}
    spec:
      serviceAccountName: orion
      securityContext:
        fsGroup: 999
      containers:
        - name: keeper
          image: ghcr.io/pgvillage-tools/orion/keeper:18-<release>
          env:
            - name: POD_NAME
              valueFrom: {fieldRef: {fieldPath: metadata.name}}
            - name: ORIONKEEPER_PG_LISTEN_ADDRESS
              valueFrom: {fieldRef: {fieldPath: status.podIP}}
            - {name: ORIONKEEPER_CLUSTER_NAME, value: mycluster}
            - {name: ORIONKEEPER_STORE_BACKEND, value: kubernetes}
            - {name: ORIONKEEPER_KUBE_RESOURCE_KIND, value: configmap}
            - {name: ORIONKEEPER_PG_SU_PASSWORDFILE, value: /etc/orion/pg/su-password}
            - {name: ORIONKEEPER_PG_REPL_USERNAME, value: repluser}
            - {name: ORIONKEEPER_PG_REPL_PASSWORDFILE, value: /etc/orion/pg/repl-password}
            - {name: ORIONKEEPER_METRICS_LISTEN_ADDRESS, value: "0.0.0.0:8080"}
          ports:
            - {name: postgres, containerPort: 5432}
            - {name: metrics, containerPort: 8080}
          volumeMounts:
            - {name: data, mountPath: /pgdata}
            - {name: wal, mountPath: /pgwal}
            - {name: pg, mountPath: /etc/orion/pg, readOnly: true}
      volumes:
        - name: pg
          secret: {secretName: orion-pg, defaultMode: 0440}
  volumeClaimTemplates:
    - metadata: {name: data}
      spec:
        accessModes: [ReadWriteOnce]
        resources: {requests: {storage: 10Gi}}
    - metadata: {name: wal}
      spec:
        accessModes: [ReadWriteOnce]
        resources: {requests: {storage: 2Gi}}
```

- The superuser is `postgres` (the user that runs the keeper) unless you set `ORIONKEEPER_PG_SU_USERNAME`. The image
  sets the replication user to `postgres` too; with the same name, both passwords must be the same. The manifest uses
  a separate `repluser`.
- The keeper advertises its pod IP to the other components. The IP changes when the pod restarts; the keeper then
  publishes the new one.

## Proxies

The proxy image listens on port 25432. Clients connect through the Service, which only reaches the primary.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: orion-proxy
  namespace: orion
spec:
  replicas: 2
  selector:
    matchLabels: {component: orion-proxy, orion-cluster: mycluster}
  template:
    metadata:
      labels: {component: orion-proxy, orion-cluster: mycluster}
    spec:
      serviceAccountName: orion
      containers:
        - name: proxy
          image: ghcr.io/pgvillage-tools/orion/proxy:<release>
          env:
            - name: POD_NAME
              valueFrom: {fieldRef: {fieldPath: metadata.name}}
            - {name: ORIONPROXY_CLUSTER_NAME, value: mycluster}
            - {name: ORIONPROXY_STORE_BACKEND, value: kubernetes}
            - {name: ORIONPROXY_KUBE_RESOURCE_KIND, value: configmap}
            - {name: ORIONPROXY_METRICS_LISTEN_ADDRESS, value: "0.0.0.0:8080"}
          ports:
            - {name: postgres, containerPort: 25432}
            - {name: metrics, containerPort: 8080}
          readinessProbe:
            tcpSocket: {port: 25432}
---
apiVersion: v1
kind: Service
metadata:
  name: orion-proxy
  namespace: orion
spec:
  selector: {component: orion-proxy, orion-cluster: mycluster}
  ports:
    - {name: postgres, port: 5432, targetPort: 25432}
```

## Api

The api pod needs `POD_NAME` too, and the `orion-cluster` label is not needed: the components don't look for the api.
For the probes, see [Probe port](../api/certificates.md#probe-port).

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: orion-api
  namespace: orion
spec:
  replicas: 1
  selector:
    matchLabels: {app: orion-api}
  template:
    metadata:
      labels: {app: orion-api}
    spec:
      serviceAccountName: orion
      securityContext:
        fsGroup: 1000
      containers:
        - name: api
          image: ghcr.io/pgvillage-tools/orion/api:<release>
          env:
            - name: POD_NAME
              valueFrom: {fieldRef: {fieldPath: metadata.name}}
            - {name: ORIONAPI_CLUSTER_NAME, value: mycluster}
            - {name: ORIONAPI_STORE_BACKEND, value: kubernetes}
            - {name: ORIONAPI_KUBE_RESOURCE_KIND, value: configmap}
          ports:
            - {name: api, containerPort: 8443}
            - {name: probe, containerPort: 8080}
          livenessProbe:
            httpGet: {path: /livez, port: 8080}
          readinessProbe:
            httpGet: {path: /readyz, port: 8080}
          volumeMounts:
            - {name: pki, mountPath: /etc/orion/api, readOnly: true}
      volumes:
        - name: pki
          secret: {secretName: orion-api-pki, defaultMode: 0440}
---
apiVersion: v1
kind: Service
metadata:
  name: orion-api
  namespace: orion
spec:
  selector: {app: orion-api}
  ports:
    - {name: api, port: 8443, targetPort: 8443}
```

## Creating the cluster

Forward the api port to your machine, and check the connection:

```bash
kubectl -n orion port-forward svc/orion-api 8443:8443 &
export ORIONCLI_CLUSTER_NAME=mycluster ORIONCLI_STORE_BACKEND=kubernetes ORIONCLI_KUBE_RESOURCE_KIND=configmap
orion-cli connect
# Connected to the api as orion-cli (client-certificate)
```

The cli talks to the api only (by default on `127.0.0.1:8443`), but it still checks the cluster name and store
options, hence the environment variables. `connect` saves the api host and port in `~/.orion/config.yml`.

Create a new cluster with the default settings, and wait until it is available:

```bash
orion-cli init --wait
```

To start with your own settings, pass a cluster spec: `orion-cli init --file spec.json` (or `-` for stdin). The
sentinel leader initializes the database on one of the keepers, and the other keepers become standbys.

## Checking the cluster

```bash
orion-cli status
```

It shows the sentinels (and which one leads), the proxies, the keepers and the primary. On the Kubernetes side:

```bash
kubectl -n orion get lease orion-cluster-mycluster      # the holder is the uid of the leading sentinel
kubectl -n orion get pods -l orion-cluster=mycluster
```

Connect to PostgreSQL through the proxy Service, e.g. `psql -h orion-proxy.orion.svc -U postgres` from a pod in the
cluster.

## Limitations

- Not tested on a real cluster yet (see the top of this page).
- The components need `--kube-resource-kind=configmap`, although the election uses a Lease and the cli doesn't use the
  store at all. This option is a leftover of stolon and may go away.
- The kubernetes backend keeps all cluster data in one annotation, so it is limited by the maximum size of the
  annotations of an object (256 KiB).
- Keepers, sentinels and proxies find each other by the labels of their pods, in their own namespace: a cluster name
  must be unique within the namespace, and the `orion-cluster` label must match `--cluster-name`.
- The api must run with certificates; starting it without certificates (bootstrap mode) is planned.
