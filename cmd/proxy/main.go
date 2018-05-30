package main

import (
	"flag"
	"github.com/lamg/proxy"
	"log"
	h "net/http"
)

func main() {
	var addr string
	flag.StringVar(&addr, "a", ":8080", "Server address")
	flag.Parse()
	proxy := new(proxy.Proxy)
	server := &h.Server{
		Addr:    addr,
		Handler: proxy,
		// Disable HTTP/2.
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn,
			http.Handler)),
	}

	log.Fatal(server.ListenAndServe())
}
