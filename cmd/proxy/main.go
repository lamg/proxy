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

	alg "github.com/lamg/algorithms"
	"github.com/lamg/proxy"
)

func main() {
	var addr, lrange, proxyURL string
	var fastH bool
	flag.StringVar(&addr, "a", ":8080", "Server address")
	flag.StringVar(&lrange, "r", "127.0.0.1/32",
		"CIDR range for listening")
	flag.StringVar(&proxyURL, "p", "", "Parent proxy address")
	flag.BoolVar(&fastH, "f", false,
		"Use github.com/valyala/fasthttp")
	flag.Parse()

	var e error
	var parentProxy *url.URL
	if proxyURL != "" {
		parentProxy, e = url.Parse(proxyURL)
		if !(parentProxy.Scheme == "http" ||
			parentProxy.Scheme == "socks5") {
			e = fmt.Errorf("Not recognized URL scheme '%s', "+
				"must be 'http' or 'socks5'", parentProxy.Scheme)
		}
	}
	ar, e := newAllowedRanges(parentProxy, lrange)
	if e == nil {
		if fastH {
			np := proxy.NewFastProxy(ar.DialContext)
			e = fh.ListenAndServe(addr, np.RequestHandler)
		} else {
			np := proxy.NewProxy(ar.DialContext)
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
		TLSNextProto: make(map[string]func(*h.Server, *tls.Conn,
			h.Handler)),
	}
	e = server.ListenAndServe()
	return
}

type allowedRanges struct {
	ranges      []*net.IPNet
	parentProxy *url.URL
	timeout     time.Duration
}

func newAllowedRanges(parentProxy *url.URL,
	cidrs ...string) (a *allowedRanges, e error) {
	a = &allowedRanges{
		ranges:      make([]*net.IPNet, len(cidrs)),
		parentProxy: parentProxy,
		timeout:     90 * time.Second,
	}
	ib := func(i int) (b bool) {
		_, a.ranges[i], e = net.ParseCIDR(cidrs[i])
		b = e != nil
		return
	}
	alg.BLnSrch(ib, len(cidrs))
	return
}

func (r *allowedRanges) DialContext(ctx context.Context, network,
	addr string) (c net.Conn, e error) {
	rqp := ctx.Value(proxy.ReqParamsK).(*proxy.ReqParams)
	ip := net.ParseIP(rqp.IP)
	ok, _ := alg.BLnSrch(
		func(i int) bool { return r.ranges[i].Contains(ip) },
		len(r.ranges),
	)
	if !ok {
		e = fmt.Errorf("Client IP '%s' out of range", rqp.IP)
	}
	if e == nil {
		ifd := &proxy.IfaceDialer{Timeout: r.timeout}
		if r.parentProxy != nil {
			c, e = proxy.DialProxy(network, addr, r.parentProxy, ifd)
		} else {
			c, e = ifd.Dial(network, addr)
		}
	}
	return
}
