## orion-cli clusterdata write

Write cluster data

```
orion-cli clusterdata write [flags]
```

### Options

```
  -f, --file string         file containing the new cluster data
  -h, --help[=true|false]   help for write
      --if-match string     only write when the cluster data still has this ETag (as printed by clusterdata read)
```

### Options inherited from parent commands

```
  -H, --host string        hostname or ip of the api (default "127.0.0.1")
      --pki-dir string     directory with the client certificate of the cli, and the CA (default "~/.orion/cli")
  -p, --port uint16        port of the api (default 8443)
  -T, --timeout duration   timeout for requests to the api (default 30s)
```

### SEE ALSO

* [orion-cli clusterdata](orion-cli_clusterdata.md)	 - Manage current cluster data

