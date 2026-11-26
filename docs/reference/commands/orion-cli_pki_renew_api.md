## orion-cli pki renew api

Renew the server certificate of the api

### Synopsis

Renew the server certificate of the api, signed by the server intermediate in --pki-dir.

Online (without --api-dir): the api creates a new private key, which never leaves the api, and
sends a certificate request for it. The cli signs it, and the api stores the new certificate and
serves new connections with it, without a restart. The api must have a valid certificate: an
expired one is renewed offline.

Offline (with --api-dir): the cli writes a new certificate and key to --api-dir, together with the
api client intermediate (client-ca.crt): the same files as pki init. Copy them to the --pki-dir of
the api: it loads them within a minute, or right away on SIGHUP, without a restart.

The new certificate keeps the names of the current one (the one the api serves with; offline, the
one in --api-dir, when there is one), and gets localhost, 127.0.0.1, ::1, the configured api host
(--host) and the names in --api-san.

The private key of the intermediate is decrypted with the passphrase of the CA keys: it is read
from --ca-passphrase-file, else from ORIONCLI_CA_PASSPHRASE, else the cli asks for it.

The new certificate never outlives the intermediate: when the intermediate expires first, the cli
warns, and the intermediate needs to be renewed.

```
orion-cli pki renew api [flags]
```

### Options

```
      --api-dir string              renew offline: directory to write the api files to (default: renew online, on the api)
      --api-san strings             extra name or ip address for the api certificate (can be repeated)
      --ca-passphrase-file string   file with the passphrase for the CA keys (- for stdin; default: ORIONCLI_CA_PASSPHRASE, else a prompt)
  -h, --help[=true|false]           help for api
      --key-algorithm string        algorithm of the new private key (ecdsa-p256, rsa-2048, rsa-3072, rsa-4096; default: that of the current key)
      --leaf-validity string        validity of the new certificate (e.g. 1y, 90d, 2160h) (default "1y")
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

