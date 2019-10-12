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
	"io"
	"net"
	"sync"
)

type Dialer func(context.Context, string, string) (net.Conn, error)

func transferWg(wg *sync.WaitGroup,
	dest io.Writer, src io.Reader) {
	io.Copy(dest, src)
	wg.Done()
}

// noHijacking error
func noHijacking() (e error) {
	e = fmt.Errorf("No hijacking supported")
	return
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
