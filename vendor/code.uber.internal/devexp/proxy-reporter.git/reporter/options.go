package reporter

import (
	"net/url"
	"time"
)

// Option lets you to configure the reporter.
type Option interface {
	apply(*cfg)
}

// cfg holds parameters for a reporter.
type cfg struct {
	// Interval between reporting.
	interval time.Duration

	// Max number of metrics to emit per interval tick.
	count int

	// Address where to send metrics.
	addr *url.URL

	// Entity name to authorize requests.
	entity string

	ussoAuth UssoAuth

	// Rate to sample metrics.
	sample float64
}

var defaultCfg = cfg{
	interval: 5 * time.Second,
	count:    100,
	addr:     mustParse("https://proxyreporter.uberinternal.com/tally"),
	entity:   "proxyreporter",
	ussoAuth: OfflineToken{},
	sample:   0.01,
}

type opt func(c *cfg)

func (o opt) apply(c *cfg) {
	o(c)
}

// WithInterval sets the reporting interval for reporter.
func WithInterval(interval time.Duration) Option {
	return opt(func(c *cfg) {
		c.interval = interval
	})
}

// WithUssoAuth sets the auth type to get offline token from ssh cert
func WithUssoAuth(ua UssoAuth) Option {
	return opt(func(c *cfg) {
		c.ussoAuth = ua
	})
}

// WithEntity sets the name of the entity that sends metrics.
func WithEntity(entity string) Option {
	return opt(func(c *cfg) {
		c.entity = entity
	})
}

// WithAddress sets the reporter's endpoint.
func WithAddress(addr string) Option {
	u := mustParse(addr)

	return opt(func(c *cfg) {
		c.addr = u
	})
}

// WithSample sets a sample rate for metrics reporting.
// It is the probability to get actual reporter, not a noop version.
func WithSample(rate float64) Option {
	return opt(func(c *cfg) {
		c.sample = rate
	})
}
