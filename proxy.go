package proxy

import (
	"context"
	"fmt"
	fh "github.com/valyala/fasthttp"
	"golang.org/x/net/proxy"
	"io"
	"net"
	h "net/http"
	"sync"
	"time"
)

// Proxy is an HTTP proxy
type Proxy struct {
	clock       func() time.Time
	rt          *h.Transport
	dialContext DialCtxF
	ctxValue    CtxValueF

	fastCl *fh.Client
}

type CtxValueF func(
	context.Context,
	string, // HTTP method
	string, // URL
	string, // Remote address
	time.Time,
) context.Context

func NewProxy(d DialCtxF, v CtxValueF, maxIdleConns int,
	idleConnTimeout, tlsHandshakeTimeout,
	expectContinueTimeout time.Duration,
	clock func() time.Time,
) (p *Proxy) {
	rt := &h.Transport{
		Proxy:                 h.ProxyFromEnvironment,
		DialContext:           d,
		MaxIdleConns:          maxIdleConns,
		IdleConnTimeout:       idleConnTimeout,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ExpectContinueTimeout: expectContinueTimeout,
	}
	p = &Proxy{
		clock:       clock,
		rt:          rt,
		dialContext: d,
		ctxValue:    v,
	}
	return
}

type DialCtxF func(context.Context, string,
	string) (net.Conn, error)

func (p *Proxy) ServeHTTP(w h.ResponseWriter,
	r *h.Request) {
	ctx := r.Context()
	nctx := p.ctxValue(ctx, r.Method, r.URL.String(),
		r.RemoteAddr, p.clock())
	cr := r.WithContext(nctx)
	if r.Method == h.MethodConnect {
		p.handleTunneling(w, cr)
	} else {
		p.handleHTTP(w, cr)
	}
}

// type direct struct {
// 	dialf func(string, string) (net.Conn, error)
// }

// func (d *direct) Dial(nt, addr string) (c net.Conn, e error) {
// 	c, e = d.dialf(nt, addr)
// 	return
// }

func (p *Proxy) handleTunneling(w h.ResponseWriter,
	r *h.Request) {
	var destConn net.Conn
	var e error
	dl := proxy.FromEnvironment()
	if dl != nil {
		destConn, e = dl.Dial("tcp", r.Host)
	} else {
		destConn, e = p.rt.DialContext(r.Context(), "tcp", r.Host)
	}

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

func (p *Proxy) handleHTTP(w h.ResponseWriter,
	req *h.Request) {
	resp, e := p.rt.RoundTrip(req)
	if e == nil {
		copyHeader(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, e = io.Copy(w, resp.Body)
		resp.Body.Close()
	} else {
		h.Error(w, e.Error(), h.StatusServiceUnavailable)
	}
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
