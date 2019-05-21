// Copyright © 2018-2019 Luis Ángel Méndez Gort

// This file is part of Proxy.

// Proxy is free software: you can redistribute it and/or
// modify it under the terms of the GNU Lesser General
// Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your
// option) any later version.

// Proxy is distributed in the hope that it will be
// useful, but WITHOUT ANY WARRANTY; without even the
// implied warranty of MERCHANTABILITY or FITNESS FOR A
// PARTICULAR PURPOSE. See the GNU Lesser General Public
// License for more details.

// You should have received a copy of the GNU Lesser General
// Public License along with Proxy.  If not, see
// <https://www.gnu.org/licenses/>.

package proxy

import (
	h "net/http"
	"net/url"
	"time"
)

// ConnControl is the signature of a function that returns if
// is possible carriyng on with the connection operation
type ConnControl func(*Operation) *Result

type Operation struct {
	// One of Open, ReadRequest, ReadReport, Close
	Command int
	// IP at HTTP request that originated the connection operation
	IP string
	// Method of that request
	Method string
	// URL of that request
	URL string
	// time when the operation was created
	Time time.Time
	// Amount of bytes requested to be read or actually read
	// sent with commands ReadRequest and ReadReport respectively
	Amount int
}

type Result struct {
	// Iface is the network interface for dialing the connection.
	// This field is used only with the Open command
	Iface string
	// Proxy is the proxy URL for dialing the connection.
	// This field is used only with the Open command
	Proxy *url.URL
	Error error
}

const (
	// Open command must return the proper network interface and
	// proxy for dialing a connection. They are not used for the
	// rest of the commands
	Open = iota
	// ReadRequest command is performed before calling the Read
	// method of the underlying connection. It's sent with
	// the amount of bytes requested to read.
	ReadRequest
	// ReadReport command is performed after calling the Read
	// method of the underlynig connection. It's sent with
	// the amount of bytes actually read
	ReadReport
	// Close is performed after the underlying connection is closed
	Close
)

// DefaultConnControl is the default connection controller
// (ConnControl) returning the proper result for dialing the
// connections with the proxy from environment, using the default
// network interface
func DefaultConnControl(op *Operation) (r *Result) {
	p, e := h.ProxyFromEnvironment(nil)
	r = &Result{Proxy: p, Error: e}
	return
}
