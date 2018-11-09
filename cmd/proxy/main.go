package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	h "net/http"
	"time"

	"github.com/lamg/proxy"
)

func main() {
	var addr, lrange string
	flag.StringVar(&addr, "a", ":8080", "Server address")
	flag.StringVar(&lrange, "r", "", "CIDR range for listening")
	flag.Parse()

	rgs := []string{lrange}
	tr, e := newTransport(rgs)
	if e == nil {
		np := &proxy.Proxy{
			Rt:          tr,
			DialContext: dialContext,
			AddCtxValue: tr.addCtxValue,
		}
		server := &h.Server{
			Addr:         addr,
			Handler:      np,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			// Disable HTTP/2.
			TLSNextProto: make(map[string]func(*h.Server, *tls.Conn,
				h.Handler)),
		}
		e = server.ListenAndServe()
	}

	if e != nil {
		log.Fatal(e)
	}
}

type transport struct {
	h.RoundTripper
	iprgs []*net.IPNet
}

func newTransport(cidrs []string) (n *transport, e error) {
	iprgs := make([]*net.IPNet, len(cidrs))
	for i := 0; e == nil && i != len(cidrs); i++ {
		_, iprgs[i], e = net.ParseCIDR(cidrs[i])
	}
	if e == nil {
		tr := &h.Transport{
			DialContext:           dialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}
		n = &transport{
			RoundTripper: tr,
			iprgs:        iprgs,
		}
	}
	return
}

type ctxValKT string

var ctxValK = ctxValKT("ctxVal")

type ctxVal struct {
	err error
}

func (n *transport) addCtxValue(r *h.Request) (x *h.Request) {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e == nil {
		ni := net.ParseIP(host)
		if ni != nil {
			ok := false
			for i := 0; !ok && i != len(n.iprgs); i++ {
				ok = n.iprgs[i].Contains(ni)
			}
			if !ok {
				e = fmt.Errorf("Host %s out of range", host)
			}
		} else {
			e = fmt.Errorf("Error parsing host IP %s", host)
		}
	}
	ctx := r.Context()
	nctx := context.WithValue(ctx, ctxValK, &ctxVal{err: e})
	x = r.WithContext(nctx)
	return
}

func dialContext(ctx context.Context,
	network, addr string) (c net.Conn, e error) {
	cv, ok := ctx.Value(ctxValK).(*ctxVal)
	if !ok {
		e = fmt.Errorf("No error value with key %s", ctxValK)
	} else {
		e = cv.err
	}
	if e == nil {
		c, e = net.Dial(network, addr)
	}
	return
}
