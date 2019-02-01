package proxy

import (
	"context"
	"fmt"
	fh "github.com/valyala/fasthttp"
	gp "golang.org/x/net/proxy"
	"io"
	"net"
	h "net/http"
	"net/url"
	"sync"
	"time"
)

// Proxy is an HTTP proxy
type Proxy struct {
	clock     func() time.Time
	trans     *h.Transport
	connectDl ContextDialer
	contextV  ContextValueF

	fastCl *fh.Client
}

type ContextValueF func(
	context.Context,
	string, // HTTP method
	string, // URL
	string, // Remote address
	time.Time,
) context.Context

type ContextDialer func(context.Context, string,
	string) (net.Conn, error)

type Dialer func(string, string) (net.Conn, error)

type dlWrap struct {
	ctx func() context.Context
	dl  ContextDialer
}

func (d *dlWrap) Dial(nt, addr string) (c net.Conn, e error) {
	c, e = d.dl(d.ctx(), nt, addr)
	return
}

func NewProxy(direct ContextDialer, v ContextValueF,
	maxIdleConns int,
	idleConnTimeout, tlsHandshakeTimeout,
	expectContinueTimeout time.Duration,
	clock func() time.Time,
	prxF func(*h.Request) (*url.URL, error),
) (p *Proxy, e error) {
	p = &Proxy{
		clock:    clock,
		contextV: v,
		trans: &h.Transport{
			Proxy:                 prxF,
			DialContext:           direct,
			MaxIdleConns:          maxIdleConns,
			IdleConnTimeout:       idleConnTimeout,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			ExpectContinueTimeout: expectContinueTimeout,
		},
	}
	gp.RegisterDialerType("http", newHTTPProxy)
	return
}

func (p *Proxy) ServeHTTP(w h.ResponseWriter,
	r *h.Request) {
	ctx := r.Context()
	nctx := p.contextV(ctx, r.Method, r.URL.String(),
		r.RemoteAddr, p.clock())
	cr := r.WithContext(nctx)
	if r.Method == h.MethodConnect {
		p.handleTunneling(w, cr)
	} else {
		p.handleHTTP(w, cr)
	}
}

func (p *Proxy) handleTunneling(w h.ResponseWriter,
	r *h.Request) {
	e := p.setIfParProxy(r)
	destConn, e := p.connectDl(r.Context(), "tcp", r.Host)

	var hijacker h.Hijacker
	status := h.StatusOK
	if e == nil {
		var ok bool
		w.WriteHeader(status)
		hijacker, ok = w.(h.Hijacker)
		if !ok {
			e = NoHijacking()
		}
	} else {
		status = h.StatusServiceUnavailable
	}
	var clientConn net.Conn
	if e == nil {
		clientConn, _, e = hijacker.Hijack()
	} else {
		status = h.StatusInternalServerError
	}
	if e == nil {
		// learning from https://github.com/elazarl/goproxy
		// /blob/2ce16c963a8ac5bd6af851d4877e38701346983f
		// /https.go#L103
		clientConn.Write([]byte("HTTP/1.0 200 OK\r\n\r\n"))
		clientTCP, cok := clientConn.(*net.TCPConn)
		destTCP, dok := destConn.(*net.TCPConn)
		if cok && dok {
			go transfer(destTCP, clientTCP)
			go transfer(clientTCP, destTCP)
		} else {
			go func() {
				var wg sync.WaitGroup
				wg.Add(2)
				go transferWg(&wg, destConn, clientConn)
				go transferWg(&wg, clientConn, destConn)
				wg.Wait()
				clientConn.Close()
				destConn.Close()
			}()
		}
	} else {
		status = h.StatusServiceUnavailable
	}

	if e != nil {
		h.Error(w, e.Error(), status)
	}
}

func (p *Proxy) handleHTTP(w h.ResponseWriter,
	req *h.Request) {
	resp, e := p.trans.RoundTrip(req)
	if e == nil {
		copyHeader(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, e = io.Copy(w, resp.Body)
		resp.Body.Close()
	} else {
		h.Error(w, e.Error(), h.StatusServiceUnavailable)
	}
}

func (p *Proxy) setIfParProxy(r *h.Request) (e error) {
	prxURL, _ := p.trans.Proxy(r)
	if prxURL != nil {
		wr := &dlWrap{dl: p.trans.DialContext}
		var dl gp.Dialer
		dl, e = gp.FromURL(prxURL, wr)
		if e == nil {
			p.connectDl = func(c context.Context, nt,
				addr string) (n net.Conn, e error) {
				wr.ctx = func() context.Context { return c }
				n, e = dl.Dial(nt, addr)
				return
			}
		}
	} else {
		p.connectDl = p.trans.DialContext
	}
	// p.connectDl correctly set if there's a proxy
	return
}

func transferWg(wg *sync.WaitGroup,
	dest io.Writer, src io.Reader) {
	io.Copy(dest, src)
	wg.Done()
}

func transfer(dest io.WriteCloser, src io.ReadCloser) {
	io.Copy(dest, src)
	dest.Close()
	src.Close()
}

func copyHeader(dst, src h.Header) {
	// hbh: hop-by-hop headers. Shouldn't be sent to the
	// requested host.
	// https://developer.mozilla.org/en-US/docs/
	// Web/HTTP/Headers#hbh
	hbh := []string{
		"Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "TE", "Trailer",
		"Transfer-Encoding", "Upgrade",
	}
	for k, vv := range src {
		f, i := false, 0
		// f: found in hbh
		for !f && i != len(hbh) {
			f, i = k == hbh[i], i+1
		}
		if !f {
			for _, v := range vv {
				dst.Add(k, v)
			}
		}
	}
}

// NoHijacking error
func NoHijacking() (e error) {
	e = fmt.Errorf("No hijacking supported")
	return
}
