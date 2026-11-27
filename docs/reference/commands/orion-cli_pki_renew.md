## orion-cli pki renew

Renew certificates of the cli and the api

### Options

```
  -h, --help[=true|false]   help for renew
```

### Options inherited from parent commands

```
  -H, --host string        hostname or ip of the api (default "127.0.0.1")
      --pki-dir string     directory with the client certificate of the cli, and the CA (default "~/.orion/cli")
  -p, --port uint16        port of the api (default 8443)
  -T, --timeout duration   timeout for requests to the api (default 30s)
```

### SEE ALSO

* [orion-cli pki](orion-cli_pki.md)	 - Manage the certificates of Orion
* [orion-cli pki renew api](orion-cli_pki_renew_api.md)	 - Renew the server certificate of the api, in a directory to copy to the api
* [orion-cli pki renew cli](orion-cli_pki_renew_cli.md)	 - Renew the client certificate of the cli

