## orion-cli pki renew intermediate

Renew an intermediate, and the certificates it signs

### Synopsis

Replace an intermediate in --pki-dir (its certificate and private key) with a new one, with the
same subject, signed by the root, and renew the certificates it signs: they don't chain to the
new key.

  server       the server certificate of the api: written to --api-dir, like pki renew api.
  api-clients  the client certificate of the cli, in --pki-dir, like pki renew cli; the new
               intermediate is written to --api-dir as client-ca.crt.

Copy the files in --api-dir to the --pki-dir of the api: it loads them within a minute, or right
away on SIGHUP, without a restart. After renewing api-clients, the cli can't reach the api until
the api has the new client-ca.crt.

The current certificate of the intermediate is kept as ca/<intermediate>/previous.crt: pki renew
cli and pki renew api still renew certificates it signed, e.g. when this renewal stopped halfway.

The private keys of the root and the intermediates are decrypted with the passphrase of the CA
keys, which also encrypts the new key: it is read from --ca-passphrase-file, else from
ORIONCLI_CA_PASSPHRASE, else the cli asks for it.

The new intermediate never outlives the root: when the root expires first, the cli warns.

```
orion-cli pki renew intermediate {server|api-clients} [flags]
```

### Options

```
      --api-dir string                 directory to write the api files to
      --api-san strings                extra name or ip address for the api certificate (server; can be repeated)
      --ca-passphrase-file string      file with the passphrase for the CA keys (- for stdin; default: ORIONCLI_CA_PASSPHRASE, else a prompt)
  -h, --help[=true|false]              help for intermediate
      --intermediate-validity string   validity of the new intermediate (e.g. 3y, 1000d) (default "3y")
      --key-algorithm string           algorithm of the new private key (ecdsa-p256, rsa-2048, rsa-3072, rsa-4096; default: that of the current key)
      --leaf-validity string           validity of the new certificate (e.g. 1y, 90d, 2160h) (default "1y")
```

### Options inherited from parent commands

```
  -H, --host string        hostname or ip of the api (default "127.0.0.1")
      --pki-dir string     directory with the client certificate of the cli, and the CA (default "~/.orion/cli")
  -p, --port uint16        port of the api (default 8443)
  -T, --timeout duration   timeout for requests to the api (default 30s)
```

### SEE ALSO

* [orion-cli pki renew](orion-cli_pki_renew.md)	 - Renew certificates of the cli and the api

