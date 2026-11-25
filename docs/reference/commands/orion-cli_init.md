## orion-cli init

Initialize a new cluster

```
orion-cli init [cluster spec] [flags]
```

### Options

```
  -f, --file string             file with the cluster spec, - for stdin (without a spec: a new cluster with the default settings)
  -h, --help[=true|false]       help for init
      --wait[=true|false]       wait until the cluster applied the change (available, and every healthy keeper converged)
      --wait-timeout duration   how long --wait waits at most (default 5m0s)
```

### Options inherited from parent commands

```
  -H, --host string        hostname or ip of the api (default "127.0.0.1")
      --pki-dir string     directory with the client certificate of the cli, and the CA (default "~/.orion/cli")
  -p, --port uint16        port of the api (default 8443)
  -T, --timeout duration   timeout for requests to the api (default 30s)
```

### SEE ALSO

* [orion-cli](orion-cli.md)	 - orion command line interface

