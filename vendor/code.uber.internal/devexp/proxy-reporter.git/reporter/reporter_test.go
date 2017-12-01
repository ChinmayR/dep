package reporter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/uber-go/tally"

	"code.uber.internal/devexp/proxy-reporter.git/types"
	"code.uber.internal/engsec/wonka-go.git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockClaimer func() (*wonka.Claim, error)

func (m mockClaimer) ClaimResolveTTL(context.Context, string, time.Duration) (*wonka.Claim, error) {
	return m()
}

func TestNew(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	r, err := New(
		"test",
		WithSample(1.0),
		WithClaimer(mockClaimer(func() (*wonka.Claim, error) {
			return &wonka.Claim{}, nil
		})),
		WithAddress(srv.URL),
	)

	require.NoError(t, err)
	r.Flush()

	assert.True(t, r.Capabilities().Reporting())
	assert.True(t, r.Capabilities().Tagging())
}

func TestReportingValues(t *testing.T) {
	t.Parallel()

	c := make(chan string, 15)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		for _, v := range r.Form["counter"] {
			c <- v
		}
		for _, v := range r.Form["gauge"] {
			c <- v
		}
		for _, v := range r.Form["timer"] {
			c <- v
		}
	}))

	require.NotNil(t, srv)

	r, err := New(
		"test",
		WithAddress(srv.URL),
		WithSample(1.0),
		WithClaimer(mockClaimer(func() (*wonka.Claim, error) {
			return &wonka.Claim{}, nil
		})),
	)
	require.NoError(t, err)

	r.ReportCounter("someCounter", map[string]string{"some": "tag"}, 10)
	r.ReportGauge("someGauge", map[string]string{"another": "tag"}, 33.0)
	r.ReportTimer("someTimer", map[string]string{"batman": "robin"}, 2*time.Second)

	r.Flush()
	require.Equal(t, 4, len(c))

	var boot types.Counter
	require.NoError(t, json.Unmarshal([]byte(<-c), &boot))
	assert.Equal(t, types.Counter{
		Identity: types.Identity{
			Name: "boot",
			Tags: map[string]string{
				"version": types.Version,
				"tool":    "test",
			},
		},
		Value: 1,
	}, boot)

	var cnt types.Counter
	require.NoError(t, json.Unmarshal([]byte(<-c), &cnt))
	assert.Equal(t, types.Counter{
		Identity: types.Identity{
			Name: "someCounter",
			Tags: map[string]string{
				"version": types.Version,
				"tool":    "test",
				"some":    "tag",
			},
		},
		Value: 10,
	}, cnt)

	var gg types.Gauge
	require.NoError(t, json.Unmarshal([]byte(<-c), &gg))
	assert.Equal(t, types.Gauge{
		Identity: types.Identity{
			Name: "someGauge",
			Tags: map[string]string{
				"version": types.Version,
				"tool":    "test",
				"another": "tag",
			},
		},
		Value: 33,
	}, gg)

	var tmr types.Timer
	require.NoError(t, json.Unmarshal([]byte(<-c), &tmr))
	assert.Equal(t, types.Timer{
		Identity: types.Identity{
			Name: "someTimer",
			Tags: map[string]string{
				"version": types.Version,
				"tool":    "test",
				"batman":  "robin",
			},
		},
		Interval: 2 * time.Second,
	}, tmr)

	assert.NotPanics(t, r.Flush)
}

func TestClaimError(t *testing.T) {
	t.Parallel()

	_, err := New(
		"test",
		WithSample(1.0),
		WithClaimer(mockClaimer(func() (*wonka.Claim, error) {
			return nil, errors.New("boom")
		})))

	assert.EqualError(t, err, "boom")
}

func TestSample(t *testing.T) {
	t.Parallel()

	t.Run("backend", func(t *testing.T) {
		r, err := New(
			"test",
			WithSample(1.0),
			WithClaimer(mockClaimer(func() (*wonka.Claim, error) {
				return nil, nil
			})))

		require.NoError(t, err)
		_, ok := r.(*backend)
		assert.True(t, ok, "expect a backend for 1.0 sample rate")
	})

	t.Run("nullreporter", func(t *testing.T) {
		r, err := New(
			"test",
			WithSample(0.0),
			WithClaimer(mockClaimer(func() (*wonka.Claim, error) {
				return nil, nil
			})))

		require.NoError(t, err)
		assert.Equal(t, tally.NullStatsReporter, r)
	})
}

func TestReportingHistograms(t *testing.T) {
	t.Parallel()

	c := make(chan string, 15)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		for _, v := range r.Form[types.HDurationKey] {
			c <- v
		}
		for _, v := range r.Form[types.HValueKey] {
			c <- v
		}
	}))

	require.NotNil(t, srv)

	r, err := New(
		"test",
		WithAddress(srv.URL),
		WithSample(1.0),
		WithClaimer(mockClaimer(func() (*wonka.Claim, error) {
			return &wonka.Claim{}, nil
		})),
	)
	require.NoError(t, err)

	r.ReportHistogramValueSamples(
		"values",
		map[string]string{"second": "2"},
		tally.ValueBuckets{0.0, 1.0, 10.0},
		0.0,
		1.0,
		2,
	)

	r.ReportHistogramDurationSamples(
		"durations",
		map[string]string{"second": "1s"},
		tally.DurationBuckets{time.Millisecond, time.Second, time.Minute},
		0,
		time.Second,
		4,
	)

	r.Flush()
	require.Equal(t, 2, len(c))

	var hd types.HistogramDuration
	require.NoError(t, json.Unmarshal([]byte(<-c), &hd))
	assert.Equal(t, types.HistogramDuration{
		Identity: types.Identity{
			Name: "durations",
			Tags: map[string]string{
				"version": types.Version,
				"tool":    "test",
				"second":  "1s",
			},
		},
		Values:     []time.Duration{time.Millisecond, time.Second, time.Minute},
		UpperBound: time.Second,
		Samples:    4,
	}, hd)

	var hv types.HistogramValue
	require.NoError(t, json.Unmarshal([]byte(<-c), &hv))
	assert.Equal(t, types.HistogramValue{
		Identity: types.Identity{
			Name: "values",
			Tags: map[string]string{
				"version": types.Version,
				"tool":    "test",
				"second":  "2",
			},
		},
		Values:     []float64{0.0, 1.0, 10.0},
		UpperBound: 1.0,
		Samples:    2,
	}, hv)
}
