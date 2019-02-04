package proxy

import (
	"context"
	fh "github.com/valyala/fasthttp"
	gp "golang.org/x/net/proxy"
	"io"
	"log"
	"net"
	h "net/http"
	"net/url"
	"sync"
	"time"
)

type ParentProxyF func(string, string, string,
	time.Time) (*url.URL, error)

func NewFastProxy(direct ContextDialer,
	v ContextValueF, prxF ParentProxyF,
	clock func() time.Time,
) (p *Proxy) {
	gp.RegisterDialerType("http", newHTTPProxy)
	p = &Proxy{
		direct:   direct,
		contextV: v,
		fastCl: &fh.Client{
			DialDualStack: true,
		},
		clock:  clock,
		parent: prxF,
	}
	return
}

func (p *Proxy) FastHandler(ctx *fh.RequestCtx) {
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

func (p *Proxy) setFastDl(ctx context.Context, method, ürl,
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
