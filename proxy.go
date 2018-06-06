package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	h "net/http"
	"time"
)

type Proxy struct {
	Timeout time.Duration
	Tr      *h.Transport
}

func (p *Proxy) ServeHTTP(w h.ResponseWriter, r *h.Request) {
	if r.Method == h.MethodConnect {
		p.handleTunneling(w, r)
	} else {
		p.handleHTTP(w, r)
	}
}

func (p *Proxy) handleTunneling(w h.ResponseWriter, r *h.Request) {
	dest_conn, e := p.Tr.DialContext(context.Background(),
		"tcp", r.Host)
	var hijacker h.Hijacker
	status := h.StatusOK
	if e == nil {
		var ok bool
		hijacker, ok = w.(h.Hijacker)
		if !ok {
			e = NoHijacking()
		}
	} else {
		status = h.StatusServiceUnavailable
	}
	var client_conn net.Conn
	if e == nil {
		client_conn, _, e = hijacker.Hijack()
	} else {
		status = h.StatusInternalServerError
	}
	if e == nil {
		go transfer(dest_conn, client_conn)
		go transfer(client_conn, dest_conn)
	} else {
		status = h.StatusServiceUnavailable
	}

	if e != nil {
		h.Error(w, e.Error(), status)
	} else {
		w.WriteHeader(status)
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
		_, e = io.Copy(w, resp.Body)
		resp.Body.Close()
	}
	if e != nil {
		h.Error(w, e.Error(), h.StatusServiceUnavailable)
	} else {
		w.WriteHeader(resp.StatusCode)
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

func NoHijacking() (e error) {
	e = fmt.Errorf("No hijacking supported")
	return
}
