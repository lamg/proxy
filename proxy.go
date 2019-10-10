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
	"context"
	"io"
	"net"
	h "net/http"
	"time"

	fh "github.com/valyala/fasthttp"
	gp "golang.org/x/net/proxy"
)

type Proxy struct {
	now      func() time.Time
	ctl      ConnControl
	trans    *h.Transport
	fastCl   *fh.Client
	dialFunc func(string) func(string, string) (net.Conn, error)
}

// NewProxy creates a net/http.Handler ready to be used
// as an HTTP/HTTPS proxy server in conjunction with
// a net/http.Server
func NewProxy(
	ctl ConnControl,
	dialer func(string) func(string, string) (net.Conn, error),
	now func() time.Time,
) (p *Proxy) {
	prx := &Proxy{
		now:      now,
		ctl:      ctl,
		dialFunc: dialer,
		trans:    new(h.Transport),
	}
	prx.trans.DialContext = prx.dialContext
	gp.RegisterDialerType("http", newHTTPProxy)
	p = prx
	return
}

type reqParamsKT string

const reqParamsK = "reqParams"

type reqParams struct {
	method string
	ip     string
	ürl    string
}

func (p *Proxy) ServeHTTP(w h.ResponseWriter,
	r *h.Request) {
	i := &reqParams{method: r.Method, ürl: r.URL.Host}
	var e error
	i.ip, _, e = net.SplitHostPort(r.RemoteAddr)
	if e == nil {
		c := context.WithValue(r.Context(), reqParamsK, i)
		nr := r.WithContext(c)
		if r.Method == h.MethodConnect {
			p.handleTunneling(w, nr)
		} else {
			p.handleHTTP(w, nr)
		}
	} else {
		h.Error(w, "Malformed remote address "+r.RemoteAddr,
			h.StatusBadRequest)
	}
}

func (p *Proxy) handleTunneling(w h.ResponseWriter,
	r *h.Request) {
	destConn, e := p.dialContext(r.Context(), "tcp", r.Host)
	var hijacker h.Hijacker
	status := h.StatusOK
	if e == nil {
		w.WriteHeader(status)
		var ok bool
		hijacker, ok = w.(h.Hijacker)
		if !ok {
			e = noHijacking()
		}
	} else {
		status = h.StatusServiceUnavailable
	}
	var clientConn net.Conn
	if e == nil {
		clientConn, _, e = hijacker.Hijack()
	} else {
		status = h.StatusInternalServerError
	}
	if e == nil {
		copyConns(destConn, clientConn)
	} else {
		status = h.StatusServiceUnavailable
	}

	if e != nil {
		h.Error(w, e.Error(), status)
	}
}

func (p *Proxy) handleHTTP(w h.ResponseWriter,
	req *h.Request) {
	resp, e := p.trans.RoundTrip(req)
	if e == nil {
		copyHeader(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, e = io.Copy(w, resp.Body)
		resp.Body.Close()
	} else {
		h.Error(w, e.Error(), h.StatusServiceUnavailable)
	}
}
