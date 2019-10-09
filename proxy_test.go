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
	"net"
	h "net/http"
	ht "net/http/httptest"
	"testing"
	"time"
)

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
