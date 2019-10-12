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
	"fmt"
	"net"
	"net/url"
	"time"

	gp "golang.org/x/net/proxy"
)

// IfaceDialer dials connections using the supplied network
// interface and timeout. It implements the
// golang.org/x/proxy.Dialer interface, and that allows to use it
// as parameter for DialProxy. If the interface is the empty string
// it uses the default provided by the OS.
type IfaceDialer struct {
	Interface string
	Timeout   time.Duration
}

func (d *IfaceDialer) Dial(network, addr string) (n net.Conn,
	e error) {
	dlr := &net.Dialer{
		Timeout: d.Timeout,
	}
	var nf *net.Interface
	nf, e = net.InterfaceByName(d.Interface)
	var laddr []net.Addr
	if e == nil {
		laddr, e = nf.Addrs()
	}
	if len(laddr) != 0 {
		dlr.LocalAddr = &net.TCPAddr{IP: laddr[0].(*net.IPNet).IP}
	} else {
		e = &NoLocalIPErr{Interface: d.Interface}
	}
	if e == nil {
		n, e = dlr.Dial(network, addr)
	}
	return
}

// DialProxy dials using a parent proxy if it can be reached
// using the supplied dialer
func DialProxy(network, addr string, parentProxy *url.URL,
	direct gp.Dialer) (n net.Conn, e error) {
	var d gp.Dialer
	d, e = gp.FromURL(parentProxy, direct)
	if e == nil {
		n, e = d.Dial(network, addr)
	}
	return
}

// NoLocalIPErr implements error and is returned when there
// is no local IP associated to a network interface name
type NoLocalIPErr struct {
	Interface string
}

func (e *NoLocalIPErr) Error() (s string) {
	s = fmt.Sprintf("No local IP for '%s'", e.Interface)
	return
}
