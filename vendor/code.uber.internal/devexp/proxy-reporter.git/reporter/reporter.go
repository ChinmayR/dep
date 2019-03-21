package reporter

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"

	"code.uber.internal/devexp/proxy-reporter.git/types"
	"github.com/uber-go/tally"
	"golang.org/x/net/publicsuffix"
)

type metric struct {
	name  string
	value string
}

// backend implements tally.StatsReporter interface.
// Constructor creates a go routine, that periodically
// collects metrics from the metrics channel and posts
// them via client.
type backend struct {
	metrics    chan *metric
	ticker     *time.Ticker
	startFlush chan struct{}
	flushDone  chan struct{}

	client *http.Client
	token  string
	addr   string
	tool   string
}

const (
	// Number of retries for posting data.
	_retryCount = 5
)

// New returns a new reporter.
func New(tool string, opt ...Option) (tally.StatsReporter, error) {
	c := defaultCfg
	for _, o := range opt {
		o.apply(&c)
	}

	s := rand.NewSource(time.Now().Unix())
	if rand.New(s).Float64() < c.sample {
		return newBackend(tool, c, make(chan *metric, c.count))
	}

	return tally.NullStatsReporter, nil
}

// NewScope is a helper to return a tally.Scope based on a reporter from New.
// If New fails to create a reporter(e.g. ussh certificate is not fresh),
// it falls back to tally.NoopScope.
func NewScope(tool string, interval time.Duration) (tally.Scope, io.Closer) {
	r, err := New(tool, WithInterval(interval))
	if err != nil {
		return tally.NoopScope, ioutil.NopCloser(nil)
	}

	return tally.NewRootScope(tally.ScopeOptions{Reporter: r}, interval)
}

// newBackend creates a backend that is listening the metric channel
// in a separate go routine.
func newBackend(tool string, c cfg, metrics chan *metric) (tally.StatsReporter, error) {
	token, err := c.ussoAuth.GetTokenForDomain("proxyreporter")
	if err != nil {
		return nil, err
	}

	jar := mustBake()
	jar.SetCookies(c.addr, []*http.Cookie{
		{
			Name:  "tool",
			Value: tool,
		},
		{
			Name:  "version",
			Value: types.Version,
		},
	})

	b := &backend{
		metrics:    metrics,
		client:     &http.Client{Timeout: 30 * time.Second, Jar: jar},
		addr:       c.addr.String(),
		token:      token,
		ticker:     time.NewTicker(c.interval),
		startFlush: make(chan struct{}),
		flushDone:  make(chan struct{}),
		tool:       tool,
	}

	// Collect and send all the metrics in background.
	go b.send()

	b.ReportCounter(
		"boot",
		map[string]string{
			"version": types.Version,
			"tool":    tool,
		},
		1,
	)

	return b, nil
}

// send collects metrics in an infinite loop and returns
// when signal received from the close channel.
func (b *backend) send() {
	for {
		select {
		case <-b.ticker.C:
			b.postHTTP(collectMetrics(b.metrics))
		case <-b.startFlush:
			b.postHTTP(collectMetrics(b.metrics))
			b.flushDone <- struct{}{}
		}
	}
}

// Flushes metrics from the channel to url.Values.
// There must be only one caller of collectMetrics on a single channel.
func collectMetrics(m <-chan *metric) url.Values {
	l := len(m)
	if l == 0 {
		return nil
	}

	values := make(url.Values, l)
	for i := 0; i < l; i++ {
		x := <-m
		values[x.name] = append(values[x.name], x.value)
	}

	return values
}

