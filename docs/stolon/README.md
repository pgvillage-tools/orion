## Stolon Documentation

We suggest that you first read the [Stolon Architecture and Requirements](architecture.md) to understand the primary concepts and avoid possible mistakes.

* [Stolon Architecture and Requirements](architecture.md)
* [Commands Invocation](commands_invocation.md)
* [Cluster Specification](../cluster/spec.md) (reviewed for Orion, moved out of docs/stolon)
* [Cluster Initialization](../cluster/initialization.md) (reviewed for Orion, moved out of docs/stolon)
* [Setting instance parameters](postgres_parameters.md)
* [Custom pg_hba.conf entries](custom_pg_hba_entries.md)
* [Stolon Client](orion-cli.md)
* Backup/Restore
  * [Point In Time Recovery](../operations/pitr.md) (reviewed for Orion, moved out of docs/stolon)
* [Standby Cluster](../operations/standby-cluster.md) (reviewed for Orion, moved out of docs/stolon)

### Misc topics

* [Enabling pg_rewind](pg_rewind.md)
* [Enabling synchronous replication](syncrepl.md)
* [PostgreSQL SSL/TLS setup](ssl.md)
* [Forcing a failover](forcefailover.md)
* [Service Discovery](service_discovery.md)

### Recipes

* [Manual switchover without transactions loss](manual_switchover.md)

### Examples

* [Simple test cluster](simplecluster.md)
* [Kubernetes](../examples/kubernetes/README.md)
* [Two (or more) nodes setup](twonodes.md)

### [FAQ](faq.md)
