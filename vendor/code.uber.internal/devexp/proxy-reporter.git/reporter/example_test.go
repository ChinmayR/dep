// +build ussh

package reporter_test

import (
	"log"
	"math/rand"
	"time"

	"code.uber.internal/devexp/proxy-reporter.git/reporter"
	"github.com/uber-go/tally"
)

// This is an example test one can run locally to check
// their metrics are being reported:
//
// `go test -v -tags ussh`
//
// If ussh is fresh, it will report successfully, otherwise
// it will fail with an empty wonka claim error.
func ExampleNew() {
	r, err := reporter.New("test", reporter.WithSample(1.0))
	if err != nil {
		log.Fatalf("failed to create a reporter %v", err)
	}

	s, c := tally.NewRootScope(tally.ScopeOptions{
		Reporter: r,
		Tags:     map[string]string{"function": "ExampleNew"},
	}, time.Second)

	defer c.Close()

	rand.Seed(time.Now().Unix())
	s.Counter("counter").Inc(rand.Int63n(42))
	s.Timer("timer").Record(time.Duration(rand.Int63n(13)) * time.Second)
	s.Gauge("gauge").Update(rand.Float64())

	s.Histogram(
		"duration_histogram",
		tally.DurationBuckets{0, time.Millisecond, 500 * time.Millisecond, time.Second, 30 * time.Second, time.Minute},
	).RecordDuration(time.Duration(rand.Int63n(60000)) * time.Millisecond)

	s.Histogram(
		"value_histogram",
		tally.ValueBuckets{0, 0.1, 1.0, 10.0, 100.0, 1000.0},
	).RecordValue(rand.ExpFloat64())

	// Output:
}

func ExampleNewScope() {
	s, c := reporter.NewScope("test", time.Second)
	s.Counter("run").Inc(1)

	if err := c.Close(); err != nil {
		log.Fatalf("error closing scope: %v", err)
	}
	// Output:
}
