# Proxy

[![GoDoc][0]][1] [![Go Report Card][2]][3]

HTTP proxy that uses custom procedures for network dialing and parent proxy selection (HTTP or SOCKS5). It can be served using [standard library server][4] or [fasthttp server][5]

## Usage

The command line program at [cmd/proxy](cmd/proxy) is a simple example of how to use the library. With Go 1.11 or superior install with:

```sh
git clone git@github.com:lamg/proxy.git
cd proxy/cmd/proxy && go install
```

[0]: https://godoc.org/github.com/lamg/proxy?status.svg
[1]: https://godoc.org/github.com/lamg/proxy

[2]: https://goreportcard.com/badge/github.com/lamg/proxy
[3]: https://goreportcard.com/report/github.com/lamg/proxy

[4]: https://godoc.org/net/http#Server
[5]: https://godoc.org/github.com/valyala/fasthttp#Server
