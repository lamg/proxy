package proxy

import (
	h "net/http"
	"net/url"
	"time"
)

// IfaceParentProxy is the signature of a function that
// from method, url, IP and time, determines the
// network interface and, optionally, a parent proxy for making
// the connection
type IfaceParentProxy func(string, string, string,
	time.Time) (string, *url.URL, error)

// ControlConn is the signature of a function that returns if
// is possible carriyng on with the operation at the connection
// with the internet host, made by the proxy. The first parameter
// is the name of the operation, which is one stored in at the
// constants Request, Report and Close. The second parameter
// is the client IP address that originated the connection with
// the remote host. The third is an amount of bytes requested
// by the client or the amount actually read from the remote
// host. Both amounts are sent with Request and Report constants
// respectively. When the connection is closed the
// Close operation is sent as parameter.
type ControlConn func(int, string, int) error

const (
	Request = iota
	Report
	Close
)

// DefaultIfaceProxyFromEnv is the IfaceParentProxy implementation
// for selecting the default interface and the parent proxy
// from environment
func DefaultIfaceProxyFromEnv(meth, ürl, ip string,
	t time.Time) (iface string, p *url.URL,
	e error) {
	p, e = h.ProxyFromEnvironment(nil)
	return
}

// UnrestrictedConn is the ControlConn implementation for allowing
// the dialed connection perform its default behavior
func UnrestrictedConn(operation int, ip string,
	amount int) (e error) {
	return
}
