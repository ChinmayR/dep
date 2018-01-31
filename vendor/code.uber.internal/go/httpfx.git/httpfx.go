// Package httpfx provides Uber-compatible HTTP integration for Fx
// applications.
//
// Standard HTTP Client
//
// httpfx.ClientModule provides an http.Client to Fx applications with the
// following features.
//
//   - Graceful shutdown: The application will wait for ongoing requests to
//     finish (up to some timeout) before exiting when the application
//     receives a SIGTERM.
//   - Tracing: All requests made with this HTTP client will have Jaeger
//     tracing data attached to them.
//   - Authentication: Requests made with this HTTP client that have an
//     Rpc-Service header will have Galileo authentication tokens attached to
//     them.
//
// The HTTP client can be consumed by declaring a dependency on *http.Client.
// This can be done by adding *http.Client as a parameter to one of your
// constructors like so.
//
//   func newHandler(client *http.Client) *requestHandler {
//     // ...
//   }
//
// This dependency can also be declared by adding *http.Client as a field in
// an Fx parameter object.
//
//   type handlerParams struct {
//     fx.In
//
//     HTTPClient *http.Client
//     ...
//   }
//
//   func newHandler(p handlerParams) *requestHandler {
//     // ...
//   }
//
// Make sure to include httpfx.ClientModule in your Fx application.
//
//   fx.New(
//     httpfx.ClientModule,
//     fx.Provide(
//       newHandler,
//       ..
//     ),
//   ),
//
// Propagating Context
//
// For Galileo and Jaeger to work correctly, HTTP clients MUST propagate
// context from incoming HTTP and YARPC requests to outgoing HTTP and YARPC
// requests.
//
// For incoming HTTP requests, the context is attached to the http.Request and
// is accessible by using the Context() method.
//
//   func (h *myHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
//     ctx := r.Context()
//     res, err := h.keyValueClient.GetValue(ctx, ...)
//     ...
//   }
//
// To specify the context for outgoing HTTP requests, build an http.Request
// with http.NewRequest and attach the context to it by using the WithContext
// method.
//
//   req, err := http.NewRequest(...)
//   if err != nil {
//     ...
//   }
//   req = req.WithContext(ctx)
//
// This can get repetitive so we recommend using the
// https://golang.org/x/net/context/ctxhttp package with the http.Client.
//
//   import "golang.org/x/net/context/ctxhttp"
//
//   func (h *myHandler) GetValue(ctx context.Context, req *GetValueRequest) (*GetValueResponse, error) {
//     url := ...
//     res, err := ctxhttp.Get(ctx, h.httpClient, url)
//     ...
//   }
//
// Installing Client Middleware
//
// Client middleware is defined as any function with the signature,
//
//   func(http.RoundTripper) http.RoundTripper
//
// If you need to install your own middleware for HTTP clients, you can do so
// by using the InstrumentClient function exported by httpfx.ClientModule.
// This function has the signature,
//
//   func(client *http.Client, middlewares ...func(http.RoundTripper) http.RoundTripper)
//
// This function will instrument any http.Client with the same functionality
// as the http.Client produced by httpfx, along with the provided middleware
// installed into it.
//
//   type fooClientParams struct {
//     fx.In
//
//     // Provided by httpfx.ClientModule.
//     InstrumentClient func(*http.Client, ...func(http.RoundTripper) http.RoundTripper)
//     ...
//   }
//
//   func newFooClient(p fooClientParams) *FooClient {
//     client := &http.Client{}
//     // ...
//
//     p.InstrumentClient(client, loggingMiddleware, rateLimitingMiddleware)
//     return &FooClient{httpClient: client}
//   }
//
// Starting an HTTP Server
//
// (See the Examples section for a full example of setting up an HTTP server
// with Jaeger and Galileo support.)
//
// To start an HTTP server in an Fx application, build a *http.Server with the
// desired configuration and start it with an httpfx.git/httpserver.Handle in
// an fx.Invoke.
//
//   // import "code.uber.internal/go/httpfx.git/httpserver"
//
//   fx.Invoke(func(server *http.Server, lc fx.Lifecycle) {
//     handle := httpserver.NewHandle(server)
//     lc.Append(fx.Hook{
//       OnStart: handle.Start,
//       OnStop: handle.Shutdown,
//     })
//   })
//
// See
// https://go.uberinternal.com/pkg/code.uber.internal/go/httpfx.git/httpserver
// for more information on the httpserver package.
//
// Note that this DOES NOT give you Galileo or Jaeger support. Read the
// following section for more information.
//
// Galileo and Jaeger Support for Servers
//
// To add Galileo and Jaeger support for your HTTP server, you need to wrap
// your http.Handler with Galileo and Jaeger middleware. An HTTP handler
// middleware is any function in the form,
//
//   func(http.Handler) http.Handler
//
// The Galileo middleware is provided by galileofx under the name "auth" and
// the Jaeger middleware is provided by jaegerfx under the name "trace".
// These may be consumed inside an Fx parameter object like so.
//
//   type serverParams struct {
//     fx.In
//
//     // When installing the middleware, note that the Jaeger middleware MUST
//     // run before Galileo.
//     WrapJaeger  func(http.Handler) http.Handler `name:"trace"`
//     WrapGalileo func(http.Handler) http.Handler `name:"auth"`
//     ...
//   }
//
// See the Examples section for an example of installing this middleware.
//
// See Also
//
// https://go.uberinternal.com/pkg/code.uber.internal/go/httpfx.git/clientmiddleware/
// https://go.uberinternal.com/pkg/code.uber.internal/go/httpfx.git/servermiddleware/
// https://go.uberinternal.com/pkg/code.uber.internal/go/galileofx.git/
// https://go.uberinternal.com/pkg/code.uber.internal/go/jaegerfx.git/
package httpfx // import "code.uber.internal/go/httpfx.git"

const (
	// Version of the client module.
	Version = "2.0.0"

	_name = "httpfx"
)
