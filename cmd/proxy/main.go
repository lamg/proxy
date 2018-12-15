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
	gp "golang.org/x/net/proxy"
)

func main() {
	var addr, lrange, socks string
	flag.StringVar(&addr, "a", ":8080", "Server address")
	flag.StringVar(&lrange, "r", "127.0.0.1/32", "CIDR range for listening")
	flag.StringVar(&socks, "s", "", "SOCKS5 proxy address")
	flag.Parse()

	var dl gp.Dialer
	if socks != "" {
		var er error
		dl, er = gp.SOCKS5("tcp", socks, nil, gp.Direct)
		if er != nil {
			dl = gp.Direct
			log.Println(er.Error())
		}
	}

	rgs := []string{lrange}
	ctxDl := newCtxDialer(dl)
	tr, e := newTransport(rgs, ctxDl)
	if e == nil {
		np := &proxy.Proxy{
			Rt:          tr,
			DialContext: ctxDl,
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

func newTransport(cidrs []string, cd ctxDialer) (n *transport,
	e error) {
	iprgs := make([]*net.IPNet, len(cidrs))
	for i := 0; e == nil && i != len(cidrs); i++ {
		_, iprgs[i], e = net.ParseCIDR(cidrs[i])
	}
	if e == nil {
		tr := &h.Transport{
			DialContext:           cd,
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

type ctxDialer func(context.Context, string,
	string) (net.Conn, error)

func newCtxDialer(d gp.Dialer) (x ctxDialer) {
	x = func(ctx context.Context, network, addr string) (c net.Conn, e error) {
		cv, ok := ctx.Value(ctxValK).(*ctxVal)
		if !ok {
			e = fmt.Errorf("No error value with key %s", ctxValK)
		} else {
			e = cv.err
		}
		if e == nil {
			c, e = d.Dial(network, addr)
		}
		return
	}
	return
}
