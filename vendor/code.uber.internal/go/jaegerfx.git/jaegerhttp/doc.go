// Package jaegerhttp provides middleware that instruments net/http servers
// and clients with Jaeger tracing.
//
// Clients
//
// HTTP clients provided by httpfx 2.0 or newer should be instrumented with
// Jaeger by default. See the documentation for httpfx.git for more
// information.
//
// To manually instrument an HTTP client with jaegerhttp, wrap its Transport
// with StartSpanMiddleware and InjectSpanMiddleware.
//
//   client := ...
//   startSpanMW := StartSpanMiddleware(tracer)
//   injectSpanMW := InjectSpanMiddleware(tracer)
//   client.Transport = startSpanMW(injectSpanMW(client.Transport))
//
// Servers
//
// Wrap an http.Handler with ExtractSpanMiddleware to add Jaeger support to
// it.
//
//   handler := ...
//   extractSpanMW := ExtractSpanMiddleware(tracer)
//   handler = extractSpanMW(handler)
//
// See Also
//
// https://go.uberinternal.com/pkg/code.uber.internal/go/httpfx.git
package jaegerhttp // import "code.uber.internal/go/jaegerfx.git/jaegerhttp"
