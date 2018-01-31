package httpserver

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockDialer is a mock implementation of the dialer interface.
type MockDialer struct {
	ctrl     *gomock.Controller
	recorder *MockDialerRecorder
}

var _ dialer = (*MockDialer)(nil)

func NewMockDialer(ctrl *gomock.Controller) *MockDialer {
	m := &MockDialer{ctrl: ctrl}
	m.recorder = &MockDialerRecorder{m: m}
	return m
}

func (m *MockDialer) EXPECT() *MockDialerRecorder { return m.recorder }

func (m *MockDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	results := m.ctrl.Call(m, "DialContext", ctx, network, addr)
	conn, _ := results[0].(net.Conn)
	err, _ := results[1].(error)
	return conn, err
}

type MockDialerRecorder struct{ m *MockDialer }

func (r *MockDialerRecorder) DialContext(ctx interface{}, network interface{}, addr interface{}) *gomock.Call {
	return r.m.ctrl.RecordCall(r.m, "DialContext", ctx, network, addr)
}

func TestWaitUntilAvailableErrors(t *testing.T) {
	t.Run("dial error", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		dialer := NewMockDialer(mockCtrl)
		dialer.EXPECT().
			DialContext(gomock.Any(), "tcp", "127.0.0.1:8888").
			Return(nil, errors.New("great sadness"))

		ctx := context.Background()
		err := waitUntilAvailable(ctx, dialer, "127.0.0.1:8888")
		require.Error(t, err, "expected failure")
		assert.Contains(t, err.Error(), `failed to dial to "127.0.0.1:8888": great sadness`)
	})

	t.Run("dial timeout", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()

		dialer := NewMockDialer(mockCtrl)
		dialer.EXPECT().
			DialContext(ctx, "tcp", "127.0.0.1:8888").
			Return(nil, &net.DNSError{Err: "great sadness", IsTimeout: true})

		err := waitUntilAvailable(ctx, dialer, "127.0.0.1:8888")
		assert.Equal(t, context.DeadlineExceeded, err, "expected DeadlineExceeded error")
	})

	t.Run("write error", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		conn := NewMockConn(mockCtrl)
		dialer := NewMockDialer(mockCtrl)

		dialer.EXPECT().
			DialContext(gomock.Any(), "tcp", "127.0.0.1:8888").
			Return(conn, nil)
		conn.EXPECT().
			Write(gomock.Any()).
			Return(0, errors.New("great sadnesse"))
		conn.EXPECT().Close()

		ctx := context.Background()
		err := waitUntilAvailable(ctx, dialer, "127.0.0.1:8888")
		require.Error(t, err, "expected failure")
		assert.Contains(t, err.Error(), "failed to write request to server: great sadness")
	})

	t.Run("set deadline error", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		conn := NewMockConn(mockCtrl)
		dialer := NewMockDialer(mockCtrl)

		deadline := time.Now().Add(100 * time.Millisecond)

		dialer.EXPECT().
			DialContext(gomock.Any(), "tcp", "127.0.0.1:8888").
			Return(conn, nil)
		conn.EXPECT().
			SetDeadline(deadline).
			Return(errors.New("great sadness"))
		conn.EXPECT().Close()

		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()

		err := waitUntilAvailable(ctx, dialer, "127.0.0.1:8888")
		require.Error(t, err, "expected failure")

		assert.Contains(t, err.Error(), "failed to set connection deadline")
		assert.Contains(t, err.Error(), "great sadness")
	})

	t.Run("write timeout", func(t *testing.T) {
		ln, err := net.Listen("tcp", ":0")
		require.NoError(t, err, "failed to listen")
		defer func() {
			assert.NoError(t, ln.Close(), "failed to close listener")
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// We never accept the connection so the caller is never able to write
		// to it.

		var dialer net.Dialer
		err = waitUntilAvailable(ctx, &dialer, ln.Addr().String())
		assert.Equal(t, context.DeadlineExceeded, err)
	})

	t.Run("read error", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		conn := NewMockConn(mockCtrl)
		dialer := NewMockDialer(mockCtrl)

		dialer.EXPECT().
			DialContext(gomock.Any(), "tcp", "127.0.0.1:8888").
			Return(conn, nil)

		deadline := time.Now().Add(100 * time.Millisecond)

		conn.EXPECT().Write(gomock.Any()).Return(42, nil)
		conn.EXPECT().SetDeadline(deadline).Return(nil)
		conn.EXPECT().Close()

		conn.EXPECT().
			Read(gomock.Any()).
			Return(0, errors.New("great sadness"))

		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()

		err := waitUntilAvailable(ctx, dialer, "127.0.0.1:8888")
		require.Error(t, err, "expected failure")
		assert.Contains(t, err.Error(), "failed to read response from server: great sadness")
	})

	t.Run("read timeout", func(t *testing.T) {
		ln, err := net.Listen("tcp", ":0")
		require.NoError(t, err, "failed to listen")
		defer func() {
			assert.NoError(t, ln.Close(), "failed to close listener")
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		go func() {
			conn, err := ln.Accept()
			assert.NoError(t, err, "failed to accept connection")

			data := make([]byte, 1024)
			_, err = conn.Read(data)
			assert.NoError(t, err, "failed to read request")

			// We never write anything back so the caller will wait for read
			// forever.
		}()

		var dialer net.Dialer
		err = waitUntilAvailable(ctx, &dialer, ln.Addr().String())
		assert.Equal(t, context.DeadlineExceeded, err)
	})
}
