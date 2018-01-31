package authmiddleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"code.uber.internal/engsec/galileo-go.git/galileotest"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/api/transport/transporttest"
	"go.uber.org/yarpc/yarpcerrors"
)

func TestYARPCMiddlewareUnaryOutbound(t *testing.T) {
	type ctxKey string

	t.Run("authentication success", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		g := galileotest.NewMockGalileo(mockCtrl)
		out := transporttest.NewMockUnaryOutbound(mockCtrl)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		authCtx := context.WithValue(ctx, ctxKey("foo"), "bar") // to get a different context

		req := &transport.Request{Service: "myservice"}
		res := &transport.Response{}

		g.EXPECT().AuthenticateOut(ctx, "myservice").Return(authCtx, nil)
		out.EXPECT().Call(authCtx, req).Return(res, nil)

		mw := New(g)
		gotRes, err := mw.Call(ctx, req, out)
		require.NoError(t, err, "expected success")
		assert.True(t, res == gotRes, "response must match")
	})

	t.Run("authentication failure", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		g := galileotest.NewMockGalileo(mockCtrl)
		out := transporttest.NewMockUnaryOutbound(mockCtrl)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		g.EXPECT().AuthenticateOut(ctx, "someservice").
			Return(nil, errors.New("can't touch this"))

		mw := New(g)
		_, err := mw.Call(ctx, &transport.Request{
			Service:   "someservice",
			Procedure: "KeyValue::setValue",
		}, out)
		require.Error(t, err, "expected failure")
		assert.Contains(t, err.Error(),
			`unable to authenticate request to procedure "KeyValue::setValue" of service "someservice": can't touch this`)
	})
}

func TestYARPCMiddlewareUnaryInbound(t *testing.T) {
	t.Run("authentication success", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		g := galileotest.NewMockGalileo(mockCtrl)
		handler := transporttest.NewMockUnaryHandler(mockCtrl)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		req := &transport.Request{Service: "myservice"}

		g.EXPECT().AuthenticateIn(ctx).Return(nil)
		handler.EXPECT().Handle(ctx, req, gomock.Any()).Return(nil)

		mw := New(g)
		err := mw.Handle(ctx, req, &transporttest.FakeResponseWriter{}, handler)
		require.NoError(t, err, "expected success")
	})

	t.Run("authentication failure", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		g := galileotest.NewMockGalileo(mockCtrl)
		handler := transporttest.NewMockUnaryHandler(mockCtrl)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		g.EXPECT().AuthenticateIn(ctx).Return(errors.New("great sadness"))

		mw := New(g)
		err := mw.Handle(ctx, &transport.Request{
			Service:   "someservice",
			Procedure: "KeyValue::setValue",
		}, &transporttest.FakeResponseWriter{}, handler)
		require.Error(t, err, "expected failure")
		status := yarpcerrors.FromError(err)
		assert.Contains(t, status.Message(),
			`access denied to procedure "KeyValue::setValue" of service "someservice": great sadness`)
		assert.Equal(t, yarpcerrors.CodeUnauthenticated, status.Code(), "error code must match")
	})

	t.Run("health check exempt", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		g := galileotest.NewMockGalileo(mockCtrl)
		healthCheckHandler := transporttest.NewMockUnaryHandler(mockCtrl)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		req := &transport.Request{
			Service:   "someservice",
			Procedure: "Meta::health",
		}
		// Shouldn't attempt to use Galileo to authenticate health checks, so no
		// Galileo expectations required.
		healthCheckHandler.EXPECT().Handle(ctx, req, gomock.Any()).Return(nil)

		mw := New(g)
		err := mw.Handle(ctx, req, &transporttest.FakeResponseWriter{}, healthCheckHandler)
		require.NoError(t, err, "expected success")
	})

}

type ack struct{}

func (ack) String() string { return "ack" }

func TestYARPCMiddlewareOnewayOutbound(t *testing.T) {
	type ctxKey string

	t.Run("authentication success", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		g := galileotest.NewMockGalileo(mockCtrl)
		out := transporttest.NewMockOnewayOutbound(mockCtrl)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		authCtx := context.WithValue(ctx, ctxKey("foo"), "bar") // to get a different context
		req := &transport.Request{Service: "myservice"}

		g.EXPECT().AuthenticateOut(ctx, "myservice").Return(authCtx, nil)
		out.EXPECT().CallOneway(authCtx, req).Return(ack{}, nil)

		mw := New(g)
		_, err := mw.CallOneway(ctx, req, out)
		require.NoError(t, err, "expected success")
	})

	t.Run("authentication failure", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		g := galileotest.NewMockGalileo(mockCtrl)
		out := transporttest.NewMockOnewayOutbound(mockCtrl)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		g.EXPECT().AuthenticateOut(ctx, "someservice").
			Return(nil, errors.New("can't touch this"))

		mw := New(g)
		_, err := mw.CallOneway(ctx, &transport.Request{
			Service:   "someservice",
			Procedure: "KeyValue::setValue",
		}, out)
		require.Error(t, err, "expected failure")
		assert.Contains(t, err.Error(),
			`unable to authenticate request to procedure "KeyValue::setValue" of service "someservice": can't touch this`)
	})
}

func TestYARPCMiddlewareOnewayInbound(t *testing.T) {
	t.Run("authentication success", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		g := galileotest.NewMockGalileo(mockCtrl)
		handler := transporttest.NewMockOnewayHandler(mockCtrl)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		req := &transport.Request{Service: "myservice"}

		g.EXPECT().AuthenticateIn(ctx).Return(nil)
		handler.EXPECT().HandleOneway(ctx, req).Return(nil)

		mw := New(g)
		err := mw.HandleOneway(ctx, req, handler)
		require.NoError(t, err, "expected success")
	})

	t.Run("authentication failure", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		g := galileotest.NewMockGalileo(mockCtrl)
		handler := transporttest.NewMockOnewayHandler(mockCtrl)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		g.EXPECT().AuthenticateIn(ctx).Return(errors.New("great sadness"))

		mw := New(g)
		err := mw.HandleOneway(ctx, &transport.Request{
			Service:   "someservice",
			Procedure: "KeyValue::setValue",
		}, handler)
		require.Error(t, err, "expected failure")
		status := yarpcerrors.FromError(err)
		assert.Contains(t, status.Message(),
			`access denied to procedure "KeyValue::setValue" of service "someservice": great sadness`)
		assert.Equal(t, yarpcerrors.CodeUnauthenticated, status.Code(), "error code must match")
	})
}
