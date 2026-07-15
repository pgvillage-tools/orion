package api_client

import (
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
)

// Connection is a resource to connect to the orion API
type Connection struct {
	protocol endpoints.Protocol
	host     string
	port     uint16
}

// NewConnection returns an initialized Connection
func NewConnection(protocol endpoints.Protocol, host string, port uint16) Connection {
	return Connection{
		protocol: protocol,
		host:     host,
		port:     port,
	}
}

// EndpointURL calulates and returns the url for a specific endpoint on this Connection
func (c Connection) EndpointURL(ep endpoints.EndPoint) string {
	return ep.URL(c.protocol, c.host, c.port)
}
