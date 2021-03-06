// Copyright © 2018-2019 Luis Ángel Méndez Gort

// This file is part of Proxy.

// Proxy is free software: you can redistribute it and/or
// modify it under the terms of the GNU Lesser General
// Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your
// option) any later version.

// Proxy is distributed in the hope that it will be
// useful, but WITHOUT ANY WARRANTY; without even the
// implied warranty of MERCHANTABILITY or FITNESS FOR A
// PARTICULAR PURPOSE. See the GNU Lesser General Public
// License for more details.

// You should have received a copy of the GNU Lesser General
// Public License along with Proxy.  If not, see
// <https://www.gnu.org/licenses/>.

package proxy

import (
	"context"
	"io"
	"net"
	h "net/http"
	"sync"

	fh "github.com/valyala/fasthttp"
	gp "golang.org/x/net/proxy"
)

// NewFastProxy creates a
// github.com/valyala/fasthttp.RequestHandler ready to be
// used as an HTTP/HTTPS proxy server, in conjunction with
// a github.com/valyala/fasthttp.Server
func NewFastProxy(dial Dialer) (p *Proxy) {
	gp.RegisterDialerType("http", newHTTPProxy)
	p = &Proxy{
		dialContext: dial,
		fastCl: &fh.Client{
			DialDualStack: true,
		},
	}
	return
}

func (p *Proxy) RequestHandler(ctx *fh.RequestCtx) {
	i := &ReqParams{
		Method: string(ctx.Request.Header.Method()),
		URL:    string(ctx.URI().Host()),
	}
	raddr := ctx.RemoteAddr().String()
	i.IP, _, _ = net.SplitHostPort(raddr)
	nctx := context.WithValue(ctx, ReqParamsK, i)
	p.fastCl.Dial = func(addr string) (c net.Conn, e error) {
		c, e = p.dialContext(nctx, "tcp", addr)
		return
	}
	if ctx.IsConnect() {
		dest, e := p.fastCl.Dial(i.URL)
		if e == nil {
			ctx.SetStatusCode(h.StatusOK)
			ctx.Hijack(func(client net.Conn) {
				dTCP, dok := dest.(*net.TCPConn)
				cTCP, cok := client.(*net.TCPConn)
				if dok && cok {
					transWait(dTCP, cTCP)
				} else {
					transWait(dest, client)
				}
			})
		} else {
			if i.URL == "" {
				ctx.Response.SetStatusCode(h.StatusBadRequest)
			} else {
				ctx.Response.SetStatusCode(h.StatusServiceUnavailable)
			}
		}
	} else {
		copyFastHd(&ctx.Response.Header, &ctx.Request.Header)
		p.fastCl.Do(&ctx.Request, &ctx.Response)
	}
}

func transWait(dest, src io.ReadWriteCloser) {
	var wg sync.WaitGroup
	wg.Add(2)
	go transferWg(&wg, dest, src)
	go transferWg(&wg, src, dest)
	wg.Wait()
	dest.Close()
	src.Close()
}

func copyFastHd(resp *fh.ResponseHeader,
	req *fh.RequestHeader) {
	req.VisitAll(func(k, v []byte) {
		ks := string(k)
		ok := searchHopByHop(ks)
		if !ok {
			resp.Add(ks, string(v))
		}
	})
}
