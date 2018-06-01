package main

import (
	"crypto/tls"
	"flag"
	"github.com/lamg/proxy"
	"log"
	"net"
	h "net/http"
	"time"
)

func main() {
	var addr, lrange string
	flag.StringVar(&addr, "a", ":8080", "Server address")
	flag.StringVar(&lrange, "r", "", "CIDR range for listening")
	flag.Parse()

	rgs := []string{lrange}
	iprgs := make([]*net.IPNet, len(rgs))
	var e error
	for i := 0; e == nil && i != len(rgs); i++ {
		_, iprgs[i], e = net.ParseCIDR(rgs[i])
	}
	if e == nil {
		prox := &proxy.Proxy{
			Tr: &h.Transport{},
		}
		np := &nProxy{
			proxy:   prox,
			ipRange: iprgs,
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

type nProxy struct {
	proxy   *proxy.Proxy
	ipRange []*net.IPNet
}

func (p *nProxy) ServeHTTP(w h.ResponseWriter, r *h.Request) {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e == nil {
		ni := net.ParseIP(host)
		ok := false
		for i := 0; !ok && i != len(p.ipRange); i++ {
			ok = p.ipRange[i].Contains(ni)
		}
		if ok {
			p.proxy.ServeHTTP(w, r)
		} else {
			h.Error(w, "Out of range", h.StatusBadRequest)
		}
	}
}
