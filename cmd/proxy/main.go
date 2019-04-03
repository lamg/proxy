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

// proxy is an usage example of github.com/lamg/proxy
// library
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"

	"log"
	"net"
	h "net/http"
	"net/url"
	"time"

	fh "github.com/valyala/fasthttp"

	"github.com/lamg/proxy"
)

func main() {
	var addr, lrange, proxyURL string
	var fastH bool
	flag.StringVar(&addr, "a", ":8080", "Server address")
	flag.StringVar(&lrange, "r", "127.0.0.1/32",
		"CIDR range for listening")
	flag.StringVar(&proxyURL, "p", "", "Proxy address")
	flag.BoolVar(&fastH, "f", false,
		"Use github.com/valyala/fasthttp")
	flag.Parse()

	var e error
	var u *url.URL
	if proxyURL != "" {
		u, e = url.Parse(proxyURL)
		if !(u.Scheme == "http" || u.Scheme == "socks5") {
			e = fmt.Errorf("Not recognized URL scheme '%s', "+
				"must be 'http' or 'socks5'", u.Scheme)
		}
	}
	var ctxV *rangeIPCtx
	if e == nil {
		rgs := []string{lrange}
		ctxV, e = newRangeIPCtx(rgs, u)
	}
	if e == nil {
		apply := func(n net.Conn, mods []string) (c net.Conn) {
			c = n
			return
		}
		if fastH {
			np := proxy.NewFastProxy(ctxV.setContext, params, apply,
				90*time.Second, time.Now)
			e = fh.ListenAndServe(addr, np)
		} else {
			maxIdleConns := 100
			idleConnTimeout := 90 * time.Second
			tlsHandshakeTimeout := 10 * time.Second
			expectContinueTimeout := time.Second

			np := proxy.NewProxy(
				ctxV.setContext,
				params,
				apply,
				idleConnTimeout,
				maxIdleConns,
				idleConnTimeout,
				tlsHandshakeTimeout,
				expectContinueTimeout,
				time.Now,
			)
			e = standardSrv(np, addr)
		}
	}
	if e != nil {
		log.Fatal(e)
	}
}

func standardSrv(hn h.Handler, addr string) (e error) {
	server := &h.Server{
		Addr:         addr,
		Handler:      hn,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		// Disable HTTP/2.
		TLSNextProto: make(map[string]func(*h.Server,
			*tls.Conn, h.Handler)),
	}
	e = server.ListenAndServe()
	return
}

type rangeIPCtx struct {
	parentProxy *url.URL
	iprgs       []*net.IPNet
}

func newRangeIPCtx(cidrs []string,
	parentProxy *url.URL) (n *rangeIPCtx, e error) {
	iprgs := make([]*net.IPNet, len(cidrs))
	ib := func(i int) (b bool) {
		var e error
		_, iprgs[i], e = net.ParseCIDR(cidrs[i])
		b = e != nil
		return
	}
	bLnSrch(ib, len(cidrs))
	if e == nil {
		n = &rangeIPCtx{
			iprgs:       iprgs,
			parentProxy: parentProxy,
		}
	}
	return
}

type keyT string

var key = keyT("error")

func (n *rangeIPCtx) setContext(ctx context.Context,
	method, url, rAddr string,
	t time.Time) (nctx context.Context) {
	host, _, e := net.SplitHostPort(rAddr)
	if e == nil {
		ni := net.ParseIP(host)
		if ni != nil {
			ib := func(i int) (b bool) {
				b = n.iprgs[i].Contains(ni)
				return
			}
			ok, _ := bLnSrch(ib, len(n.iprgs))
			if !ok {
				e = fmt.Errorf("Host %s out of range", host)
			}
		} else {
			e = fmt.Errorf("Error parsing host IP %s", host)
		}
	}
	nctx = context.WithValue(ctx, key,
		&proxy.ConnParams{ParentProxy: n.parentProxy, Error: e})
	return
}

func params(ctx context.Context) (p *proxy.ConnParams) {
	p = ctx.Value(key).(*proxy.ConnParams)
	return
}

type intBool func(int) bool

// bLnSrch is the bounded lineal search algorithm
// { n ≥ 0 ∧ ⟨∀i: 0 ≤ i < n: def.(ib.i)⟩ }
// { i = ⟨↑j: 0 ≤ j ≤ n ∧ ⟨∀k: 0 ≤ k < j: ¬ib.k⟩: j⟩ ∧
//   b ≡ i ≠ n }
func bLnSrch(ib intBool, n int) (b bool, i int) {
	b, i = false, 0
	for !b && i != n {
		b = ib(i)
		if !b {
			i = i + 1
		}
	}
	return
}
