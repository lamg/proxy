package proxy

import (
	"net"
	h "net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProxy(t *testing.T) {
	d := &dialer{t: t}
	p := &Proxy{
		Dial: d.Dial,
	}
	r := httptest.NewRequest(h.MethodGet, "http://bla.com", nil)
	w := httptest.NewRecorder()
	p.handleTunneling(w, r)
}

type dialer struct {
	t *testing.T
}

func (d *dialer) Dial(r *h.Request) (c net.Conn, e error) {
	require.Equal(d.t, "bla.com", r.Host)
	return
}
