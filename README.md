# Proxy

[![GoDoc][0]][1] [![Go Report Card][2]][3] [![Build Status][7]][8]

HTTP/HTTPS proxy library that dials connections using the network interface and parent proxy (HTTP or SOCKS5) determined by a custom procedure, having request method, URL, remote address and time as parameters. The dialed connection's operation is controlled by that procedure. It can be served using [standard library server][4] or [fasthttp server][5]

## Install

With Go 1.11 or superior:

```sh
git clone git@github.com:lamg/proxy.git
cd proxy/cmd/proxy && go install
```

## Usage

The library uses the custom procedure [ConnControl][6], for determining the network interface and parent proxy for making the connection, and for controlling the established connection's behavior.
 
## Example

This is a proxy that denies all the connections coming from IP addresses outside a given range.

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
	rip, e := restrictedIPRange([]string{"127.0.0.1/32"}) // localhost clients only
	if e == nil {
		timeout := 30 * time.Second
		p := proxy.NewProxy(rip, 10*time.Second, time.Now)
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

func restrictedIPRange(cidrs []string) (f proxy.ConnControl,
	e error) {
	iprgs := make([]*net.IPNet, len(cidrs))
	ib := func(i int) (b bool) {
		_, iprgs[i], e = net.ParseCIDR(cidrs[i])
		b = e != nil
		return
	}
	alg.BLnSrch(ib, len(cidrs))
	if e == nil {
		f = func(op *proxy.Operation) (r *proxy.Result) {
			r = new(proxy.Result)
			// if proxy.Open command is sent,
			// a *proxy.Result.Error != nil means the connection will
			// not be established, and since the rest of the operations
			// are performed if this is successful, there's no need to
			// check them in this case where the connection's behavior
			// is not controlled after it's established
			if op.Command == proxy.Open {
				ni := net.ParseIP(op.IP)
				if ni != nil {
					ib := func(i int) bool { return iprgs[i].Contains(ni) }
					ok, _ := alg.BLnSrch(ib, len(iprgs))
					if !ok {
						r.Error = fmt.Errorf("Client IP '%s' out of range",
							op.IP)
					}
				} else {
					r.Error = fmt.Errorf("Error parsing client IP '%s'",
						op.IP)
				}
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

[6]: https://godoc.org/github.com/lamg/proxy#ConnControl

[7]: https://travis-ci.com/lamg/proxy.svg?branch=master
[8]: https://travis-ci.com/lamg/proxy
