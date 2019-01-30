package proxy

import (
	"context"
	fh "github.com/valyala/fasthttp"
	"net"
	"net/http"
	"time"
)

func NewFastProxy(d DialCtxF, v CtxValueF, maxIdleConns int,
	idleConnTimeout, tlsHandshakeTimeout,
	expectContinueTimeout time.Duration,
	clock func() time.Time,
) (p *Proxy) {
	p = &Proxy{
		clock:       clock,
		dialContext: d,
		ctxValue:    v,
		fastCl: &fh.Client{
			MaxIdleConnDuration: idleConnTimeout,
		},
	}
	return
}

func (p *Proxy) ReqHnd(ctx *fh.RequestCtx) {
	uri := ctx.URI()
	nctx := p.ctxValue(ctx, string(ctx.Method()),
		uri.String(), ctx.RemoteAddr().String(),
		p.clock())
	if ctx.IsConnect() {
		dest, e := p.dialContext(nctx, "tcp",
			string(uri.Host()))
		status := http.StatusOK
		if e == nil {
			ctx.Hijack(copyHijacked(dest))
		} else {
			ctx.Response.SetBodyString(e.Error())
			status = http.StatusServiceUnavailable
		}
		ctx.Response.SetStatusCode(status)
	} else {
		// copy headers
		println("ok")
		p.fastCl.Dial = dialer(nctx, p.dialContext)
		p.fastCl.Do(&ctx.Request, &ctx.Response)
	}
}

func copyHijacked(dest net.Conn) (h fh.HijackHandler) {
	h = func(src net.Conn) {
		go transfer(dest, src)
		transfer(src, dest)
	}
	return
}

func dialer(ctx context.Context,
	dc DialCtxF) (d fh.DialFunc) {
	d = func(addr string) (c net.Conn, e error) {
		println(addr)
		c, e = dc(ctx, "tcp", addr)
		println(c != nil)
		return
	}
	return
}
