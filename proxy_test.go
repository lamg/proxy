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
	"bufio"
	"bytes"
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	fh "github.com/valyala/fasthttp"
	"io/ioutil"
	"net"
	h "net/http"
	ht "net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestFastProxyRoundTrip(t *testing.T) {
	bla, blabla := "bla", "blabla"
	resp := fh.AcquireResponse()
	resp.SetStatusCode(h.StatusOK)
	resp.SetBodyString(blabla)
	buff := new(bytes.Buffer)
	resp.WriteTo(buff)
	server := newMockConn(buff.String(), false)
	dial := func(c context.Context, n, a string) (net.Conn, error) {
		return server, nil
	}
	req := fh.AcquireRequest()
	req.SetHost(ht.DefaultRemoteAddr)
	req.Header.SetMethod(h.MethodPost)
	req.SetBodyString(bla)
	buff0 := new(bytes.Buffer)
	req.WriteTo(buff0)
	client := newMockConn(buff0.String(), false)

	p := NewFastProxy(dial)
	srv := &fh.Server{
		Handler: p.RequestHandler,
	}
	go func() {
		e := srv.ServeConn(client)
		require.NoError(t, e)
	}()
	<-client.clöse
	resp0 := fh.AcquireResponse()
	resp0.Read(bufio.NewReader(client.write))
	require.Equal(t, blabla, string(resp0.Body()))
	req0 := fh.AcquireRequest()
	req0.Read(bufio.NewReader(server.write))
	require.Equal(t, bla, string(req0.Body()))
}

func TestFastProxyConnect(t *testing.T) {
	bla, blabla := "bla", "blabla"
	server := newMockConn(blabla, false)
	dial := func(c context.Context, n, a string) (net.Conn, error) {
		return server, nil
	}

	p := NewFastProxy(dial)
	srv := &fh.Server{
		Handler: p.RequestHandler,
	}
	r := fh.AcquireRequest()
	r.Header.SetMethod(h.MethodConnect)
	r.SetHost(ht.DefaultRemoteAddr)
	buff := new(bytes.Buffer)
	_, e := r.WriteTo(buff)
	require.NoError(t, e)
	s := buff.String()
	client := newMockConn(s+bla, false)
	// client connection has the content of a valid request followed
	// by raw data
	e0 := srv.ServeConn(client)
	require.NoError(t, e0)
	<-server.clöse
	<-client.clöse
	require.Equal(t, bla, server.write.String())
	// only connection's raw data reached the server
	for i := 0; i != 5; i++ {
		client.write.ReadString('\n')
	}
	// skiped HTTP response, now just the raw connection data
	// is in the buffer
	require.Equal(t, blabla, client.write.String())
}

func TestStdProxyRoundTrip(t *testing.T) {
	bla := "bla"
	rec := ht.NewRecorder()
	rec.Body.WriteString(bla)
	resp, buff := rec.Result(), new(bytes.Buffer)
	resp.Write(buff)
	r := ht.NewRequest(h.MethodGet, "http://example.com", nil)
	resp.Request = r
	s := buff.String()
	server := newMockConn(s, true)
	dial := func(c context.Context, n, a string) (net.Conn, error) {
		return server, nil
	}
	p := NewProxy(dial)
	w := ht.NewRecorder()
	p.ServeHTTP(w, r)
	require.Equal(t, bla, w.Body.String())
}

func TestStdProxyConnect(t *testing.T) {
	bla, blabla := "bla", "blabla"
	client, server :=
		newMockConn(bla, false),
		newMockConn(blabla, false)
	dial := func(c context.Context, n, a string) (net.Conn, error) {
		return server, nil
	}
	p := NewProxy(dial)
	w, r :=
		&hijacker{
			ResponseRecorder: ht.NewRecorder(),
			n:                client,
		},
		ht.NewRequest(h.MethodConnect, ht.DefaultRemoteAddr, nil)
	p.ServeHTTP(w, r)
	require.Equal(t, h.StatusOK, w.Code)
	<-client.clöse
	<-server.clöse
	require.Equal(t, bla, server.write.String())
	require.Equal(t, blabla, client.write.String())
}

