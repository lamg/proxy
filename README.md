# Proxy

[![GoDoc](https://godoc.org/github.com/lamg/proxy?status.svg)](https://godoc.org/github.com/lamg/proxy)

HTTP proxy that uses custom procedures for network dialing and parent proxy selection (HTTP or SOCKS5). It can be served using [standard library server](https://godoc.org/net/http#Server) or [fasthttp server](https://godoc.org/github.com/valyala/fasthttp#Server)

## Usage

The command line program at [cmd/proxy](cmd/proxy) is a simple example of how to use the library. With Go 1.11 or superior install with:

```sh
git clone git@github.com:lamg/proxy.git
cd proxy/cmd/proxy && go install
```