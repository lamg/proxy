package proxy

import (
	"io"
	"net"
	h "net/http"
	"time"
)

type Proxy struct {
	Timeout     time.Duration
	Tr          *h.Transport
	ConnectDial func(ntw, addr string,
		timeout time.Duration) (net.Conn, error)
}

func (p *Proxy) ServeHTTP(w h.ResponseWriter, r *h.Request) {
	if r.Method == h.MethodConnect {
		p.handleTunneling(w, r)
	} else {
		p.handleHTTP(w, r)
	}
}

func (p *Proxy) handleTunneling(w h.ResponseWriter, r *h.Request) {
	dest_conn, e := p.ConnectDial("tcp", r.Host, p.Timeout)
	var hijacker h.Hijacker
	var status int
	if e == nil {
		w.WriteHeader(h.StatusOK)
		hijacker, ok := w.(h.Hijacker)
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
	}
}

func transfer(dest io.WriteCloser, src io.ReadCloser) {
	io.Copy(dest, src)
	des.Close()
	src.Close()
}

func (p *Proxy) handleHTTP(w h.ResponseWriter, req *h.Request) {
	resp, e := p.Tr.RoundTrip(req)
	if e == nil {
		copyHeader(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, e = io.Copy(w, resp.Body)
		resp.Body.Close()
	}
	if e != nil {
		h.Error(w, err.Error(), h.StatusServiceUnavailable)
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
