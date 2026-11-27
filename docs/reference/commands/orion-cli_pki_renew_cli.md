## orion-cli pki renew cli

Renew the client certificate of the cli

### Synopsis

Replace the client certificate of the cli in --pki-dir, and its private key, with a new one.

The new certificate has the same subject, and is signed by the api client intermediate in
--pki-dir, so the api accepts it right away: nothing changes on the api. The current certificate
may have expired, but it must be signed by that intermediate.

The private key of the intermediate is decrypted with the passphrase of the CA keys: it is read
from --ca-passphrase-file, else from ORIONCLI_CA_PASSPHRASE, else the cli asks for it.

The new certificate never outlives the intermediate: when the intermediate expires first, the cli
warns, and the intermediate needs to be renewed.

```
orion-cli pki renew cli [flags]
```

### Options

```
      --ca-passphrase-file string   file with the passphrase for the CA keys (- for stdin; default: ORIONCLI_CA_PASSPHRASE, else a prompt)
  -h, --help[=true|false]           help for cli
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