func TestContainsNilIP(t *testing.T) {
	var ip net.IP
	_, ipn, e := net.ParseCIDR("127.0.0.1/32")
	require.NoError(t, e)
	f := ipn.Contains(ip)
	require.False(t, f)
}

type hijacker struct {
	*ht.ResponseRecorder
	n net.Conn
}

func (j *hijacker) Hijack() (c net.Conn,
	b *bufio.ReadWriter, e error) {
	c = j.n
	return
}

func TestCopyConns(t *testing.T) {
	bla, blabla := "bla", "blabla"
	client, server :=
		newMockConn(bla, false),
		newMockConn(blabla, false)
	copyConns(server, client)
	<-client.clöse
	<-server.clöse
	require.Equal(t, bla, server.write.String())
	require.Equal(t, blabla, client.write.String())
}

type mockConn struct {
	name       string
	read       *bytes.Buffer
	write      *bytes.Buffer
	clöse      chan bool
	closed     bool
	writeFirst bool
	writeOk    chan bool
}

func newMockConn(content string, writeFirst bool) (m *mockConn) {
	m = &mockConn{
		read:       bytes.NewBufferString(content),
		write:      new(bytes.Buffer),
		clöse:      make(chan bool, 1),
		writeOk:    make(chan bool, 1),
		writeFirst: writeFirst,
	}
	return
}

func (m *mockConn) Read(p []byte) (n int, e error) {
	if m.writeFirst && m.read.Len() != 0 {
		<-m.writeOk
	}
	n, e = m.read.Read(p)
	return
}

func (m *mockConn) Write(p []byte) (n int, e error) {
	n, e = m.write.Write(p)
	if m.writeFirst {
		m.writeOk <- true
	}
	return
}

func (m *mockConn) Close() (e error) {
	m.clöse <- true
	return
}

func (m *mockConn) LocalAddr() (a net.Addr) { return }

func (m *mockConn) RemoteAddr() (a net.Addr) { return }

func (m *mockConn) SetDeadline(t time.Time) (e error) { return }

func (m *mockConn) SetReadDeadline(t time.Time) (e error) {
	return
}

func (m *mockConn) SetWriteDeadline(t time.Time) (e error) {
	return
}

func TestIfaceDialer(t *testing.T) {
	addr, bla := "127.0.0.1:8000", "bla"
	l, e := net.Listen(tcp, addr)
	go func(lst net.Listener) {
		for {
			c, e := lst.Accept()
			if e == nil {
				c.Write([]byte(bla))
				c.Close()
			}
		}
	}(l)
	iface, e0 := net.InterfaceByIndex(1)
	var ifaceName string
	if e0 == nil {
		ifaceName = iface.Name
	}
	ifd := &IfaceDialer{
		Timeout:   90 * time.Second,
		Interface: ifaceName,
	}
	if e == nil {
		n, e := ifd.Dial(tcp, addr)
		if e == nil {
			bs, e := ioutil.ReadAll(n)
			require.NoError(t, e)
			require.Equal(t, bla, string(bs))
		}
	}
	if e != nil {
		t.Log(e)
	}
	ifd.Interface = " pipo pérez "
	_, e = ifd.Dial(tcp, addr)
	var ne *NoLocalIPErr
	ok := errors.As(e, &ne)
	require.True(t, ok)
	require.Equal(t, ifd.Interface, ne.Interface)
}

func TestRegisterHTTPProxy(t *testing.T) {
	prx, e := url.Parse("http://127.0.0.1:8080")
	require.NoError(t, e)
	resp := "HTTP/1.1 502 Bad Gateway\r\n\r\n"
	dlr := &dialer{c: newMockConn(resp, false)}
	hpd, _ := newHTTPProxy(prx, dlr)
	_, e = hpd.Dial(tcp, "https://example.com")
	var err *ExpectingCodeErr
	ok := errors.As(e, &err)
	require.True(t, ok)
	require.Equal(t, h.StatusOK, err.Expected)
	require.Equal(t, h.StatusBadGateway, err.Actual)
}

type dialer struct {
	c net.Conn
}

func (d *dialer) Dial(n, a string) (net.Conn, error) {
	return d.c, nil
}
