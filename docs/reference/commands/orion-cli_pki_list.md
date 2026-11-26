## orion-cli pki list

List the certificates of the cli and the api, and when they expire

### Synopsis

List the certificates in --pki-dir (the client certificate of the cli, the root and the
intermediates), and the certificates the api sends (its server certificate and chain).

For the api, the cli only does a TLS handshake: it doesn't need a valid client certificate or the
passphrase, so it also lists an api certificate that expired. It checks the certificates of the api
against the root in --pki-dir, and reports when they don't match. Use --local to leave out the api.

A certificate that expires within 30 days is marked "expires soon".

```
orion-cli pki list [flags]
```

### Options

```
  -f, --format string        output format (text or json) (default "text")
  -h, --help[=true|false]    help for list
      --local[=true|false]   only list the certificates in --pki-dir, not those of the api
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

