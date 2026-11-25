## orion-cli pki init

Create the CA, the api server certificate and the cli client certificate

### Synopsis

Create a root CA, a server intermediate with the server certificate of the api, and an api client
intermediate with the client certificate of the cli.

The cli files (its client certificate, and the certificates and private keys of the root and the
intermediates) are stored in --pki-dir. The api files are stored in --api-dir: copy them to the
--pki-dir of the api.

The api certificate is valid for localhost, 127.0.0.1, ::1, the configured api host (--host) and
the names in --api-san.

All certificates get the subject of --org, --country, --state and --locality, with their own
common name (CN). There is no flag for the organisational unit (OU): the api uses the OUs of a
client certificate as the groups of the client.

The private keys of the root and the intermediates are encrypted with a passphrase (at least 8
characters). It is read from --ca-passphrase-file, else from ORIONCLI_CA_PASSPHRASE, else the cli
asks for it on the terminal. The keys are encrypted PKCS#8, so openssl can read them too.

```
orion-cli pki init [flags]
```

### Options

```
      --api-dir string                 directory to store the api files in (default "~/.orion/api")
      --api-san strings                extra name or ip address for the api certificate (can be repeated)
      --ca-passphrase-file string      file with the passphrase for the CA keys (- for stdin; default: ORIONCLI_CA_PASSPHRASE, else a prompt)
      --country string                 two letter country code (C) in the subject of the certificates, e.g. NL
      --force[=true|false]             overwrite existing files
  -h, --help[=true|false]              help for init
  -H, --host string                    hostname or ip of the api (added to the api certificate) (default "127.0.0.1")
      --intermediate-validity string   validity of the intermediates (default "3y")
      --key-algorithm string           algorithm of the private keys (ecdsa-p256, rsa-2048, rsa-3072, rsa-4096) (default "ecdsa-p256")
      --leaf-validity string           validity of the server and client certificates (default "1y")
      --locality string                locality or city (L) in the subject of the certificates
      --org string                     organisation (O) in the subject of the certificates (default "PGVillage")
      --root-validity string           validity of the root (e.g. 5y, 365d, 8760h) (default "5y")
      --state string                   state or province (ST) in the subject of the certificates
```

### Options inherited from parent commands

```
      --pki-dir string     directory with the client certificate of the cli, and the CA (default "~/.orion/cli")
  -p, --port uint16        port of the api (default 8443)
  -T, --timeout duration   timeout for requests to the api (default 30s)
```

### SEE ALSO

* [orion-cli pki](orion-cli_pki.md)	 - Manage the certificates of Orion

