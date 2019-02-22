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
	"log"
	"net"
	h "net/http"
	"net/url"
	"sync"
	"time"

	fh "github.com/valyala/fasthttp"
	gp "golang.org/x/net/proxy"
)

type proxyS struct {
	clock     func() time.Time
	trans     *h.Transport
	direct    ContextDialer
	connectDl Dialer
	contextV  ContextValueF
	parent    ParentProxyF

	fastCl *fh.Client
}

// ContextValueF is the signature of a procedure
// that calculates a value and embeds it in the
// returned context. This context is passed to the
// supplied ContextDialer.
type ContextValueF func(
	context.Context,
	string, // HTTP method
	string, // URL
	string, // Remote address
	time.Time,
) context.Context

// ContextDialer is the signature of a procedure
// that creates network connections possibly taking
// a value created by ContextValueF
type ContextDialer func(context.Context, string,
	string) (net.Conn, error)

// Dialer is the procedure signature of net.Dial
type Dialer func(string, string) (net.Conn, error)

// ParentProxyF is the signature of a procedure that
// calculates the parent HTTP proxy for processing a
// request
type ParentProxyF func(string, string, string,
	time.Time) (*url.URL, error)

type dlWrap struct {
	ctx func() context.Context
	dl  ContextDialer
}

func (d *dlWrap) Dial(nt, addr string) (c net.Conn,
	e error) {
	c, e = d.dl(d.ctx(), nt, addr)
	return
}

// NewProxy creates a net/http.Handler ready to be used
// as an HTTP/HTTPS proxy server in conjunction with
// a net/http.Server
func NewProxy(
	direct ContextDialer,
	v ContextValueF,
	prxF ParentProxyF,
	maxIdleConns int,
	idleConnTimeout,
	tlsHandshakeTimeout,
	expectContinueTimeout time.Duration,
	clock func() time.Time,
) (p h.Handler) {
	p = &proxyS{
		clock:    clock,
		contextV: v,
		direct:   direct,
		parent:   prxF,
		trans: &h.Transport{
			Proxy: func(r *h.Request) (u *url.URL,
				e error) {
				u, e = prxF(r.Method, r.URL.String(),
					r.RemoteAddr, clock())
				return
			},
			DialContext:           direct,
			MaxIdleConns:          maxIdleConns,
			IdleConnTimeout:       idleConnTimeout,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			ExpectContinueTimeout: expectContinueTimeout,
		},
	}
	gp.RegisterDialerType("http", newHTTPProxy)
	return
}

func (p *proxyS) ServeHTTP(w h.ResponseWriter,
	r *h.Request) {
	p.setStdDl(r.Context(), r.Method, r.URL.String(),
		r.RemoteAddr)
	if r.Method == h.MethodConnect {
		p.handleTunneling(w, r)
	} else {
		p.handleHTTP(w, r)
	}
}

var (
	clientConnectOK = []byte("HTTP/1.0 200 OK\r\n\r\n")
)

func (p *proxyS) handleTunneling(w h.ResponseWriter,
	r *h.Request) {
	destConn, e := p.connectDl("tcp", r.Host)

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

func (p *proxyS) setStdDl(c context.Context,
	meth, ürl, rAddr string) {
	t := p.clock()
	ctx := p.contextV(c, meth, ürl, rAddr, t)
	prxURL, e := p.parent(meth, ürl, rAddr, t)
	if e != nil {
		log.Print(e.Error())
	}
	wr := &dlWrap{
		ctx: func() context.Context { return ctx },
		dl:  p.direct,
	}
	p.connectDl = wr.Dial
	if prxURL != nil {
		prxDl, e := gp.FromURL(prxURL, wr)
		if e == nil {
			p.connectDl = prxDl.Dial
		} else {
			log.Print(e.Error())
		}
	}
	// p.connectDl correctly set if there's a proxy
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
