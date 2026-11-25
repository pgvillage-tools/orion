## orion-cli

orion command line interface

```
orion-cli [flags]
```

### Options

```
  -h, --help[=true|false]   help for orion-cli
  -H, --host string         hostname or ip of the api (default "127.0.0.1")
      --pki-dir string      directory with the client certificate of the cli, and the CA (default "~/.orion/cli")
  -p, --port uint16         port of the api (default 8443)
  -T, --timeout duration    timeout for requests to the api (default 30s)
```

### SEE ALSO

* [orion-cli bootstrap](orion-cli_bootstrap.md)	 - Bootstrap an api that started without certificates
* [orion-cli clusterdata](orion-cli_clusterdata.md)	 - Manage current cluster data
* [orion-cli connect](orion-cli_connect.md)	 - connect to an API
* [orion-cli failkeeper](orion-cli_failkeeper.md)	 - Force keeper as "temporarily" failed. The sentinel will compute a new clusterdata considering it as failed and then restore its state to the real one.
* [orion-cli init](orion-cli_init.md)	 - Initialize a new cluster
* [orion-cli pki](orion-cli_pki.md)	 - Manage the certificates of Orion
* [orion-cli promote](orion-cli_promote.md)	 - Promotes a standby cluster to a primary cluster
* [orion-cli removekeeper](orion-cli_removekeeper.md)	 - Removes a keeper that is gone for good from the cluster data
* [orion-cli spec](orion-cli_spec.md)	 - Retrieve the current cluster specification
* [orion-cli status](orion-cli_status.md)	 - Display the current cluster status
* [orion-cli update](orion-cli_update.md)	 - Update a cluster specification
* [orion-cli version](orion-cli_version.md)	 - Display the version

