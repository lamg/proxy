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
	"log"
	"net"
	h "net/http"
	"sync"
	"time"

	fh "github.com/valyala/fasthttp"
	gp "golang.org/x/net/proxy"
)

// NewFastProxy creates a
// github.com/valyala/fasthttp.RequestHandler ready to be
// used as an HTTP/HTTPS proxy server, in conjunction with
// a github.com/valyala/fasthttp.Server
func NewFastProxy(direct ContextDialer,
	v ContextValueF, prxF ParentProxyF,
	clock func() time.Time,
) (hn fh.RequestHandler) {
	gp.RegisterDialerType("http", newHTTPProxy)
	p := &proxyS{
		direct:   direct,
		contextV: v,
		fastCl: &fh.Client{
			DialDualStack: true,
		},
		clock:  clock,
		parent: prxF,
	}
	hn = p.fastHandler
	return
}

func (p *proxyS) fastHandler(ctx *fh.RequestCtx) {
	p.setFastDl(
		ctx,
		string(ctx.Request.Header.Method()),
		ctx.URI().String(),
		ctx.RemoteAddr().String(),
	)
	if ctx.IsConnect() {
		dest, e := p.fastCl.Dial(string(ctx.Host()))
		if e == nil {
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
			ctx.Response.SetStatusCode(h.StatusServiceUnavailable)
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

func (p *proxyS) setFastDl(ctx context.Context, method, ürl,
	rAddr string) {
	t := p.clock()
	nctx := p.contextV(ctx, method, ürl, rAddr, t)
	wr := &dlWrap{
		ctx: func() context.Context { return nctx },
		dl:  p.direct,
	}
	prxURL, e := p.parent(method, ürl, rAddr, t)
	if e != nil {
		log.Print(e.Error())
	}
	var fdl Dialer
	if prxURL != nil {
		dl, e := gp.FromURL(prxURL, wr)
		if e == nil {
			fdl = dl.Dial
		} else {
			log.Print(e.Error())
		}
	} else {
		fdl = wr.Dial
	}
	if fdl != nil {
		p.fastCl.Dial = func(addr string) (n net.Conn,
			e error) {
			n, e = fdl("tcp", addr)
			return
		}
	}
	return
}