// post metrics as url.Values and report unsuccessful requests.
func (b *backend) postHTTP(values url.Values) {
	if len(values) == 0 {
		return
	}

	for i := 0; i < _retryCount; i++ {
		req, err := http.NewRequest("POST", b.addr, strings.NewReader(values.Encode()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating POST request: (%v), no retries remain\n", err)
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Authorization", "Bearer "+b.token)
		rsp, err := b.client.Do(req)
		if err != nil {
			if i < _retryCount-1 {
				delay := int(math.Pow(float64(i), 2))
				time.Sleep(time.Duration(delay) * time.Second)
			} else {
				fmt.Fprintf(os.Stderr, "Error posting request: (%v), no retries remain\n", err)
			}
			continue
		}

		if err := rsp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing response body: %v\n", err)
		}

		if rsp.StatusCode < http.StatusBadRequest {
			return
		}

		fmt.Fprintf(os.Stderr, "Received bad response: %s\n", rsp.Status)
	}
}

// Capabilities returns reporter's capabilities: can actively report and tag.
func (b *backend) Capabilities() tally.Capabilities {
	return capabilities{}
}

// Flush collects current metrics from a channel
// and sends them to the mothership.
func (b *backend) Flush() {
	b.startFlush <- struct{}{}
	<-b.flushDone
}

func makeIdentity(tool, name string, tags map[string]string) types.Identity {
	t := copyMap(tags)
	t["tool"] = tool
	t["version"] = types.Version

	return types.Identity{
		Name: name,
		Tags: t,
	}
}

// ReportCounter reports a counter value.
func (b *backend) ReportCounter(
	name string,
	tags map[string]string,
	value int64,
) {
	j, err := json.Marshal(types.Counter{
		Identity: makeIdentity(b.tool, name, tags),
		Value:    value,
	})

	if err != nil {
		panic(fmt.Errorf("failed to marshal counter: %v", err))
	}

	b.metrics <- &metric{
		name:  types.CounterKey,
		value: string(j),
	}
}

// ReportGauge reports a gauge value.
func (b *backend) ReportGauge(
	name string,
	tags map[string]string,
	value float64,
) {
	j, err := json.Marshal(types.Gauge{
		Identity: makeIdentity(b.tool, name, tags),
		Value:    value,
	})

	if err != nil {
		panic(fmt.Errorf("failed to marshal gauge: %v", err))
	}

	b.metrics <- &metric{
		name:  types.GaugeKey,
		value: string(j),
	}
}

// ReportTimer reports a timer value.
func (b *backend) ReportTimer(
	name string,
	tags map[string]string,
	interval time.Duration,
) {
	j, err := json.Marshal(types.Timer{
		Identity: makeIdentity(b.tool, name, tags),
		Interval: interval,
	})

	if err != nil {
		panic(fmt.Errorf("failed to marshal timer: %v", err))
	}

	b.metrics <- &metric{
		name:  types.TimerKey,
		value: string(j),
	}
}

// ReportHistogramValueSamples reports histogram samples for a bucket.
func (b *backend) ReportHistogramValueSamples(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
	bucketLowerBound,
	bucketUpperBound float64,
	samples int64,
) {
	j, err := json.Marshal(types.HistogramValue{
		Identity:   makeIdentity(b.tool, name, tags),
		Values:     buckets.AsValues(),
		LowerBound: bucketLowerBound,
		UpperBound: bucketUpperBound,
		Samples:    samples,
	})

	if err != nil {
		panic(fmt.Errorf("failed to marshal histogram value: %v", err))
	}

	b.metrics <- &metric{
		name:  types.HValueKey,
		value: string(j),
	}
}

// ReportHistogramDurationSamples reports histogram samples for a bucket
func (b *backend) ReportHistogramDurationSamples(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
	bucketLowerBound,
	bucketUpperBound time.Duration,
	samples int64,
) {

	j, err := json.Marshal(types.HistogramDuration{
		Identity:   makeIdentity(b.tool, name, tags),
		Values:     buckets.AsDurations(),
		LowerBound: bucketLowerBound,
		UpperBound: bucketUpperBound,
		Samples:    samples,
	})

	if err != nil {
		panic(fmt.Errorf("failed to marshal histogram duration: %v", err))
	}

	b.metrics <- &metric{
		name:  types.HDurationKey,
		value: string(j),
	}
}

func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}

	res := make(map[string]string, len(m))
	for k, v := range m {
		res[k] = v
	}

	return res
}

func mustParse(u string) *url.URL {
	res, err := url.Parse(u)
	if err != nil {
		panic(err)
	}

	return res
}

func mustBake() *cookiejar.Jar {
	jar, err := cookiejar.New(
		&cookiejar.Options{PublicSuffixList: publicsuffix.List})

	if err != nil {
		panic(err)
	}

	return jar
}
