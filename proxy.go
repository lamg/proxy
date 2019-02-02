package proxy

import (
	"context"
	"fmt"
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

// Proxy is an HTTP proxy
type Proxy struct {
	clock     func() time.Time
	trans     *h.Transport
	direct    ContextDialer
	connectDl Dialer
	contextV  ContextValueF
	parent    ParentProxyF

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

func (d *dlWrap) Dial(nt, addr string) (c net.Conn,
	e error) {
	c, e = d.dl(d.ctx(), nt, addr)
	return
}

func NewProxy(direct ContextDialer, v ContextValueF,
	maxIdleConns int,
	idleConnTimeout, tlsHandshakeTimeout,
	expectContinueTimeout time.Duration,
	clock func() time.Time,
	prxF ParentProxyF,
) (p *Proxy) {
	p = &Proxy{
		clock:    clock,
		contextV: v,
		direct:   direct,
		trans: &h.Transport{
			Proxy: func(r *h.Request) (u *url.URL,
				e error) {
				u, e = prxF(r.Method, r.URL.String(),
					r.RemoteAddr, clock())
				return
			},
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
	p.setStdDl(r.Context(), r.Method, r.URL.String(),
		r.RemoteAddr)
	if r.Method == h.MethodConnect {
		p.handleTunneling(w, r)
	} else {
		p.handleHTTP(w, r)
	}
}

var (
	clientConnectOK = []byte("HTTP/1.0 200 OK\r\n\r\n")
)

func (p *Proxy) handleTunneling(w h.ResponseWriter,
	r *h.Request) {
	destConn, e := p.connectDl("tcp", r.Host)

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
		copyConns(destConn, clientConn)
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

func (p *Proxy) setStdDl(c context.Context,
	meth, ürl, rAddr string) {
	t := p.clock()
	ctx := p.contextV(c, meth, ürl, rAddr, t)
	prxURL, e := p.parent(meth, ürl, rAddr, t)
	if e != nil {
		log.Print(e.Error())
	}
	wr := &dlWrap{
		ctx: func() context.Context { return ctx },
		dl:  p.direct,
	}
	p.connectDl = wr.Dial
	if prxURL != nil {
		prxDl, e := gp.FromURL(prxURL, wr)
		if e == nil {
			p.connectDl = prxDl.Dial
		} else {
			log.Print(e.Error())
		}
	}
	// p.connectDl correctly set if there's a proxy
	return
}

func copyConns(dest, client net.Conn) {
	// learning from https://github.com/elazarl/goproxy
	// /blob/2ce16c963a8ac5bd6af851d4877e38701346983f
	// /https.go#L103
	client.Write(clientConnectOK)
	clientTCP, cok := client.(*net.TCPConn)
	destTCP, dok := dest.(*net.TCPConn)
	if cok && dok {
		go transfer(destTCP, clientTCP)
		go transfer(clientTCP, destTCP)
	} else {
		go func() {
			var wg sync.WaitGroup
			wg.Add(2)
			go transferWg(&wg, dest, client)
			go transferWg(&wg, client, dest)
			wg.Wait()
			client.Close()
			dest.Close()
		}()
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

func searchHopByHop(hd string) (ok bool) {
	// hop-by-hop headers. Shouldn't be sent to the
	// requested host.
	// https://developer.mozilla.org/en-US/docs/
	// Web/HTTP/Headers#hbh
	hbh := []string{
		"Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "TE", "Trailer",
		"Transfer-Encoding", "Upgrade",
	}
	ib := func(i int) (b bool) {
		b = hbh[i] == hd
		return
	}
	ok, _ = bLnSrch(ib, len(hbh))
	return
}

func copyHeader(dst, src h.Header) {
	for k, vv := range src {
		ok := searchHopByHop(k)
		if !ok {
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

type intBool func(int) bool

// bLnSrch is the bounded lineal search algorithm
// { n ≥ 0 ∧ forall.n.(def.ib) }
// { i =⟨↑j: 0 ≤ j ≤ n ∧ ⟨∀k: 0 ≤ k < j: ¬ib.k⟩: j⟩
//   ∧ b ≡ i ≠ n }
func bLnSrch(ib intBool, n int) (b bool, i int) {
	b, i, udb := false, 0, true
	// udb: undefined b for i
	for !b && i != n {
		if udb {
			b, udb = ib(i), false
		} else {
			i, udb = i+1, true
		}
	}
	return
}
