// Copyright 2026 PgVillage
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"crypto/x509"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	metricsNamespace = "orion"
	metricsSubsystem = "api"
	labelMethod      = "method"
	labelRoute       = "route"
	labelCode        = "code"
)

// apiMetrics holds the metrics of the api. They are registered in their own registry (see newAPIMetrics), so tests
// can create as many as they need.
type apiMetrics struct {
	registry        *prometheus.Registry
	requests        *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	certExpiry      *prometheus.GaugeVec
}

// newAPIMetrics creates the metrics of the api and registers them in registry
func newAPIMetrics(registry *prometheus.Registry) *apiMetrics {
	m := &apiMetrics{
		registry: registry,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "requests_total",
			Help:      "Number of authenticated requests on the api port, by method, route and status code",
		}, []string{labelMethod, labelRoute, labelCode}),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "request_duration_seconds",
			Help:      "Duration of the authenticated requests on the api port, by method and route",
			Buckets:   prometheus.DefBuckets,
		}, []string{labelMethod, labelRoute}),
		certExpiry: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "certificate_expiry_timestamp_seconds",
			Help: "Expiry of the certificates the api loaded at start (its certificate, its chain and the CA of " +
				"the clients), as seconds since the unix epoch",
		}, []string{"common_name", "serial", "ca"}),
	}
	registry.MustRegister(m.requests, m.requestDuration, m.certExpiry)
	return m
}

// instrument counts and times the requests that route handles. The route label is the path of the pattern (without
// the method), e.g. /v1/cluster/keepers/{uid}, so the number of series doesn't depend on the requests.
func (m *apiMetrics) instrument(route Route) http.HandlerFunc {
	path := route.Pattern
	if _, p, found := strings.Cut(route.Pattern, " "); found {
		path = p
	}
	labels := prometheus.Labels{labelRoute: path}
	return promhttp.InstrumentHandlerDuration(m.requestDuration.MustCurryWith(labels),
		promhttp.InstrumentHandlerCounter(m.requests.MustCurryWith(labels), route.Handler))
}

// setCertificates sets the expiry of the certificates
func (m *apiMetrics) setCertificates(certs []*x509.Certificate) {
	m.certExpiry.Reset()
	for _, c := range certs {
		m.certExpiry.WithLabelValues(c.Subject.CommonName, hex.EncodeToString(c.SerialNumber.Bytes()),
			strconv.FormatBool(c.IsCA)).Set(float64(c.NotAfter.Unix()))
	}
}

// handler serves the metrics of the api, together with those in gatherer (e.g. the go and process metrics and
// orion_cluster_identifier in the default registry)
func (m *apiMetrics) handler(gatherer prometheus.Gatherer) http.Handler {
	return promhttp.HandlerFor(prometheus.Gatherers{gatherer, m.registry}, promhttp.HandlerOpts{})
}
