package api_client

import (
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
)

type Connection struct {
	protocol endpoints.Protocol
	host     string
	port     uint16
}

func NewConnection(protocol endpoints.Protocol, host string, port uint16) Connection {
	return Connection{
		protocol: protocol,
		host:     host,
		port:     port,
	}
}

func (c Connection) EndpointUrl(ep endpoints.EndPoint) string {
	return ep.URL(c.protocol, c.host, c.port)
}
