package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	h "net/http"
)

// Proxy is an http.Handler that proxying HTTP or HTTPS requests
type Proxy struct {
	Tr *h.Transport
}

func (p *Proxy) ServeHTTP(w h.ResponseWriter, r *h.Request) {
	if r.Method == h.MethodConnect {
		p.handleTunneling(w, r)
	} else {
		p.handleHTTP(w, r)
	}
}

// ReqKeyT is the type for creating the value sent in context
type ReqKeyT struct {
	Value string
}

// ReqKey is the instance sent in context
var ReqKey = &ReqKeyT{Value: "ReqKey"}

func (p *Proxy) handleTunneling(w h.ResponseWriter, r *h.Request) {
	ctx := context.WithValue(context.Background(), ReqKey, r)
	destConn, e := p.Tr.DialContext(ctx, "tcp", r.Host)
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
		go transfer(destConn, clientConn)
		go transfer(clientConn, destConn)
	} else {
		status = h.StatusServiceUnavailable
	}

	if e != nil {
		h.Error(w, e.Error(), status)
	}
}

func transfer(dest io.WriteCloser, src io.ReadCloser) {
	io.Copy(dest, src)
	dest.Close()
	src.Close()
}

func (p *Proxy) handleHTTP(w h.ResponseWriter, req *h.Request) {
	resp, e := p.Tr.RoundTrip(req)
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
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers#hbh
	hbh := []string{
		"Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding",
		"Upgrade",
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
