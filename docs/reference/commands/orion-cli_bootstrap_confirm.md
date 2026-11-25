## orion-cli bootstrap confirm

Confirm a bootstrap with the client certificate of the cli

### Synopsis

Confirm a bootstrap with the client certificate of the cli (over mTLS), after which the api
deletes the archive with the CA keys. `orion-cli bootstrap` does this itself; use this command when that
step failed. It can be repeated.

```
orion-cli bootstrap confirm [flags]
```

### Options

```
  -h, --help[=true|false]   help for confirm
```

### Options inherited from parent commands

```
  -H, --host string        hostname or ip of the api (default "127.0.0.1")
      --pki-dir string     directory with the client certificate of the cli, and the CA (default "~/.orion/cli")
  -p, --port uint16        port of the api (default 8443)
  -T, --timeout duration   timeout for requests to the api (default 30s)
```

### SEE ALSO

* [orion-cli bootstrap](orion-cli_bootstrap.md)	 - Bootstrap an api that started without certificates

