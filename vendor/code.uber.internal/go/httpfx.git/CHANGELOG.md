# Changelog

## v2.0.0 (2017-12-18)

This module has been rewritten to support all users of go-common's x/net/xhttp
package including,

- Galileo support through galileofx.
- Jaeger tracing support through jaegerfx.
- HTTP server support.
- Graceful shutdown for both, servers and clients.

If you were previously using `httpfx.Module`, change that to
`httpfx.ClientModule` to get a Galileo and Jaeger compatible `*http.Client`.
See the package documentation for more information:
<https://go.uberinternal.com/pkg/code.uber.internal/go/httpfx.git/>.

## v1.0.0 (2017-08-01)

- Initial release.
