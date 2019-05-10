# Proxy

[![GoDoc][0]][1] [![Go Report Card][2]][3]

HTTP proxy that dials connections according parameters determined by request method, URL, remote address and time. These parameters include parent proxy (HTTP or SOCKS5), and network interface. It can be served using [standard library server][4] or [fasthttp server][5]

## Install

With Go 1.11 or superior:

```sh
git clone git@github.com:lamg/proxy.git
cd proxy/cmd/proxy && go install
```

## Usage

The library provides great flexibility for specifying the parameters for making connections. The [Modify][6] function signature is meant for a function that will set a value in the `context.Context` of the incoming `net/http.Request`, according it's method, URL, remote address and time it arrived. This context is processed by the [Extract][7] function to get the `proxy.ConnParams` value. The latter is used for dialing all the connections, it specifies the interface for dialing, optionally a parent proxy, a slice of names of connection modifiers to be used by [Wrapper][8]. The connection modifiers are implementations of the `net.Conn` interface that may delay the connection or limit the amount of data available for downloading.

## Example

This is a proxy that denies all the connections coming from IP addresses outside a given range.

```go
type keyT string
var key = keyT("ok")

func params (ctx context.Context) (p *proxy.ConnParams){
	p = ctx.Value(key).(*proxy.ConnParams)
	return
}
```

```go
rg, _ := net.ParseCIDR("192.168.1.0/24")
modCtx := func(c context.Context, method, url, remoteAddr string, 
		t time.Time) (nc context.Context){
	host, _, _ := net.SplitHostPort(remoteAddr)
	ip := net.ParseIP(host)
	params := &proxy.ConnParams{Iface:"eth0"}
	if !rg.Contains(ip) {
		params.Error = fmt.Errorf("Not allowed from %s", host)
	}
	nc = context.WithValue(c, key, params)
	return
}

apply := func(n net.Conn, clientIP string, mods []string) (c net.Conn, e error) {
	c = n
	return
}

dr := 10*time.Second
np := proxy.NewProxy(modCtx, params, apply, dr, 100, dr, dr, dr, 
	time.Now)
server := server := &h.Server{
		Addr:         ":8080",
		Handler:      np,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		// Disable HTTP/2.
		TLSNextProto: make(map[string]func(*h.Server,
			*tls.Conn, h.Handler)),
}
server.ListenAndServe()
```

[0]: https://godoc.org/github.com/lamg/proxy?status.svg
[1]: https://godoc.org/github.com/lamg/proxy

[2]: https://goreportcard.com/badge/github.com/lamg/proxy
[3]: https://goreportcard.com/report/github.com/lamg/proxy

[4]: https://godoc.org/net/http#Server
[5]: https://godoc.org/github.com/valyala/fasthttp#Server

[6]: https://godoc.org/github.com/lamg/proxy#Modify
[7]: https://godoc.org/github.com/lamg/proxy#Extract
[8]: https://godoc.org/github.com/lamg/proxy#Wrapper 
