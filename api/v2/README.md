# api/v2 docs

## design

Cluster: A Cluster  consists of  group of servers, is reachable by API's and is running all processes for all topologies
- API's: All API' instances that are serving API methods for this cluster
- Topologies: All Cluster topologies on this cluster
- Nodes: All nodes (either hardwae servers, VM's or pods) that are running all processes for this cluster
- Stores: All consensus stores that provide a consistent state of the config together

Topology: Every topologie is a logical unit of PostgreSQL instances in a certain topology.
- Proxies: Every topology has one or more proxies. The proxies point to 'the master instance' of the 'master replicaset' of the toplogy.
- Replicasets: Every replicaset is a set of PostgreSQL instances that are replicating using physical replication
- Streams: Streams define a streaming replication connection between 2 replicasets. This could be Physical Replication (standby cluster), Logical Replication, BDR, or Kafka.

Replicasets: A Physical Replication cluster, they should be considered multiple instances replicating from primary to standby's. Roles are not hrd defined.
- Keepers: Every Keeper is a PostgreSQL instnce
- Sentinels: Every sentinel takes place in voting a new primary of the original standby is demoted or in failure
