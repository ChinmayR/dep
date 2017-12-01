package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeRequest(c *Coordinator, method, checkType, service string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/", nil)

	query := req.URL.Query()
	query.Set("service", service)
	query.Set("type", checkType)
	req.URL.RawQuery = query.Encode()

	res := httptest.NewRecorder()
	NewPlain(c).ServeHTTP(res, req)
	return res
}

func assertHeaders(t testing.TB, expected State, service string, res *httptest.ResponseRecorder) {
	assert.Equal(
		t,
		strings.ToUpper(expected.String()),
		res.HeaderMap.Get("Health-Status"),
		"Unexpected health status header.",
	)
	assert.Equal(
		t,
		service,
		res.HeaderMap.Get("Health-Service"),
		"Unexpected health service header.",
	)
}

func TestPlainNagios(t *testing.T) {
	c := NewCoordinator("foo")
	defer c.cleanup()
	c.AcceptTraffic()

	run := func(t testing.TB, method string, checkBody bool) {
		res := makeRequest(c, method, "", "")
		assert.Equal(t, http.StatusOK, res.Code, "Expected 200 when accepting.")
		assertHeaders(t, AcceptingTraffic, "foo", res)
		if checkBody {
			assert.Equal(t, "OK", res.Body.String(), "Unexpected body.")
		}

		// Wrong service name.
		res = makeRequest(c, method, "", "bar")
		assert.Equal(t, http.StatusForbidden, res.Code, "Expected 403 on name mismatch.")
		assertHeaders(t, AcceptingTraffic, "foo", res)
		if checkBody {
			assert.Equal(
				t,
				`check target was service "bar", but this is "foo"`,
				res.Body.String(),
				"Unexpected body.",
			)
		}

		c.RefuseTraffic()
		defer c.AcceptTraffic()

		res = makeRequest(c, method, "", "")
		assert.Equal(t, http.StatusOK, res.Code, "Expected 200 from job health when refusing.")
		assertHeaders(t, RefusingTraffic, "foo", res)
		if checkBody {
			assert.Equal(t, "OK", res.Body.String(), "Unexpected body.")
		}
	}

	t.Run("get", func(t *testing.T) {
		run(t, http.MethodGet, true)
	})

	t.Run("head", func(t *testing.T) {
		run(t, http.MethodHead, false)
	})

	t.Run("trace", func(t *testing.T) {
		res := makeRequest(c, http.MethodTrace, "", "")
		assert.Equal(t, http.StatusMethodNotAllowed, res.Code, "Expected to only allow GET and HEAD.")
	})
}

func TestPlainHealthController(t *testing.T) {
	c := NewCoordinator("foo")
	defer c.cleanup()
	c.AcceptTraffic()

	run := func(t testing.TB, method string, checkBody bool) {
		res := makeRequest(c, method, "traffic", "")
		assert.Equal(t, http.StatusOK, res.Code, "Expected 200 when accepting.")
		assertHeaders(t, AcceptingTraffic, "foo", res)
		if checkBody {
			assert.Equal(t, "ACCEPTING\n", res.Body.String(), "Unexpected body.")
		}

		// Wrong service name.
		res = makeRequest(c, method, "traffic", "bar")
		assert.Equal(t, http.StatusForbidden, res.Code, "Expected 403 on name mismatch.")
		assertHeaders(t, AcceptingTraffic, "foo", res)
		if checkBody {
			assert.Equal(
				t,
				`check target was service "bar", but this is "foo"`,
				res.Body.String(),
				"Unexpected body.",
			)
		}

		c.RefuseTraffic()
		defer c.AcceptTraffic()

		res = makeRequest(c, method, "traffic", "")
		assert.Equal(t, http.StatusNotFound, res.Code, "Expected 404 from RPC health when refusing.")
		assertHeaders(t, RefusingTraffic, "foo", res)
		if checkBody {
			assert.Equal(t, "REFUSING\n", res.Body.String(), "Unexpected body.")
		}
	}

	t.Run("get", func(t *testing.T) {
		run(t, http.MethodGet, true)
	})

	t.Run("head", func(t *testing.T) {
		run(t, http.MethodHead, false)
	})

	t.Run("trace", func(t *testing.T) {
		res := makeRequest(c, http.MethodTrace, "traffic", "")
		assert.Equal(t, http.StatusMethodNotAllowed, res.Code, "Expected to only allow GET and HEAD.")
	})
}

func TestPlainClient(t *testing.T) {
	c := NewCoordinator("foo")
	defer c.cleanup()
	c.AcceptTraffic()

	server := httptest.NewServer(NewPlain(c))
	defer server.Close()

	t.Run("Success", func(t *testing.T) {
		client := NewPlainClient(server.URL)
		res, err := client.Health(context.Background())
		require.NoError(t, err, "Failed to check health.")
		assert.True(t, res.JobHealth, "Unexpected job health.")
		assert.Equal(t, AcceptingTraffic, res.RPCHealth, "Unexpected RPC health.")
	})

	t.Run("RequestFailure", func(t *testing.T) {
		client := NewPlainClient("http://localhost:1234/health")
		_, err := client.Health(context.Background())
		require.Error(t, err, "Expected error calling non-existent endpoint.")
		assert.Contains(t, err.Error(), "request failed", "Unexpected error.")
	})

	t.Run("WrongService", func(t *testing.T) {
		client := NewPlainClient(server.URL + "/?service=bar")
		_, err := client.Health(context.Background())
		require.Error(t, err, "Expected error health-checking wrong service.")
		assert.Contains(t, err.Error(), "non-200 status code", "Unexpected error.")
	})
}
