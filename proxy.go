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
	"fmt"
	"io"
	"net"
	h "net/http"
	"net/url"
	"sync"
	"time"

	fh "github.com/valyala/fasthttp"
	gp "golang.org/x/net/proxy"
)

type proxyS struct {
	clock      func() time.Time
	setContext Modify
	params     Extract
	apply      Wrapper
	timeout    time.Duration

	trans  *h.Transport
	fastCl *fh.Client
}

// Modify is the signature of the function that sets the context value
// according request method, request url, request remote address
// and time.
type Modify func(context.Context, string, string, string,
	time.Time) context.Context

// Extract is the signature of the function that extracts the
// *ConnParams value from the context set by the function with
// the signature of Modify
type Extract func(context.Context) *ConnParams

// Wrapper is the signature of the function for wrapping a connection
// given the client IP and a slice of modifiers to be applied
type Wrapper func(net.Conn, string, []string) (net.Conn, error)

// NewProxy creates a net/http.Handler ready to be used
// as an HTTP/HTTPS proxy server in conjunction with
// a net/http.Server
func NewProxy(
	setCtx Modify,
	params Extract,
	apply Wrapper,
	dialTimeout time.Duration,
	maxIdleConns int,
	idleConnTimeout,
	tlsHandshakeTimeout,
	expectContinueTimeout time.Duration,
	clock func() time.Time,
) (p h.Handler) {
	prx := &proxyS{
		clock:      clock,
		setContext: setCtx,
		params:     params,
		apply:      apply,
		timeout:    dialTimeout,
		trans: &h.Transport{
			MaxIdleConns:          maxIdleConns,
			IdleConnTimeout:       idleConnTimeout,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			ExpectContinueTimeout: expectContinueTimeout,
		},
	}
	prx.trans.DialContext = prx.dialContext
	gp.RegisterDialerType("http", newHTTPProxy)
	p = prx
	return
}

func (p *proxyS) ServeHTTP(w h.ResponseWriter,
	r *h.Request) {
	t := p.clock()
	c0 := p.setContext(r.Context(), r.Method, r.URL.String(),
		r.RemoteAddr, t)
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	c1 := context.WithValue(c0, ipK, ip)
	nr := r.WithContext(c1)
	if r.Method == h.MethodConnect {
		p.handleTunneling(w, nr)
	} else {
		p.handleHTTP(w, nr)
	}
}

var (
	clientConnectOK = []byte("HTTP/1.0 200 OK\r\n\r\n")
)

func (p *proxyS) handleTunneling(w h.ResponseWriter,
	r *h.Request) {
	destConn, e := p.dialContext(r.Context(), "tcp", r.Host)

	var hijacker h.Hijacker
	status := h.StatusOK
	if e == nil {
		var ok bool
		w.WriteHeader(status)
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

func (p *proxyS) handleHTTP(w h.ResponseWriter,
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

// ConnParams is a value determined by the functions with Modify
// and Extract signatures. Is used for dialing connections and
// for wrapping the dialed connection, using the function with
// Wrapper signature, with ConnParams.Modifiers. These are
// implementations of the net.Conn interface that could make
// slower the connection or limit the amount of data available to
// download.
type ConnParams struct {
	Iface       string
	ParentProxy *url.URL
	Modifiers   []string
	Error       error
}

type ipKT string

var ipK = ipKT("ip")

func (p *proxyS) dialContext(ctx context.Context, network,
	addr string) (c net.Conn, e error) {
	cp := p.params(ctx)
	clientIP := ctx.Value(ipK).(string)
	if cp != nil && cp.Error == nil {
		dlr := &net.Dialer{
			Timeout: p.timeout,
		}
		if cp.Iface != "" {
			var nf *net.Interface
			nf, e = net.InterfaceByName(cp.Iface)
			var laddr []net.Addr
			if e == nil {
				laddr, e = nf.Addrs()
			}
			if len(laddr) != 0 {
				dlr.LocalAddr = &net.TCPAddr{IP: laddr[0].(*net.IPNet).IP}
			} else {
				e = fmt.Errorf("No local IP")
			}
		}
		var n net.Conn
		if cp.ParentProxy != nil {
			var d gp.Dialer
			d, e = gp.FromURL(cp.ParentProxy, dlr)
			if e == nil {
				n, e = d.Dial(network, addr)
			}
		} else {
			n, e = dlr.Dial(network, addr)
		}
		if e == nil {
			c, e = p.apply(n, clientIP, cp.Modifiers)
		}
	} else if cp != nil && cp.Error != nil {
		e = cp.Error
	} else {
		e = fmt.Errorf("No connection parameters in context")
	}
	return
}

func copyConns(dest, client net.Conn) {
	// learning from https://github.com/elazarl/goproxy
	// /blob/2ce16c963a8ac5bd6af851d4877e38701346983f
	// /https.go#L103
	client.Write(clientConnectOK)
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

func transferWg(wg *sync.WaitGroup,
	dest io.Writer, src io.Reader) {
	io.Copy(dest, src)
	wg.Done()
}

func transfer(dest io.WriteCloser, src io.ReadCloser) {
	io.Copy(dest, src)
	dest.Close()
	src.Close()
}

func searchHopByHop(hd string) (ok bool) {
	// hop-by-hop headers. Shouldn't be sent to the
	// requested host.
	// https://developer.mozilla.org/en-US/docs/
	// Web/HTTP/Headers#hbh
	hbh := []string{
		"Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "TE", "Trailer",
		"Transfer-Encoding", "Upgrade",
	}
	ib := func(i int) (b bool) {
		b = hbh[i] == hd
		return
	}
	ok, _ = bLnSrch(ib, len(hbh))
	return
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

// noHijacking error
func noHijacking() (e error) {
	e = fmt.Errorf("No hijacking supported")
	return
}

type intBool func(int) bool

// bLnSrch is the bounded lineal search algorithm
// { n ≥ 0 ∧ forall.n.(def.ib) }
// { i =⟨↑j: 0 ≤ j ≤ n ∧ ⟨∀k: 0 ≤ k < j: ¬ib.k⟩: j⟩
//   ∧ b ≡ i ≠ n }
func bLnSrch(ib intBool, n int) (b bool, i int) {
	b, i, udb := false, 0, true
	// udb: undefined b for i
	for !b && i != n {
		if udb {
			b, udb = ib(i), false
		} else {
			i, udb = i+1, true
		}
	}
	return
}
