package cmd

import (
	"fmt"
	"net/http"
	"strings"
)

// Protocol defines a protocol (basically either http or https in our case)
type Protocol string

const (
	// HTTPS is used to specify the https protocol
	HTTPS = "https"
	// HTTP is used to specify the http protocol
	HTTP = "http"
)

// Method defines the HTTP
type Method string

const (
	methodGet    = http.MethodGet
	methodPatch  = http.MethodPatch
	methodPost   = http.MethodPost
	methodPut    = http.MethodPut
	methodDelete = http.MethodDelete
)

type EndPoint string

func (ep EndPoint) clean() string {
	return strings.Trim(string(ep), "/")
}
func (ep EndPoint) URL(protocol Protocol, ip string, port uint16) string {
	return fmt.Sprintf("%s://%s:%d/%s", protocol, ip, port, ep.clean())
}

func (ep EndPoint) route(method Method) string {
	return fmt.Sprintf("%s /%s", method, ep.clean())
}
