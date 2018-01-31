// Package clientmiddleware provides tools to help write HTTP client
// middleware. HTTP client middleware is defined as any function with the
// signature,
//
//   func(http.RoundTripper) http.RoundTripper
//
// Multiple such middlewares may be grouped together with the use of the Chain
// function.
package clientmiddleware
