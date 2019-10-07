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
	"sync"
	"time"

	alg "github.com/lamg/algorithms"
	fh "github.com/valyala/fasthttp"
	gp "golang.org/x/net/proxy"
)

type Proxy struct {
	now     func() time.Time
	timeout time.Duration
	ctl     ConnControl
	trans   *h.Transport
	fastCl  *fh.Client
	Log     func(string)
}

// NewProxy creates a net/http.Handler ready to be used
// as an HTTP/HTTPS proxy server in conjunction with
// a net/http.Server
func NewProxy(
	ctl ConnControl,
	dialTimeout time.Duration,
	now func() time.Time,
) (p *Proxy) {
	prx := &Proxy{
		now:     now,
		ctl:     ctl,
		timeout: dialTimeout,
		trans:   new(h.Transport),
	}
	prx.trans.DialContext = prx.dialContext
	gp.RegisterDialerType("http", newHTTPProxy)
	p = prx
	return
}

type reqParamsKT string

const reqParamsK = "reqParams"

type reqParams struct {
	method string
	ip     string
	ürl    string
}

func (p *Proxy) ServeHTTP(w h.ResponseWriter,
	r *h.Request) {
	i := &reqParams{method: r.Method, ürl: r.URL.Host}
	var e error
	i.ip, _, e = net.SplitHostPort(r.RemoteAddr)
	if e == nil {
		c := context.WithValue(r.Context(), reqParamsK, i)
		nr := r.WithContext(c)
		if r.Method == h.MethodConnect {
			p.handleTunneling(w, nr)
		} else {
			p.handleHTTP(w, nr)
		}
	} else {
		h.Error(w, "Malformed remote address "+r.RemoteAddr,
			h.StatusBadRequest)
	}
}

func (p *Proxy) handleTunneling(w h.ResponseWriter,
	r *h.Request) {
	destConn, e := p.dialContext(r.Context(), "tcp", r.Host)

	var hijacker h.Hijacker
	status := h.StatusOK
	if e == nil {
		w.WriteHeader(status)
		var ok bool
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
		p.log("net/http tunneling: " + e.Error())
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

func (p *Proxy) dialContext(ctx context.Context, network,
	addr string) (c net.Conn, e error) {
	rqp := ctx.Value(reqParamsK).(*reqParams)
	op := &Operation{
		Command: Open,
		Method:  rqp.method,
		IP:      rqp.ip,
		URL:     rqp.ürl,
		Time:    p.now(),
	}
	r := p.ctl(op)
	e = r.Error
	var n net.Conn
	if e == nil {
		dlr := &net.Dialer{
			Timeout: p.timeout,
		}
		if r.Iface != "" {
			var nf *net.Interface
			nf, e = net.InterfaceByName(r.Iface)
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
		if r.Proxy != nil {
			p.log("parent proxy: " + r.Proxy.String())
			var d gp.Dialer
			d, e = gp.FromURL(r.Proxy, dlr)
			if e == nil {
				n, e = d.Dial(network, addr)
			}
		} else {
			p.log("dial:" + network + " " + addr)
			n, e = dlr.Dial(network, addr)
		}
	}
	if e == nil {
		nc := &ctlConn{Conn: n, par: rqp, ctl: p.ctl, now: p.now}
		nc.log = p.Log
		c = nc
	} else {
		p.log("error: " + e.Error())
	}
	return
}

func (p *Proxy) log(s string) {
	if p.Log != nil {
		p.Log(s)
	}
}

type ctlConn struct {
	net.Conn
	par *reqParams
	ctl ConnControl
	now func() time.Time
	log func(string)
}

func (c *ctlConn) Read(p []byte) (n int, e error) {
	e = c.operation(ReadRequest, len(p))
	if e == nil {
		n, e = c.Conn.Read(p)
		c.operation(ReadReport, n)
	}
	return
}

func (c *ctlConn) Close() (e error) {
	e = c.Conn.Close()
	c.operation(Close, 0)
	return
}

func (c *ctlConn) operation(op, amount int) (e error) {
	x := &Operation{
		Command: op,
		IP:      c.par.ip,
		Method:  c.par.method,
		URL:     c.par.ürl,
		Time:    c.now(),
		Amount:  amount,
	}
	e = c.ctl(x).Error
	if c.log != nil {
		c.log(fmt.Sprintf(
			"Op: %d Remote: %s URL: %s Amount: %d Error:%v",
			op, c.par.ip, c.par.ürl, amount, e,
		))
	}
	return
}

func copyConns(dest, client net.Conn) {
	// learning from https://github.com/elazarl/goproxy
	// /blob/2ce16c963a8ac5bd6af851d4877e38701346983f
	// /https.go#L103
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
