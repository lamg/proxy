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
	alg "github.com/lamg/algorithms"
	gp "golang.org/x/net/proxy"
	"io"
	"net"
	h "net/http"
	"net/url"
	"sync"
	"time"
)

// ConnControl is the signature of a function that returns if
// is possible carriyng on with the connection operation
type ConnControl func(*Operation) *Result

type Operation struct {
	// One of Open, ReadRequest, ReadReport, Close
	Command int
	// IP at HTTP request that originated the connection operation
	IP string
	// Method of that request
	Method string
	// URL of that request
	URL string
	// time when the operation was created
	Time time.Time
	// Amount of bytes requested to be read or actually read
	// sent with commands ReadRequest and ReadReport respectively
	Amount int
}

type Result struct {
	// Iface is the network interface for dialing the connection.
	// This field is used only with the Open command
	Iface string
	// Proxy is the proxy URL for dialing the connection.
	// This field is used only with the Open command
	Proxy *url.URL
	// Error not nil means the operation will not be performed
	// by the connection. An error sent with the Open operation
	// means the connection won't be opened, and therefore the
	// rest of operations (ReadRequest, ReadReport, Close)
	// won't be performed
	Error error
}

const (
	// Open command must return the proper network interface and
	// proxy for dialing a connection. They are not used for the
	// rest of the commands
	Open = iota
	// ReadRequest command is performed before calling the Read
	// method of the underlying connection. It's sent with
	// the amount of bytes requested to read.
	ReadRequest
	// ReadReport command is performed after calling the Read
	// method of the underlynig connection. It's sent with
	// the amount of bytes actually read
	ReadReport
	// Close is performed after the underlying connection is closed
	Close
)

// DefaultConnControl is the default connection controller
// (ConnControl) returning the proper result for dialing the
// connections with the proxy from environment, using the default
// network interface
func DefaultConnControl(op *Operation) (r *Result) {
	p, e := h.ProxyFromEnvironment(nil)
	r = &Result{Proxy: p, Error: e}
	return
}

type NetworkDialer struct {
	Timeout time.Duration
}

func (p *NetworkDialer) Dialer(iface string) func(string,
	string) (net.Conn, error) {
	return func(network, addr string) (n net.Conn, e error) {
		dlr := &net.Dialer{
			Timeout: p.Timeout,
		}
		if iface != "" {
			var nf *net.Interface
			nf, e = net.InterfaceByName(iface)
			var laddr []net.Addr
			if e == nil {
				laddr, e = nf.Addrs()
			}
			if len(laddr) != 0 {
				dlr.LocalAddr = &net.TCPAddr{IP: laddr[0].(*net.IPNet).IP}
			} else {
				e = &NoLocalIPErr{Iface: iface}
			}
		}
		n, e = dlr.Dial(network, addr)
		return
	}
}

type NoLocalIPErr struct {
	Iface string
}

func (e *NoLocalIPErr) Error() (s string) {
	s = fmt.Sprintf("No local IP for '%s'", e.Iface)
	return
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
		dlr := p.DialFunc(r.Iface)
		if r.Proxy != nil {
			var d gp.Dialer
			d, e = gp.FromURL(r.Proxy, &dialer{dlr})
			if e == nil {
				n, e = d.Dial(network, addr)
			}
		} else {
			n, e = dlr(network, addr)
		}
	}
	if e == nil {
		nc := &ctlConn{Conn: n, par: rqp, ctl: p.ctl, now: p.now}
		c = nc
	}
	return
}

type dialer struct {
	dlr func(string, string) (net.Conn, error)
}

func (d *dialer) Dial(network, addr string) (net.Conn, error) {
	return d.dlr(network, addr)
}

type ctlConn struct {
	net.Conn
	par *reqParams
	ctl ConnControl
	now func() time.Time
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
