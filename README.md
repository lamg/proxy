# Proxy

[![GoDoc][0]][1] [![Go Report Card][2]][3]

HTTP/HTTPS proxy library that dials connections using the network interface and parent proxy (HTTP or SOCKS5) determined by a custom procedure, having request method, URL, remote address and time as parameters. The dialed connection is controlled by another custom procedure. It can be served using [standard library server][4] or [fasthttp server][5]

## Install

With Go 1.11 or superior:

```sh
git clone git@github.com:lamg/proxy.git
cd proxy/cmd/proxy && go install
```

## Usage

The library uses custom procedures for determining the network interface and parent proxy for making the connection, and for controlling the established connection's behavior. These are [IfaceParentProxy][6] and [ControlConn][7] respectively.

## Example

This is a proxy that denies all the connections coming from IP addresses outside a given range.

```go
package main

import (
	"crypto/tls"
	"fmt"
	"net"
	h "net/http"
	"net/url"
	"time"

	alg "github.com/lamg/algorithms"
	"github.com/lamg/proxy"
)

func main() {
	rip, e := restrictedIPRange([]string{"127.0.0.1/32"}) // localhost clients only
	if e == nil {
		timeout := 30 * time.Second
		p := proxy.NewProxy(rip, proxy.UnrestrictedConn,
			timeout,
			100,
			timeout,
			timeout,
			timeout,
			time.Now,
		)
		server := &h.Server{
			Addr:         ":8080",
			Handler:      p,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			// Disable HTTP/2.
			TLSNextProto: make(map[string]func(*h.Server,
				*tls.Conn, h.Handler)),
		}
		e = server.ListenAndServe()
	}
	if e != nil {
		log.Fatal(e)
	}
}

func restrictedIPRange(cidrs []string) (f proxy.IfaceParentProxy,
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
			return
		}
	}
	return
}
```

[0]: https://godoc.org/github.com/lamg/proxy?status.svg
[1]: https://godoc.org/github.com/lamg/proxy

[2]: https://goreportcard.com/badge/github.com/lamg/proxy
[3]: https://goreportcard.com/report/github.com/lamg/proxy

[4]: https://godoc.org/net/http#Server
[5]: https://godoc.org/github.com/valyala/fasthttp#Server

[6]: https://godoc.org/github.com/lamg/proxy#IfaceParentProxy
[7]: https://godoc.org/github.com/lamg/proxy#ControlConn
