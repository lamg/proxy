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
	"fmt"
	"io"
	"net"
	h "net/http"
	"net/url"
	"sync"
	"time"

	alg "github.com/lamg/algorithms"
	fh "github.com/valyala/fasthttp"
	gp "golang.org/x/net/proxy"
)

type proxyS struct {
	clock   func() time.Time
	timeout time.Duration
	preConn IfaceParentProxy
	ctlConn ControlConn
	trans   *h.Transport
	fastCl  *fh.Client
}

// NewProxy creates a net/http.Handler ready to be used
// as an HTTP/HTTPS proxy server in conjunction with
// a net/http.Server
func NewProxy(
	preConn IfaceParentProxy,
	ctlConn ControlConn,
	dialTimeout time.Duration,
	maxIdleConns int,
	idleConnTimeout,
	tlsHandshakeTimeout,
	expectContinueTimeout time.Duration,
	clock func() time.Time,
) (p h.Handler) {
	prx := &proxyS{
		clock:   clock,
		preConn: preConn,
		ctlConn: ctlConn,
		timeout: dialTimeout,
		trans: &h.Transport{
			MaxIdleConns:          maxIdleConns,
			IdleConnTimeout:       idleConnTimeout,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			ExpectContinueTimeout: expectContinueTimeout,
		},
	}
	prx.trans.DialContext = prx.dialContext
	gp.RegisterDialerType("http", newHTTPProxy)
	p = prx
	return
}

type ifaceProxyKT string

const ifaceProxyK = "ifaceProxyK"

type ifaceProxy struct {
	ip    string
	iface string
	proxy *url.URL
	e     error
}

func (p *proxyS) ServeHTTP(w h.ResponseWriter,
	r *h.Request) {
	t := p.clock()
	i := new(ifaceProxy)
	i.ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	i.iface, i.proxy, i.e = p.preConn(r.Method, r.URL.String(),
		i.ip, t)
	c := context.WithValue(r.Context(), ifaceProxyK, i)
	nr := r.WithContext(c)
	if r.Method == h.MethodConnect {
		p.handleTunneling(w, nr)
	} else {
		p.handleHTTP(w, nr)
	}
}

var (
	clientConnectOK = []byte("HTTP/1.0 200 OK\r\n\r\n")
)

func (p *proxyS) handleTunneling(w h.ResponseWriter,
	r *h.Request) {
	destConn, e := p.dialContext(r.Context(), "tcp", r.Host)

	var hijacker h.Hijacker
	status := h.StatusOK
	if e == nil {
		var ok bool
		w.WriteHeader(status)
		hijacker, ok = w.(h.Hijacker)
		if !ok {
			e = noHijacking()
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

func (p *proxyS) handleHTTP(w h.ResponseWriter,
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

func (p *proxyS) dialContext(ctx context.Context, network,
	addr string) (c net.Conn, e error) {
	ifpr := ctx.Value(ifaceProxyK).(*ifaceProxy)
	if ifpr != nil && ifpr.e == nil {
		dlr := &net.Dialer{
			Timeout: p.timeout,
		}
		if ifpr.iface != "" {
			var nf *net.Interface
			nf, e = net.InterfaceByName(ifpr.iface)
			var laddr []net.Addr
			if e == nil {
				laddr, e = nf.Addrs()
			}
			if len(laddr) != 0 {
				dlr.LocalAddr = &net.TCPAddr{IP: laddr[0].(*net.IPNet).IP}
			} else {
				e = fmt.Errorf("No local IP")
			}
		}
		var n net.Conn
		if ifpr.proxy != nil {
			var d gp.Dialer
			d, e = gp.FromURL(ifpr.proxy, dlr)
			if e == nil {
				n, e = d.Dial(network, addr)
			}
		} else {
			n, e = dlr.Dial(network, addr)
		}
		if e == nil {
			c = &ctlConn{Conn: n, ip: ifpr.ip, ctl: p.ctlConn}
		}
	} else if ifpr != nil && ifpr.e != nil {
		e = ifpr.e
	} else {
		e = fmt.Errorf("No connection parameters in context")
	}
	return
}

type ctlConn struct {
	net.Conn
	ip  string
	ctl ControlConn
}

func (c *ctlConn) Read(p []byte) (n int, e error) {
	e = c.ctl(Request, c.ip, len(p))
	if e == nil {
		n, e = c.Conn.Read(p)
		e = c.ctl(Report, c.ip, n)
	}
	return
}

func (c *ctlConn) Close() (e error) {
	e = c.ctl(Close, c.ip, 0)
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
	ok, _ = alg.BLnSrch(ib, len(hbh))
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

// noHijacking error
func noHijacking() (e error) {
	e = fmt.Errorf("No hijacking supported")
	return
}
