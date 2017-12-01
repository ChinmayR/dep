package handler

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"code.uber.internal/devexp/proxy-reporter.git/reporter"
	"code.uber.internal/devexp/proxy-reporter.git/types"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

type mockClaimer func() (*wonka.Claim, error)

func (m mockClaimer) ClaimResolveTTL(context.Context, string, time.Duration) (*wonka.Claim, error) {
	return m()
}

func TestHandler(t *testing.T) {
	t.Parallel()

	s := tally.NewTestScope("test", nil)
	h, err := New(s, zap.NewNop())
	require.NoError(t, err)

	srv := httptest.NewServer(h)
	r, err := reporter.New(
		"ut",
		reporter.WithAddress(srv.URL+"/tally"),
		reporter.WithSample(1.0),
		reporter.WithClaimer(mockClaimer(func() (*wonka.Claim, error) {
			return &wonka.Claim{}, nil
		})),
	)

	require.NoError(t, err)

	r.ReportCounter("counter", nil, 13)
	r.ReportGauge("gauge", map[string]string{"tag": "works"}, 42)
	r.ReportTimer("timer", nil, time.Second)
	r.ReportHistogramDurationSamples("durations", nil, tally.DurationBuckets{0, time.Second}, 0, time.Second, 2)
	r.ReportHistogramValueSamples("values", nil, tally.ValueBuckets{0, 1.0, 2.0}, 0, 1.0, 3)

	r.Flush()
	srv.Close()

	sh := s.Snapshot()

	t.Run("counters", func(t *testing.T) {
		c := sh.Counters()
		assert.Equal(t, 2, len(c))
		v := c["test.counter+tool=ut,version=0.1.3"]
		assert.Equal(t, "test.counter", v.Name())
		assert.Equal(t, map[string]string{"tool": "ut", "version": types.Version}, v.Tags())
		assert.Equal(t, int64(13), v.Value())
	})

	t.Run("gauge", func(t *testing.T) {
		c := sh.Gauges()
		assert.Equal(t, 1, len(c))
		v := c["test.gauge+tag=works,tool=ut,version=0.1.3"]
		assert.Equal(t, "test.gauge", v.Name())
		assert.Equal(t,
			map[string]string{
				"tool":    "ut",
				"tag":     "works",
				"version": types.Version,
			},
			v.Tags())

		assert.Equal(t, float64(42), v.Value())
	})

	t.Run("timer", func(t *testing.T) {
		c := sh.Timers()
		assert.Equal(t, 1, len(c))
		v := c["test.timer+tool=ut,version=0.1.3"]
		assert.Equal(t, "test.timer", v.Name())
		assert.Equal(t, map[string]string{"tool": "ut", "version": types.Version}, v.Tags())
		assert.Equal(t, []time.Duration{time.Second}, v.Values())
	})

	t.Run("histograms", func(t *testing.T) {
		h := sh.Histograms()
		assert.Equal(t, 2, len(h))

		d := h["test.durations+tool=ut,version=0.1.3"]
		assert.Equal(t, "test.durations", d.Name())
		assert.Equal(t, map[string]string{"tool": "ut", "version": types.Version}, d.Tags())
		assert.Equal(t, map[time.Duration]int64{0: 0, time.Second: 2, time.Duration(math.MaxInt64): 0}, d.Durations())

		v := h["test.values+tool=ut,version=0.1.3"]
		assert.Equal(t, "test.values", v.Name())
		assert.Equal(t, map[string]string{"tool": "ut", "version": types.Version}, v.Tags())
		assert.Equal(t, map[float64]int64{0.0: 0, 1.0: 3, 2.0: 0, math.MaxFloat64: 0}, v.Values())
	})
}

func TestHandlerErrors(t *testing.T) {
	t.Parallel()

	t.Run("tool cookie not found", func(t *testing.T) {
		withSetup(t,
			func(r tally.StatsReporter, url string) {
				rsp, err := http.Get(url)
				assert.NoError(t, err)
				assert.Equal(t, http.StatusBadRequest, rsp.StatusCode)
			},
			func(s tally.Snapshot) {
				c := s.Counters()
				assert.Equal(t, 2, len(c))
				v := c["test.error+path=cookie"]
				assert.Equal(t, "test.error", v.Name())
				assert.Equal(t, map[string]string{"path": "cookie"}, v.Tags())
				assert.Equal(t, int64(1), v.Value())
			})
	})

	t.Run("form parse error", func(t *testing.T) {
		withSetup(t,
			func(r tally.StatsReporter, url string) {
				req, err := http.NewRequest(http.MethodPost, url, strings.NewReader("%HI"))
				require.NoError(t, err)

				req.AddCookie(&http.Cookie{Name: "tool", Value: "hammer"})
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

				rsp, err := http.DefaultClient.Do(req)
				assert.NoError(t, err)
				assert.Equal(t, http.StatusBadRequest, rsp.StatusCode)
			},
			func(s tally.Snapshot) {
				c := s.Counters()
				assert.Equal(t, 2, len(c))
				v := c["test.error+path=form parse,tool=hammer"]
				assert.Equal(t, "test.error", v.Name())
				assert.Equal(t, map[string]string{"tool": "hammer", "path": "form parse"}, v.Tags())
				assert.Equal(t, int64(1), v.Value())
			})
	})
}

func withSetup(t *testing.T, action func(r tally.StatsReporter, url string), verify func(s tally.Snapshot)) {
	s := tally.NewTestScope("test", nil)
	h, err := New(s, zap.NewNop())
	require.NoError(t, err)

	srv := httptest.NewServer(h)
	r, err := reporter.New(
		"ut",
		reporter.WithAddress(srv.URL+"/tally"),
		reporter.WithSample(1.0),
		reporter.WithClaimer(mockClaimer(func() (*wonka.Claim, error) {
			return &wonka.Claim{}, nil
		})),
	)
	require.NoError(t, err)

	action(r, srv.URL+"/tally")

	r.Flush()
	srv.Close()

	verify(s.Snapshot())
}
