package v1

// InfoCluster defines the cluster info tht will be returned by the API
type InfoCluster struct {
	Proxies   InfoProxies       `json:"proxies"`
	Keepers   InfoKeepers       `json:"keepers"`
	Sentinels InfoSentinels     `json:"sentinels"`
	Status    InfoGenericStatus `json:"status"`
}

// InfoKeepers defines the keepers info tht will be returned by the API
type InfoKeepers []InfoKeeper

// InfoKeeper defines the keeper info tht will be returned by the API
type InfoKeeper struct {
	UID                 string            `json:"uid"`
	ListenAddress       string            `json:"listen_address"`
	Healthy             bool              `json:"healthy"`
	PgHealthy           bool              `json:"pg_healthy"`
	PgWantedGeneration  int64             `json:"pg_wanted_generation"`
	PgCurrentGeneration int64             `json:"pg_current_generation"`
	Status              InfoGenericStatus `json:"status"`
}

// InfoSentinels defines the sentinels info tht will be returned by the API
type InfoSentinels []InfoSentinel

// InfoSentinel defines the sentinel info tht will be returned by the API
type InfoSentinel struct {
	UID    string            `json:"uid"`
	Leader bool              `json:"leader"`
	Status InfoGenericStatus `json:"status"`
}

// InfoProxies defines the proxies info tht will be returned by the API
type InfoProxies []InfoProxy

// InfoProxy defines the proxy info tht will be returned by the API
type InfoProxy struct {
	UID        string            `json:"uid"`
	Generation int64             `json:"generation"`
	Status     InfoGenericStatus `json:"status"`
}

// InfoGenericStatus cn be used to add key value pairs to the results
type InfoGenericStatus map[string]any
