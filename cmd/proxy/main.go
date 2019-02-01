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

	"github.com/lamg/proxy"
)

func main() {
	var addr, lrange, proxyURL string
	var fastH, elazarl bool
	flag.StringVar(&addr, "a", ":8080", "Server address")
	flag.StringVar(&lrange, "r", "127.0.0.1/32",
		"CIDR range for listening")
	flag.StringVar(&proxyURL, "p", "", "Proxy address")
	flag.BoolVar(&fastH, "f", false,
		"Use github.com/valyala/fasthttp")
	flag.BoolVar(&elazarl, "e", false,
		"Use github.com/elazarl/goproxy")
	flag.Parse()

	var e error
	var u *url.URL
	if proxyURL != "" {
		u, e = url.Parse(proxyURL)
	}
	var ctxV *rangeIPCtx
	if e == nil {
		rgs := []string{lrange}
		ctxV, e = newRangeIPCtx(rgs)
	}
	var np *proxy.Proxy
	if e == nil {
		maxIdleConns := 100
		idleConnTimeout := 90 * time.Second
		tlsHandshakeTimeout := 10 * time.Second
		expectContinueTimeout := time.Second

		np, e = proxy.NewProxy(
			dialContext,
			ctxV.setVal,
			maxIdleConns,
			idleConnTimeout,
			tlsHandshakeTimeout,
			expectContinueTimeout,
			time.Now,
			func(r *h.Request) (*url.URL, error) { return u, nil },
		)
	}
	if e == nil {
		e = standardSrv(np, addr)
	}
	if e != nil {
		log.Fatal(e)
	}
}

func standardSrv(p *proxy.Proxy, addr string) (e error) {
	server := &h.Server{
		Addr:         addr,
		Handler:      p,
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
	iprgs []*net.IPNet
}

func newRangeIPCtx(cidrs []string) (n *rangeIPCtx,
	e error) {
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
			iprgs: iprgs,
		}
	}
	return
}

type keyT string

var key = keyT("error")

type errV struct {
	isNil bool
	e     error
}

func (n *rangeIPCtx) setVal(ctx context.Context,
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
		&errV{isNil: e == nil, e: e})
	return
}

func dialContext(ctx context.Context, network,
	addr string) (c net.Conn, e error) {
	err, ok := ctx.Value(key).(*errV)
	if !ok {
		e = fmt.Errorf("No error value with key %s", key)
	} else {
		e = err.e
	}
	if e == nil {
		c, e = net.Dial(network, addr)
	}
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
