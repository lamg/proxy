# Proxy

[![GoDoc][0]][1] [![Go Report Card][2]][3] [![Build Status][4]][5] [![Coverage Status][6]][7]

HTTP/HTTPS proxy library with custom dialer, which receives in the context key `proxy.ReqParamsK` a `*proxy.ReqParams` instance with the IP that made the request, it's URL and method. It can be served using [standard library server][8] or [fasthttp server][9], and has two builtin dialers for dialing with a specific network interface or parent proxy.

## Install example server

With Go 1.13 or superior:

```sh
git clone git@github.com:lamg/proxy.git
cd proxy/cmd/proxy && go install
```

## Example

This is a proxy that denies all the connections coming from IP addresses different from `127.0.0.1`.

```go
package main

import (
	"crypto/tls"
	"fmt"
	"net"
	h "net/http"
	"time"

	alg "github.com/lamg/algorithms"
	"github.com/lamg/proxy"
)

func main() {
	// localhost clients only
	ar, e := newAllowedRanges("127.0.0.1/32")
	if e == nil {
		p := proxy.NewProxy(ar.DialContext)
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

type allowedRanges struct {
	ranges      []*net.IPNet
	timeout     time.Duration
}

func newAllowedRanges(cidrs ...string) (a *allowedRanges, e error) {
	a = &allowedRanges{
		ranges:      make([]*net.IPNet, len(cidrs)),
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
		c, e = ifd.Dial(network, addr)
	}
	return
}
```

[0]: https://godoc.org/github.com/lamg/proxy?status.svg
[1]: https://godoc.org/github.com/lamg/proxy

[2]: https://goreportcard.com/badge/github.com/lamg/proxy
[3]: https://goreportcard.com/report/github.com/lamg/proxy

[4]: https://travis-ci.com/lamg/proxy.svg?branch=master
[5]: https://travis-ci.com/lamg/proxy

[6]: https://coveralls.io/repos/github/lamg/proxy/badge.svg?branch=master&service=github
[7]: https://coveralls.io/github/lamg/proxy?branch=master

[8]: https://godoc.org/net/http#Server
[9]: https://godoc.org/github.com/valyala/fasthttp#Server
