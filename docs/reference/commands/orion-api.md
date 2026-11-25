## orion-api



```
orion-api [flags]
```

### Options

```
      --baseurl string                       address for the api and probe ports to listen on (default "0.0.0.0")
      --bootstrap-secret-file string         file with the bootstrap secret (default: ORIONAPI_BOOTSTRAP_SECRET, or a generated secret that is logged)
      --bootstrap-secret-ttl duration        time the bootstrap secret is valid, from the start of bootstrap mode (default 24h0m0s)
      --cluster-name string                  cluster name
      --debug[=true|false]                   enable debug logging
  -h, --help[=true|false]                    help for orion-api
      --log-color[=true|false]               enable color in log output (default if attached to a terminal)
      --log-components string                log levels per component, over --log-level, e.g. keeper=debug,kv-store=warn (a component without a level is at debug); components: keeper, kv-store, orion-cli, postgres, postgres-utils, proxy, sentinel, web-api
      --log-level string                     debug, info (default), warn or error (default "info")
      --metrics-listen-address string        metrics listen address i.e "0.0.0.0:8080" (disabled by default)
      --pki-dir string                       directory with the certificates of the api, as created by orion-cli pki init (default "~/.orion/api")
      --port uint                            port for the api (TLS, a client certificate is required) (default 8443)
      --probe-port uint                      port for /livez, /readyz, /status and /metrics (plain HTTP, no authentication) (default 8080)
      --rebootstrap[=true|false]             remove the files of a bootstrap the cli didn't confirm (e.g. it lost them), and start a new one
      --shutdown-delay duration              on SIGTERM, the time /readyz returns 503 before the api stops, so load balancers stop sending requests (default 5s)
      --store-backend string                 store backend type (etcdv3 or kubernetes)
      --store-ca-file string                 verify certificates of HTTPS-enabled store servers using this CA bundle
      --store-cert-file string               certificate file for client identification to the store
      --store-endpoints string               a comma-delimited list of store endpoints (use https for tls) (default: http://127.0.0.1:2379)
      --store-key string                     private key file for client identification to the store
      --store-prefix string                  the store base prefix (default "orion/cluster")
      --store-skip-tls-verify[=true|false]   skip store certificate verification (insecure!!!)
      --store-timeout duration               store request timeout (default 5s)
      --tls-ca-file string                   CA that signs the client certificates the api accepts (default <pki-dir>/client-ca.crt)
      --tls-cert-file string                 server certificate of the api, followed by its chain (default <pki-dir>/server.crt)
      --tls-key-file string                  private key of the server certificate (default <pki-dir>/server.key)
```

