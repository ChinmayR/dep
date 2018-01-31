// Package authmiddleware provides Galileo YARPC middleware for authenticating
// inbound and outbound requests.
//
// Servers and clients built with yarpcfx will automatically have Galileo
// support. To manually instrument a YARPC application with Galileo support,
// build a Middleware with New and pass that into your yarpc.Config as the
// inbound and outbound unary and oneway middleware.
//
//   m := authmiddleware.New(g)
//   dispatcher := yarpc.New(yarpc.Config{
//     InboundMiddleware: yarpc.InboundMiddleware{
//       Unary: m,
//       Oneway: m,
//     },
//     OutboundMiddleware: yarpc.OutboundMiddleware{
//       Unary: m,
//       Oneway: m,
//     },
//   })
package authmiddleware // import "code.uber.internal/go/galileofx.git/authmiddleware"

import (
	"context"

	"code.uber.internal/engsec/galileo-go.git"
	"go.uber.org/yarpc/api/middleware"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/yarpcerrors"
)

//go:generate mockgen -source auth_middleware.go -destination mock_auth_middleware.go -package authmiddleware

type authenticator interface {
	AuthenticateOut(ctx context.Context, destination string, explicitClaim ...interface{}) (context.Context, error)
	AuthenticateIn(ctx context.Context, allowedEntities ...interface{}) error
}

// Compile-time check that our subset of the interface always matches what's
// being provided by the library.
var _ authenticator = (galileo.Galileo)(nil)

// Middleware is a middleware that authenticates outgoing YARPC requests and
// validates that incoming requests are authenticated.
type Middleware struct {
	g authenticator
}

// New constructs a new YARPC middleware for Galileo.
func New(g galileo.Galileo) *Middleware {
	return &Middleware{g: g}
}

var (
	_ middleware.UnaryOutbound  = (*Middleware)(nil)
	_ middleware.UnaryInbound   = (*Middleware)(nil)
	_ middleware.OnewayOutbound = (*Middleware)(nil)
	_ middleware.OnewayInbound  = (*Middleware)(nil)
)

func (m *Middleware) authOut(ctx context.Context, req *transport.Request) (context.Context, error) {
	// TODO(abg): Add support for explicit claims. Since explicit claims are
	// per-request, we can't attach them to the middleware object. Perhaps
	// we'll support putting them on the context.

	ctx, err := m.g.AuthenticateOut(ctx, req.Service)
	if err != nil {
		return nil, yarpcerrors.UnauthenticatedErrorf(
			"unable to authenticate request to procedure %q of service %q: %v", req.Procedure, req.Service, err)
	}
	return ctx, err
}

// Call implements YARPC's UnaryOutbound middleware interface.
func (m *Middleware) Call(ctx context.Context, req *transport.Request, out transport.UnaryOutbound) (*transport.Response, error) {
	ctx, err := m.authOut(ctx, req)
	if err != nil {
		return nil, err
	}
	return out.Call(ctx, req)
}

// CallOneway implements YARPC's OnewayOutbound middleware interface.
func (m *Middleware) CallOneway(ctx context.Context, req *transport.Request, out transport.OnewayOutbound) (transport.Ack, error) {
	ctx, err := m.authOut(ctx, req)
	if err != nil {
		return nil, err
	}
	return out.CallOneway(ctx, req)
}

func (m *Middleware) authIn(ctx context.Context, req *transport.Request) error {
	// TODO(abg): Per-procedure configuration should determine allowedEntities
	// here.
	if req != nil && req.Procedure == "Meta::health" {
		// We don't authenticate health checks.
		return nil
	}
	if err := m.g.AuthenticateIn(ctx); err != nil {
		return yarpcerrors.UnauthenticatedErrorf(
			"access denied to procedure %q of service %q: %v", req.Procedure, req.Service, err)
	}
	return nil
}

// Handle implements YARPC's UnaryInbound middleware interface.
func (m *Middleware) Handle(ctx context.Context, req *transport.Request, resw transport.ResponseWriter, h transport.UnaryHandler) error {
	if err := m.authIn(ctx, req); err != nil {
		return err
	}
	return h.Handle(ctx, req, resw)
}

// HandleOneway implements YARPC's OnewayInbound middleware interface.
func (m *Middleware) HandleOneway(ctx context.Context, req *transport.Request, h transport.OnewayHandler) error {
	if err := m.authIn(ctx, req); err != nil {
		return err
	}
	return h.HandleOneway(ctx, req)
}
