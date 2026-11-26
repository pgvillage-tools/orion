## orion-keeper



```
orion-keeper [flags]
```

### Options

```
      --can-be-master[=true|false]                whether the keeper can be elected as master (false prevents it) (default true)
      --can-be-synchronous-replica[=true|false]   whether the keeper can be chosen as synchronous replica (false prevents it) (default true)
      --cluster-name string                       cluster name
      --data-dir string                           data directory
      --disable-data-dir-locking[=true|false]     disable locking on data dir. Warning! It'll cause data corruptions if two keepers are concurrently running with the same data dir.
  -h, --help[=true|false]                         help for orion-keeper
      --log-color[=true|false]                    enable color in log output (default if attached to a terminal)
      --log-components string                     log levels per component, over --log-level, e.g. keeper=debug,kv-store=warn (a component without a level is at debug); components: keeper, kv-store, orion-cli, postgres, postgres-utils, proxy, sentinel, web-api
      --log-level string                          debug, info (default), warn or error (default "info")
      --metrics-listen-address string             metrics listen address i.e "0.0.0.0:8080" (disabled by default)
      --pg-advertise-address string               postgresql instance address from outside. Use it to expose ip different than local ip with a NAT networking config
      --pg-advertise-port string                  postgresql instance port from outside. Use it to expose port different than local port with a PAT networking config
      --pg-bin-path string                        absolute path to postgresql binaries. If empty they will be searched in the current PATH
      --pg-listen-address string                  postgresql instance listening address, local address used for the postgres instance. For all network interface, you can set the value to '*'.
      --pg-port string                            postgresql instance listening port (default "5432")
      --pg-repl-auth-method string                postgres replication user auth method: scram-sha-256, md5, trust, cert, ident or peer (default "scram-sha-256")
      --pg-repl-connection-type string            postgres replication user connection type. Default is host. (default "host")
      --pg-repl-local-auth-method string          postgres replication user auth method. Default is same as pg-repl-auth-method.
      --pg-repl-password string                   postgres replication user password. Only one of --pg-repl-password or --pg-repl-passwordfile must be provided. Must be the same for all keepers.
      --pg-repl-passwordfile string               postgres replication user password file. Only one of --pg-repl-password or --pg-repl-passwordfile must be provided. Must be the same for all keepers.
      --pg-repl-ssl-mode string                   postgres replication user ssl-mode. Default is prefer. (default "prefer")
      --pg-repl-username string                   postgres replication user name. Required. It'll be created on db initialization. Must be the same for all keepers.
      --pg-su-auth-method string                  postgres superuser auth method: scram-sha-256, md5, trust, cert, ident or peer (default "scram-sha-256")
      --pg-su-connection-type string              postgres superuser connection type. Default is host. (default "host")
      --pg-su-local-auth-method string            postgres superuser auth method. Default is same as pg-su-auth-method.
      --pg-su-password string                     postgres superuser password. Only one of --pg-su-password or --pg-su-passwordfile must be provided. Must be the same for all keepers.
      --pg-su-passwordfile string                 postgres superuser password file. Only one of --pg-su-password or --pg-su-passwordfile must be provided. Must be the same for all keepers)
      --pg-su-username string                     postgres superuser user name. Used for keeper managed instance access and pg_rewind based synchronization. It'll be created on db initialization. Defaults to the name of the effective user running orion-keeper. Must be the same for all keepers.
      --store-backend string                      store backend type (etcdv3 or kubernetes)
      --store-ca-file string                      verify certificates of HTTPS-enabled store servers using this CA bundle
      --store-cert-file string                    certificate file for client identification to the store
      --store-endpoints string                    a comma-delimited list of store endpoints (use https for tls) (default: http://127.0.0.1:2379)
      --store-key string                          private key file for client identification to the store
      --store-prefix string                       the store base prefix (default "orion/cluster")
      --store-skip-tls-verify[=true|false]        skip store certificate verification (insecure!!!)
      --store-timeout duration                    store request timeout (default 5s)
      --uid string                                keeper uid (must be unique in the cluster and can contain only lower-case letters, numbers and the underscore character). If not provided a random uid will be generated.
      --wal-dir string                            wal directory
```

