# Administrator guide

This guide is for the people who set up and run Orion clusters. Start with a simple test cluster on your own machine,
then initialize a real cluster, create the certificates for the api and the cli, and configure and operate it.

For the options of every command, see the [Reference](../reference/README.md).

<!--nav-->
* Getting started
    * [Simple test cluster](examples/simplecluster.md)
    * [Two (or more) nodes](examples/twonodes.md)
    * [Kubernetes](https://github.com/pgvillage-tools/orion/tree/main/examples/kubernetes)
* [Cluster initialization](initialization.md)
* [The orion client](orion-cli.md)
* [Certificates for the api and the cli](api-certificates.md)
* Configuration
    * [Custom pg_hba.conf entries](custom_pg_hba_entries.md)
    * [PostgreSQL SSL/TLS](ssl.md)
    * [Synchronous replication](syncrepl.md)
    * [pg_rewind](pg_rewind.md)
* Operations
    * [Forcing a failover](forcefailover.md)
    * [Manual switchover](manual_switchover.md)
    * [Standby cluster](standbycluster.md)
* Backup and restore
    * [Point in time recovery](backup-and-restore/pitr.md)
    * [With wal-g](backup-and-restore/pitr_wal-g.md)
    * [With wal-e](backup-and-restore/pitr_wal-e.md)
