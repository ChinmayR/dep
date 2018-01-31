package redswitch

import (
	"crypto"
	"crypto/ecdsa"
	"errors"
	"time"

	"code.uber.internal/engsec/wonka-go.git/internal/dns"

	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

// ReaderOption defines a single parameter for constructing a Reader.
type ReaderOption func(*readerOptions)

// ReaderOptions defines parameters for constructing a Reader.
type readerOptions struct {
	log      *zap.Logger
	metrics  tally.Scope
	recovery chan<- time.Time
	keys     []crypto.PublicKey
	dns      dns.Client
}

func newReaderOptions() *readerOptions {
	return &readerOptions{dns: dns.DefaultClient}
}

func (g *readerOptions) validate() error {
	if g.log == nil {
		return errors.New("WithLogger option is required")
	}
	if g.metrics == nil {
		return errors.New("WithMetrics option is required")
	}
	if len(g.keys) == 0 {
		return errors.New("WithPublicKeys option is required")
	}
	if g.dns == nil {
		return errors.New("the DNS client cannot be set to nil")
	}
	return nil
}

func (g *readerOptions) publicKeys() ([]*ecdsa.PublicKey, error) {
	k := make([]*ecdsa.PublicKey, len(g.keys))
	for i := range g.keys {
		ec, ok := g.keys[i].(*ecdsa.PublicKey)
		if !ok {
			return nil, errors.New("only ecdsa.PublicKey keys are supported")
		}

		k[i] = ec
	}
	return k, nil
}

// WithLogger defines a logger that the Reader should use.
func WithLogger(log *zap.Logger) ReaderOption {
	return func(o *readerOptions) {
		o.log = log
	}
}

// WithMetrics defines a Scope that the Reader should use for
// emitting metrics.
func WithMetrics(m tally.Scope) ReaderOption {
	return func(o *readerOptions) {
		o.metrics = m
	}
}

// WithRecoveryNotification defines a channel that should be used
// for notification when transitioning from a globally disabled
// state into an enabled state.
func WithRecoveryNotification(c chan<- time.Time) ReaderOption {
	return func(o *readerOptions) {
		o.recovery = c
	}
}

// WithPublicKeys defines the Wonkamaster public keys. These keys
// are used for validating the signature of the global disabled
// DNS record(s).
func WithPublicKeys(keys ...crypto.PublicKey) ReaderOption {
	return func(o *readerOptions) {
		o.keys = keys
	}
}

// WithDNSClient sets the DNS client that will be used for querying the
// globally disabled status.
func WithDNSClient(c dns.Client) ReaderOption {
	return func(o *readerOptions) {
		o.dns = c
	}
}
