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
	"github.com/stretchr/testify/require"
	fh "github.com/valyala/fasthttp"
	"net"
	h "net/http"
	ht "net/http/httptest"
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
	server := newMockConn(buff.String())
	dial := func(iface string) func(string,
		string) (net.Conn, error) {
		return func(network, addr string) (n net.Conn, e error) {
			n = server
			return
		}
	}
	req := fh.AcquireRequest()
	req.SetHost(ht.DefaultRemoteAddr)
	req.Header.SetMethod(h.MethodPost)
	req.SetBodyString(bla)
	buff0 := new(bytes.Buffer)
	req.WriteTo(buff0)
	client := newMockConn(buff0.String())

	ctl := func(o *Operation) *Result { return new(Result) }
	p := NewFastProxy(ctl, dial, time.Now)
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
	server := newMockConn(blabla)
	dial := func(iface string) func(string, string) (net.Conn,
		error) {
		return func(network, addr string) (n net.Conn, e error) {
			n = server
			return
		}
	}
	ctl := func(o *Operation) *Result { return new(Result) }
	p := NewFastProxy(ctl, dial, time.Now)
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
	client := newMockConn(s + bla)
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
	ctl := func(o *Operation) *Result { return new(Result) }
	bla := "bla"
	rec := ht.NewRecorder()
	rec.Body.WriteString(bla)
	resp, buff := rec.Result(), new(bytes.Buffer)
	resp.Write(buff)
	server := newMockConn(buff.String())
	dial := func(iface string) func(string, string) (net.Conn,
		error) {
		return func(network, addr string) (n net.Conn, e error) {
			n = server
			return
		}
	}
	p := NewProxy(ctl, dial, time.Now)
	w, r :=
		ht.NewRecorder(),
		ht.NewRequest(h.MethodGet, "http://example.com", nil)
	go func() { p.ServeHTTP(w, r) }()
	<-server.clöse
	require.Equal(t, bla, w.Body.String())
}

func TestStdProxyConnect(t *testing.T) {
	ctl := func(o *Operation) *Result { return new(Result) }
	bla, blabla := "bla", "blabla"
	client, server := newMockConn(bla), newMockConn(blabla)
	dial := func(iface string) func(string, string) (net.Conn,
		error) {
		return func(network, addr string) (n net.Conn, e error) {
			n = server
			return
		}
	}
	p := NewProxy(ctl, dial, time.Now)
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
	client, server := newMockConn(bla), newMockConn(blabla)
	copyConns(server, client)
	<-client.clöse
	<-server.clöse
	require.Equal(t, bla, server.write.String())
	require.Equal(t, blabla, client.write.String())
}

type mockConn struct {
	name  string
	read  *bytes.Buffer
	write *bytes.Buffer
	clöse chan bool
}

func newMockConn(content string) (m *mockConn) {
	m = &mockConn{
		read:  bytes.NewBufferString(content),
		write: new(bytes.Buffer),
		clöse: make(chan bool, 0),
	}
	return
}

func (m *mockConn) Read(p []byte) (n int, e error) {
	n, e = m.read.Read(p)
	return
}

func (m *mockConn) Write(p []byte) (n int, e error) {
	n, e = m.write.Write(p)
	return
}

func (m *mockConn) Close() (e error) { m.clöse <- true; return }

func (m *mockConn) LocalAddr() (a net.Addr) { return }

func (m *mockConn) RemoteAddr() (a net.Addr) { return }

func (m *mockConn) SetDeadline(t time.Time) (e error) { return }

func (m *mockConn) SetReadDeadline(t time.Time) (e error) {
	return
}

func (m *mockConn) SetWriteDeadline(t time.Time) (e error) {
	return
}
