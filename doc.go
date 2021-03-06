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

// HTTP/HTTPS proxy library with custom dialer, which receives
// in the context key `proxy.ReqParamsK` a `*proxy.ReqParams`
// instance with the IP that made the request, it's URL and
// method. It can be served using net/http.Server or
// github.com/valyala/fasthttp.Server and
// has two builtin dialers for dialing with a specific network
// interface or parent proxy.
package proxy
