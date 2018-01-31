package rpc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/.gen/go/pullo"
	"code.uber.internal/engsec/wonka-go.git/internal/logging"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"
)

const defaultMuttleyHostPort = "127.0.0.1:5437"

// PulloClient is a pullo connection for getting group information on a user.
type PulloClient interface {
	GetGroupsForUser(ctx context.Context, user string) (map[string]struct{}, error)
	IsMemberOf(ctx context.Context, user, group string) (bool, error)
}

// NOTE(abg): This could totally just use YARPC instead.

// wmPulloClient implements the PulloClient interface.
type wmPulloClient struct {
	log     *zap.Logger
	tClient pullo.TChanPullo
}

type pulloClientConfig struct {
	Logger *zap.Logger
	Level  zap.AtomicLevel

	// If unset, defaultMuttleyHostPort will be used.
	MuttleyHostPort string

	// If unset, a new TChannel client will be built
	TClient pullo.TChanPullo
}

func (cfg *pulloClientConfig) BuildTChannel() (*tchannel.Channel, error) {
	var opts tchannel.ChannelOptions
	if cfg.Logger != nil {
		opts.Logger = logging.NewTChannelLogger(cfg.Logger, cfg.Level)
	}

	ch, err := tchannel.NewChannel("wonkamaster", &opts)
	if err != nil {
		return nil, err
	}

	ch.Peers().Add(cfg.MuttleyHostPort)
	return ch, nil
}

// PulloClientOption customizes the behavior of a Pullo client.
type PulloClientOption func(*pulloClientConfig)

// Logger specifies how messages will be logged.
func Logger(log *zap.Logger, level zap.AtomicLevel) PulloClientOption {
	return func(cfg *pulloClientConfig) {
		cfg.Logger = log
		cfg.Level = level
	}
}

// MuttleyHostPort specifies the address at which the router for Pullo
// requests is available.
func MuttleyHostPort(hostPort string) PulloClientOption {
	return func(cfg *pulloClientConfig) {
		cfg.MuttleyHostPort = hostPort
	}
}

// Client specifies inner tchannel client to use for Pullo requests, thus
// allowing the responses to be mocked.
func Client(tClient pullo.TChanPullo) PulloClientOption {
	return func(cfg *pulloClientConfig) {
		cfg.TClient = tClient
	}
}

// NewPulloClient returns a new pullo client object
func NewPulloClient(opts ...PulloClientOption) (PulloClient, error) {
	cfg := pulloClientConfig{
		Logger:          zap.L(),
		Level:           zap.NewAtomicLevel(),
		MuttleyHostPort: defaultMuttleyHostPort,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.TClient == nil {
		ch, err := cfg.BuildTChannel()
		if err != nil {
			return nil, fmt.Errorf("failed to set up TChannel: %v", err)
		}
		cfg.TClient = pullo.NewTChanPulloClient(thrift.NewClient(ch, "pullo", nil))
	}

	return &wmPulloClient{
		log:     cfg.Logger,
		tClient: cfg.TClient,
	}, nil
}

func (u *wmPulloClient) IsMemberOf(ctx context.Context, user, group string) (bool, error) {
	tctx, cancel := buildContext(ctx)
	defer cancel()

	ok, err := u.tClient.IsMemberOf(tctx, user, group)
	if err != nil {
		u.log.Error("error making pullo request", zap.String("user", user), zap.Error(err))
		return false, err
	}

	return ok, nil
}

// we should be using titus for this.
func (u *wmPulloClient) GetGroupsForUser(ctx context.Context, user string) (map[string]struct{}, error) {
	adGroups := map[string]struct{}{
		wonka.EveryEntity: {},
	}

	tctx, cancel := buildContext(ctx)
	defer cancel()

	groups, err := u.tClient.GetUserGroups(tctx, user)
	if err != nil {
		u.log.Error("error querying pullo", zap.String("user", user), zap.Error(err))
		return adGroups, err
	}

	for _, g := range groups {
		if g == "" {
			continue
		}
		adGroups[CanonicalGroupName(g)] = struct{}{}
	}

	return adGroups, nil
}

// CanonicalGroupName returns canonical AD group format: lower case with prefix
func CanonicalGroupName(g string) string {
	if strings.EqualFold(wonka.EveryEntity, g) {
		return wonka.EveryEntity // UPPERCASE
	} else if strings.EqualFold("ad:", g[:3]) {
		return strings.ToLower(g)
	}
	return fmt.Sprintf("ad:%s", strings.ToLower(g))
}

// buildContext returns a cancellable thrift context for requests to Pullo.
func buildContext(ctx context.Context) (thrift.Context, context.CancelFunc) {
	tctx, cancel := tchannel.NewContextBuilder(60*time.Second).
		SetParentContext(ctx).
		AddHeader("X-Uber-Source", "wonkamaster").
		Build()
	// NewContextBuilder returns x/net/context, not context from standard lib.
	return tctx, context.CancelFunc(cancel)
}
