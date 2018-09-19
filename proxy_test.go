package proxy

import (
	"context"
	"net"
	h "net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProxy(t *testing.T) {
	d := &dialer{t: t}
	p := &Proxy{
		Tr: &h.Transport{
			DialContext: d.DialContext,
		},
	}
	r := httptest.NewRequest(h.MethodGet, "http://bla.com", nil)
	w := httptest.NewRecorder()
	p.handleTunneling(w, r)
}

type dialer struct {
	t *testing.T
}

func (d *dialer) DialContext(ctx context.Context, nt,
	host string) (c net.Conn, e error) {
	r, ok := ctx.Value(ReqKey).(*h.Request)
	require.True(d.t, ok)
	require.Equal(d.t, "bla.com", r.Host)
	return
}
