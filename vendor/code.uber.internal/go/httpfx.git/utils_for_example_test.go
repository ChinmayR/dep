package httpfx_test

import (
	"net/http"

	"go.uber.org/fx"
)

// Types and functions we want to use in example tests that we don't want to
// appear in the user-facing example itself should go here.

type fakeServerMiddleware struct {
	fx.Out

	WrapJaeger  func(http.Handler) http.Handler `name:"trace"`
	WrapGalileo func(http.Handler) http.Handler `name:"auth"`
}

func newFakeServerMiddleware() fakeServerMiddleware {
	return fakeServerMiddleware{}
}

var FakeModule = fx.Options(
	fx.NopLogger, // Don't make noise from example tests.
	fx.Provide(newFakeServerMiddleware),
)
