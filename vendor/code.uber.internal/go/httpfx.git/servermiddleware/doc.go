// Package servermiddleware provides tools to help write HTTP server
// middleware. HTTP server middleware is defined as any function with the
// signature,
//
//   func(http.Handler) http.Handler
//
// Multiple such middlewares may be grouped together with the use of the Chain
// function.
package servermiddleware // import "code.uber.internal/go/httpfx.git/servermiddleware"
