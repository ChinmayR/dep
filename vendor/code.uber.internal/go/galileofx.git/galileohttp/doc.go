// Package galileohttp provides middleware that instruments net/http clients
// and servers with Galileo authentication.
//
// Clients
//
// HTTP clients provided by httpfx 2.0 or newer should be instrumented with
// Galileo by default. See the httpfx.git documentation for more information.
//
// To manually instrument an HTTP client with galileohttp, wrap its Transport
// with AuthenticateOutMiddleware and Jaeger tracing support. See the
// jaegerfx.git/jaegerhttp documentation for information on how to instrument
// HTTP clients with Jaeger support.
//
//   var client http.Client
//   startTraceMW, endTraceMW := ...  // see jaegerfx
//   authOutMW := galileohttp.AuthenticateOutMiddleware(g)
//   client.Transport = startTraceMW(authOutMW(endTraceMW(client.Transport)))
//
// Servers
//
// Wrap an http.Handler with AuthenticateInMiddleware and Jaeger tracing
// support to add Galileo authentication for incoming requests. See the
// jaegerfx.git/jaegerhttp documentation for information on how to instrument
// HTTP servers with Jaeger support.
//
//   handler := ...  // Your http.Handler
//   traceMW := ...  // See jaegerfx
//   authInMW := galileohttp.AuthenticateInMiddleware(g)
//   handler = traceMW(authInMW(handler))
//
// See Also
//
// https://go.uberinternal.com/pkg/code.uber.internal/go/httpfx.git
// https://go.uberinternal.com/pkg/code.uber.internal/go/jaegerfx.git/jaegerhttp/
package galileohttp // import "code.uber.internal/go/galileofx.git/galileohttp"
