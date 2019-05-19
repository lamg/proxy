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
	var rip proxy.IfaceParentProxy
	if e == nil {
		cidrs := []string{lrange}
		rip, e = restrIPRange(cidrs, u)
	}
	if e == nil {
		if fastH {
			np := proxy.NewFastProxy(rip, proxy.UnrestrictedConn,
				90*time.Second, time.Now)
			e = fh.ListenAndServe(addr, np)
		} else {
			maxIdleConns := 100
			idleConnTimeout := 90 * time.Second
			tlsHandshakeTimeout := 10 * time.Second
			expectContinueTimeout := time.Second

			np := proxy.NewProxy(
				rip,
				proxy.UnrestrictedConn,
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

func restrIPRange(cidrs []string,
	prx *url.URL) (f proxy.IfaceParentProxy,
	e error) {
	iprgs := make([]*net.IPNet, len(cidrs))
	ib := func(i int) (b bool) {
		_, iprgs[i], e = net.ParseCIDR(cidrs[i])
		b = e != nil
		return
	}
	alg.BLnSrch(ib, len(cidrs))
	if e == nil {
		f = func(meth, ürl, ip string, t time.Time) (iface string,
			p *url.URL, d error) {
			ni := net.ParseIP(ip)
			if ni != nil {
				ib := func(i int) bool { return iprgs[i].Contains(ni) }
				ok, _ := alg.BLnSrch(ib, len(iprgs))
				if !ok {
					d = fmt.Errorf("Client IP '%s' out of range", ip)
				}
			} else {
				d = fmt.Errorf("Error parsing client IP '%s'", ip)
			}
			p = prx
			return
		}
	}
	return
}
