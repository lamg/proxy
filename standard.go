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
	"sync"

	fh "github.com/valyala/fasthttp"
	gp "golang.org/x/net/proxy"
)

type Proxy struct {
	trans       *h.Transport
	fastCl      *fh.Client
	dialContext func(context.Context, string, string) (net.Conn, error)
}

// NewProxy creates a net/http.Handler ready to be used
// as an HTTP/HTTPS proxy server in conjunction with
// a net/http.Server
func NewProxy(
	dial func(context.Context, string, string) (net.Conn, error),
) (p *Proxy) {
	p = &Proxy{
		dialContext: dial,
		trans:       new(h.Transport),
	}
	p.trans.DialContext = p.dialContext
	gp.RegisterDialerType("http", newHTTPProxy)
	return
}

// ReqParamsKT is the type of ReqParamsK
type ReqParamsKT string

// ReqParamsK is the key sent in the context to the dialer,
// associated to a *ReqParams value
const ReqParamsK = ReqParamsKT("reqParams")

type ReqParams struct {
	Method string
	IP     string
	URL    string
}

func (p *Proxy) ServeHTTP(w h.ResponseWriter,
	r *h.Request) {
	i := &ReqParams{Method: r.Method, URL: r.URL.Host}
	var e error
	i.IP, _, e = net.SplitHostPort(r.RemoteAddr)
	if e == nil {
		c := context.WithValue(r.Context(), ReqParamsK, i)
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

func transfer(dest io.WriteCloser, src io.ReadCloser) {
	io.Copy(dest, src)
	dest.Close()
	src.Close()
}

func copyHeader(dst, src h.Header) {
	for k, vv := range src {
		ok := searchHopByHop(k)
		if !ok {
			for _, v := range vv {
				dst.Add(k, v)
			}
		}
	}
}

func copyConns(dest, client net.Conn) {
	// learning from https://github.com/elazarl/goproxy
	// /blob/2ce16c963a8ac5bd6af851d4877e38701346983f
	// /https.go#L103
	clientTCP, cok := client.(*net.TCPConn)
	destTCP, dok := dest.(*net.TCPConn)
	if cok && dok {
		go transfer(destTCP, clientTCP)
		go transfer(clientTCP, destTCP)
	} else {
		go func() {
			var wg sync.WaitGroup
			wg.Add(2)
			go transferWg(&wg, dest, client)
			go transferWg(&wg, client, dest)
			wg.Wait()
			client.Close()
			dest.Close()
		}()
	}
}
